package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Frame struct {
	Chain                  string
	MS, Latency, PCLatency float64
	PCValid                bool
	At                     time.Time
}
type Perf struct {
	Fresh                            bool
	FPS, Low, FrameMS, Peak, Latency float64
	LatencyHeld                      bool
	LatencySource                    string
	Graph                            [116]float64
	Samples                          int
}
type Stats struct{ chains map[string][]Frame }

func newStats() *Stats  { return &Stats{chains: make(map[string][]Frame)} }
func (s *Stats) Reset() { clear(s.chains) }
func (s *Stats) Add(f Frame) {
	if f.MS <= 0 || math.IsNaN(f.MS) || math.IsInf(f.MS, 0) {
		return
	}
	fs := s.chains[f.Chain]
	cutoff := f.At.Add(-30 * time.Second)
	n := 0
	for n < len(fs) && fs[n].At.Before(cutoff) {
		n++
	}
	fs = append(fs[n:], f)
	if len(fs) > 100000 {
		fs = fs[len(fs)-100000:]
	}
	s.chains[f.Chain] = fs
	for k, v := range s.chains {
		if len(v) > 0 && v[len(v)-1].At.Before(cutoff) {
			delete(s.chains, k)
		}
	}
}

type latencyWindow struct {
	sum, last float64
	count     int
	at        time.Time
}

func (w *latencyWindow) add(value float64, at, end time.Time) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return
	}
	if w.at.IsZero() || !at.Before(w.at) {
		w.last, w.at = value, at
	}
	if age := end.Sub(at); age >= 0 && age <= 2*time.Second {
		w.sum += value
		w.count++
	}
}
func (w latencyWindow) result(now time.Time) (value float64, held, valid bool) {
	if w.count > 0 {
		return w.sum / float64(w.count), false, true
	}
	age := now.Sub(w.at)
	if !w.at.IsZero() && age >= 0 && age <= 10*time.Second {
		return w.last, true, true
	}
	return math.NaN(), false, false
}
func (s *Stats) Snapshot(now time.Time) Perf {
	var best []Frame
	bestCount := 0
	for _, fs := range s.chains {
		if len(fs) == 0 || now.Sub(fs[len(fs)-1].At) > 3*time.Second {
			continue
		}
		end := fs[len(fs)-1].At
		count := 0
		for _, f := range fs {
			if !f.At.Before(end.Add(-2*time.Second)) && !f.At.After(end) {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = fs, count
		}
	}
	p := Perf{Latency: math.NaN()}
	if len(best) == 0 || now.Sub(best[len(best)-1].At) > 3*time.Second {
		return p
	}
	p.Fresh = true
	end := best[len(best)-1].At
	var times []float64
	sum := 0.0
	count := 0
	var input, pc latencyWindow
	for _, f := range best {
		age := end.Sub(f.At)
		if age < 0 || age > 30*time.Second {
			continue
		}
		times = append(times, f.MS)
		input.add(f.Latency, f.At, end)
		if f.PCValid {
			pc.add(f.PCLatency, f.At, end)
		}
		if age <= 2*time.Second {
			sum += f.MS
			count++
		}
		if age < 10*time.Second {
			x := 115 - int(age.Seconds()/10*116)
			if x >= 0 && x < 116 && f.MS > p.Graph[x] {
				p.Graph[x] = f.MS
			}
			if f.MS > p.Peak {
				p.Peak = f.MS
			}
		}
	}
	if count > 0 {
		p.FPS = 1000 * float64(count) / sum
		p.FrameMS = sum / float64(count)
	}
	if value, held, ok := pc.result(now); ok {
		p.Latency, p.LatencyHeld, p.LatencySource = value, held, "pc"
	} else if value, held, ok := input.result(now); ok {
		p.Latency, p.LatencyHeld, p.LatencySource = value, held, "input"
	}
	p.Samples = len(times)
	sort.Sort(sort.Reverse(sort.Float64Slice(times)))
	n := int(math.Ceil(float64(len(times)) * .01))
	if n > 0 {
		sum = 0
		for _, v := range times[:n] {
			sum += v
		}
		p.Low = 1000 * float64(n) / sum
	}
	return p
}

func readFrames(r io.Reader, start time.Time, pid uint32, emit func(Frame)) error {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return err
	}
	fields := map[string]int{}
	for i, h := range header {
		fields[strings.TrimPrefix(strings.TrimSpace(h), "\ufeff")] = i
	}
	for _, name := range []string{"ProcessID", "SwapChainAddress", "MsBetweenPresents"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("PresentMon CSV missing %s", name)
		}
	}
	timeKey := "TimeInMs"
	if _, ok := fields[timeKey]; !ok {
		timeKey = "CPUStartTime"
		if _, ok := fields[timeKey]; !ok {
			return fmt.Errorf("PresentMon CSV missing timestamp")
		}
	}
	for {
		row, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("PresentMon CSV: %w", err)
		}
		get := func(key string) string {
			i, ok := fields[key]
			if !ok || i >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[i])
		}
		got, e := strconv.ParseUint(get("ProcessID"), 10, 32)
		if e != nil || (pid != 0 && uint32(got) != pid) {
			continue
		}
		ms, e := strconv.ParseFloat(get("MsBetweenPresents"), 64)
		if e != nil || ms <= 0 || math.IsNaN(ms) || math.IsInf(ms, 0) {
			continue
		}
		ts, e := strconv.ParseFloat(get(timeKey), 64)
		if e != nil || math.IsNaN(ts) || math.IsInf(ts, 0) || ts < 0 || ts > 1e12 {
			continue
		}
		chain := get("SwapChainAddress")
		if pid == 0 {
			chain = strconv.FormatUint(got, 10) + ":" + chain
		}
		if chain == "" {
			continue
		}
		latency := math.NaN()
		if v, e := strconv.ParseFloat(get("MsAllInputToPhotonLatency"), 64); e == nil && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			latency = v
		}
		pc, pcValid := math.NaN(), false
		if v, e := strconv.ParseFloat(get("MsPCLatency"), 64); e == nil && v >= 0 && !math.IsNaN(v) && !math.IsInf(v, 0) {
			pc, pcValid = v, true
		}
		emit(Frame{Chain: chain, MS: ms, Latency: latency, PCLatency: pc, PCValid: pcValid, At: start.Add(time.Duration(ts * float64(time.Millisecond)))})
	}
}

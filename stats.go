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
	PresentTiming          bool
	At                     time.Time
	SimulationMS           float64
	SimulationAt           time.Time
}
type Perf struct {
	Fresh                            bool
	FPS, Low, FrameMS, Peak, Latency float64
	BaseFPS                          float64
	LatencyHeld                      bool
	LatencySource                    string
	PresentTiming                    bool
	Graph                            [116]float64
	Samples                          int
}
type Stats struct{ chains map[string][]Frame }

func newStats() *Stats  { return &Stats{chains: make(map[string][]Frame)} }
func (s *Stats) Reset() { clear(s.chains) }
func (s *Stats) Add(f Frame) {
	if (f.MS <= 0 || math.IsNaN(f.MS) || math.IsInf(f.MS, 0)) && !validInterval(f.SimulationMS) {
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
	var end time.Time
	bestCount := 0
	for _, fs := range s.chains {
		var displayEnd time.Time
		for _, f := range fs {
			if validInterval(f.MS) && f.At.After(displayEnd) {
				displayEnd = f.At
			}
		}
		if displayEnd.IsZero() || now.Sub(displayEnd) > 3*time.Second {
			continue
		}
		count := 0
		for _, f := range fs {
			if validInterval(f.MS) && !f.At.Before(displayEnd.Add(-2*time.Second)) && !f.At.After(displayEnd) {
				count++
			}
		}
		if count > bestCount {
			best, bestCount = fs, count
			end = displayEnd
		}
	}
	p := Perf{Latency: math.NaN(), BaseFPS: math.NaN()}
	if len(best) == 0 {
		return p
	}
	p.Fresh = true
	p.PresentTiming = best[len(best)-1].PresentTiming
	var times []float64
	sum := 0.0
	count := 0
	simulationSum, simulationCount := 0.0, 0
	var input, pc latencyWindow
	for _, f := range best {
		if age := end.Sub(f.SimulationAt); validInterval(f.SimulationMS) && age >= 0 && age <= 2*time.Second {
			simulationSum += f.SimulationMS
			simulationCount++
		}
		age := end.Sub(f.At)
		if age < 0 || age > 30*time.Second || !validInterval(f.MS) {
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
	if simulationCount > 0 {
		p.BaseFPS = 1000 * float64(simulationCount) / simulationSum
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

func validInterval(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
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
	for _, name := range []string{"ProcessID", "SwapChainAddress"} {
		if _, ok := fields[name]; !ok {
			return fmt.Errorf("PresentMon CSV missing %s", name)
		}
	}
	// Select one timing source per stream; never mix present and display intervals.
	timingKey := "MsBetweenDisplayChange"
	_, displayTiming := fields[timingKey]
	if !displayTiming {
		timingKey = "MsBetweenPresents"
		if _, ok := fields[timingKey]; !ok {
			return fmt.Errorf("PresentMon CSV missing display and present timing")
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
		ms, e := strconv.ParseFloat(get(timingKey), 64)
		if e != nil || ms <= 0 || math.IsNaN(ms) || math.IsInf(ms, 0) {
			ms = 0
		}
		simulation, _ := strconv.ParseFloat(get("MsBetweenSimulationStart"), 64)
		if !validInterval(simulation) {
			simulation = 0
		}
		if ms == 0 && simulation == 0 {
			continue
		}
		ts, e := strconv.ParseFloat(get(timeKey), 64)
		if e != nil || math.IsNaN(ts) || math.IsInf(ts, 0) || ts < 0 || ts > 1e12 {
			continue
		}
		simulationAt := start.Add(time.Duration(ts * float64(time.Millisecond)))
		if displayTiming {
			// Place display intervals at their display event when the offset is known.
			if offset, err := strconv.ParseFloat(get("MsUntilDisplayed"), 64); err == nil && offset >= 0 && offset <= 1e12 && !math.IsNaN(offset) && !math.IsInf(offset, 0) {
				ts += offset
			}
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
		emit(Frame{Chain: chain, MS: ms, Latency: latency, PCLatency: pc, PCValid: pcValid, PresentTiming: !displayTiming, At: start.Add(time.Duration(ts * float64(time.Millisecond))), SimulationMS: simulation, SimulationAt: simulationAt})
	}
}

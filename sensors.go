package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Metrics struct{ CPU, GPU, CPUTemp, GPUTemp float64 }

func missingMetrics() Metrics { return Metrics{math.NaN(), math.NaN(), math.NaN(), math.NaN()} }

type sensorNode struct {
	Text, SensorId, Value string
	Children              []sensorNode
}

func parseCPUTemperature(r io.Reader) (float64, error) {
	var root sensorNode
	if err := json.NewDecoder(io.LimitReader(r, 4<<20)).Decode(&root); err != nil {
		return 0, err
	}
	best := math.NaN()
	priority := -1
	var visit func(sensorNode)
	visit = func(n sensorNode) {
		id := strings.ToLower(n.SensorId)
		label := strings.ToLower(n.Text)
		if strings.Contains(id, "cpu") && strings.Contains(id, "/temperature/") {
			p := 0
			if strings.Contains(label, "package") || strings.Contains(label, "tctl/tdie") {
				p = 3
			} else if strings.Contains(label, "tdie") || strings.Contains(label, "core average") {
				p = 2
			} else if strings.Contains(label, "core") {
				p = 1
			}
			fields := strings.Fields(strings.ReplaceAll(n.Value, ",", "."))
			if len(fields) > 0 {
				v, e := strconv.ParseFloat(fields[0], 64)
				if e == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 && v < 150 && p > priority {
					best, priority = v, p
				}
			}
		}
		for _, child := range n.Children {
			visit(child)
		}
	}
	visit(root)
	if math.IsNaN(best) {
		return 0, fmt.Errorf("LibreHardwareMonitor has no CPU temperature sensor")
	}
	return best, nil
}
func cpuTemperature(url string) (float64, error) {
	if url == "" {
		return 0, fmt.Errorf("LibreHardwareMonitor endpoint disabled")
	}
	client := http.Client{Timeout: 1200 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("LibreHardwareMonitor HTTP %d", resp.StatusCode)
	}
	return parseCPUTemperature(resp.Body)
}

func readSensorFile(path string, now time.Time) (float64, error) {
	var b []byte
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		b, err = os.ReadFile(path)
		if err == nil || (!errors.Is(err, syscall.Errno(32)) && !errors.Is(err, syscall.Errno(33))) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		return 0, err
	}
	if len(b) > 4<<20 {
		return 0, fmt.Errorf("sensor snapshot too large")
	}
	var envelope struct{ Updated time.Time }
	if err = json.Unmarshal(b, &envelope); err != nil {
		return 0, err
	}
	age := now.Sub(envelope.Updated)
	if envelope.Updated.IsZero() || age > 8*time.Second || age < -5*time.Second {
		return 0, fmt.Errorf("LibreHardwareMonitor sensor snapshot is stale")
	}
	return parseCPUTemperature(bytes.NewReader(b))
}
func configuredCPUTemp(c Config) (float64, error) {
	if c.LHMURL != "" {
		return cpuTemperature(c.LHMURL)
	}
	return readSensorFile(c.LHMFile, time.Now())
}

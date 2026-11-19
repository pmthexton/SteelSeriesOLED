package main

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecordedBaseFPS(t *testing.T) {
	path := os.Getenv("OLED_BASE_TRACE")
	if path == "" {
		t.Skip("set OLED_BASE_TRACE to replay a simulation-instrumented PresentMon CSV")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s := newStats()
	var latest time.Time
	if err := readFrames(f, time.Unix(100, 0), 0, func(f Frame) {
		s.Add(f)
		if f.At.After(latest) {
			latest = f.At
		}
	}); err != nil {
		t.Fatal(err)
	}
	p := s.Snapshot(latest)
	t.Logf("display FPS=%.2f base FPS=%.2f", p.FPS, p.BaseFPS)
	if !p.Fresh || !validInterval(p.BaseFPS) {
		t.Fatalf("no base telemetry in instrumented recording: %+v", p)
	}
}

func TestBaseFPSUsesSimulationIntervalsIncludingUndisplayedFrames(t *testing.T) {
	start := time.Unix(100, 0)
	var csv strings.Builder
	csv.WriteString("ProcessID,SwapChainAddress,TimeInMs,MsBetweenDisplayChange,MsBetweenSimulationStart\n")
	for i := 0; i < 200; i++ {
		simulation, display := "NA", "10"
		if i%2 == 0 {
			simulation = "20"
		}
		if i%4 == 0 {
			display = "NA" // Losing displayed frames must not lose simulation samples.
			simulation = "40"
		}
		fmt.Fprintf(&csv, "42,main,%d,%s,%s\n", i*10, display, simulation)
	}
	s := newStats()
	if err := readFrames(strings.NewReader(csv.String()), start, 42, s.Add); err != nil {
		t.Fatal(err)
	}
	p := s.Snapshot(start.Add(2 * time.Second))
	if !p.Fresh || math.Abs(p.BaseFPS-1000.0/30) > .001 || p.FPS != 100 {
		t.Fatalf("display/base rates: %+v", p)
	}
}

func TestBaseFPSUnavailableExpiryAndSwapChain(t *testing.T) {
	start := time.Unix(100, 0)
	for _, invalid := range []string{"NA", "0", "-1", "NaN", "+Inf", "bad"} {
		s := newStats()
		raw := "ProcessID,SwapChainAddress,TimeInMs,MsBetweenDisplayChange,MsBetweenSimulationStart\n" +
			"42,main,0,10,20\n42,other,2500,10,5\n42,main,3000,10," + invalid + "\n42,main,3010,10," + invalid + "\n"
		if err := readFrames(strings.NewReader(raw), start, 42, s.Add); err != nil {
			t.Fatal(err)
		}
		if p := s.Snapshot(start.Add(3100 * time.Millisecond)); !math.IsNaN(p.BaseFPS) {
			t.Fatalf("expired/other-chain base survived %q: %+v", invalid, p)
		}
	}
}

func TestBaseFPSLineAlwaysVisibleAndClearsWhenStale(t *testing.T) {
	for _, p := range []Perf{{}, {Fresh: true}, {Fresh: true, BaseFPS: math.NaN()}, {BaseFPS: 60}} {
		b := render(time.Unix(0, 0), "game.exe", p, missingMetrics())
		var want Bitmap
		want.Text("BASE --", 1, 14, 1)
		for y := 14; y < 21; y++ {
			for x := 1; x < 43; x++ {
				if b[y*128+x] != want[y*128+x] {
					t.Fatalf("missing BASE -- at %d,%d for %+v", x, y, p)
				}
			}
		}
	}
}

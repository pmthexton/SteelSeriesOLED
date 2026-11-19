package main

import (
	"math"
	"os"
	"testing"
	"time"
)

func TestRecordedWitcherDisplayIntervals(t *testing.T) {
	path := os.Getenv("WITCHER_TRACE")
	if path == "" {
		t.Skip("set WITCHER_TRACE to replay recorded PresentMon CSV")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	start := time.Unix(100, 0)
	stats := newStats()
	var latest time.Time
	count := 0
	err = readFrames(file, start, 0, func(f Frame) {
		stats.Add(f)
		if f.At.After(latest) {
			latest = f.At
		}
		count++
	})
	if err != nil {
		t.Fatal(err)
	}
	p := stats.Snapshot(latest)
	t.Logf("frames=%d displayed FPS=%.2f 1%% low=%.2f APP=%v", count, p.FPS, p.Low, p.PresentTiming)
	if count != 32575 || math.Abs(p.FPS-108.6) > 1 || p.Low < 90 || p.Low > 100 || p.PresentTiming {
		t.Fatalf("unexpected replay: %+v", p)
	}
}

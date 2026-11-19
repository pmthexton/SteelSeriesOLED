package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

func TestDisplayTimingRemovesPairedPresentLowArtifact(t *testing.T) {
	var csv strings.Builder
	csv.WriteString("ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsBetweenDisplayChange,MsUntilDisplayed\n")
	for i := 0; i < 200; i++ {
		present := 0.3
		if i%2 == 1 {
			present = 18.1
		}
		fmt.Fprintf(&csv, "42,main,%.1f,%.1f,9.2,20\n", float64(i)*9.2, present)
	}
	start := time.Unix(100, 0)
	stats := newStats()
	if err := readFrames(strings.NewReader(csv.String()), start, 42, stats.Add); err != nil {
		t.Fatal(err)
	}
	p := stats.Snapshot(start.Add(2 * time.Second))
	want := 1000 / 9.2
	if math.Abs(p.FPS-want) > .001 || math.Abs(p.Low-want) > .001 || math.Abs(p.Peak-9.2) > .001 {
		t.Fatalf("paired presents biased display stats: FPS=%.3f low=%.3f peak=%.3f; want %.3f/%.3f/9.2", p.FPS, p.Low, p.Peak, want, want)
	}
}

func TestDisplayTimingSkipsUnavailableAndDroppedSamples(t *testing.T) {
	csv := "ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsBetweenDisplayChange,MsUntilDisplayed\n"
	for i, display := range []string{"NA", "0", "-1", "NaN", "+Inf", "bad", "8.5"} {
		csv += fmt.Sprintf("42,main,%d,10,%s,4\n", i*10, display)
	}
	var frames []Frame
	if err := readFrames(strings.NewReader(csv), time.Unix(0, 0), 42, func(f Frame) { frames = append(frames, f) }); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].MS != 8.5 {
		t.Fatalf("display samples=%+v", frames)
	}
	if frames[0].At.UnixNano() != 64000000 {
		t.Fatalf("sample must be timed at display: %v", frames[0].At)
	}
}

func TestDisplayTimingDoesNotRequireValidPresentInterval(t *testing.T) {
	csv := "ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsBetweenDisplayChange\n42,main,100,NA,8.5\n"
	var frames []Frame
	if err := readFrames(strings.NewReader(csv), time.Unix(0, 0), 42, func(f Frame) { frames = append(frames, f) }); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 || frames[0].MS != 8.5 {
		t.Fatalf("valid display sample lost: %+v", frames)
	}
}

func TestDisplayTimingNeverFallsBackPerRow(t *testing.T) {
	csv := "ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsBetweenDisplayChange\n42,main,100,10,NA\n"
	var frames []Frame
	if err := readFrames(strings.NewReader(csv), time.Unix(0, 0), 42, func(f Frame) { frames = append(frames, f) }); err != nil {
		t.Fatal(err)
	}
	if len(frames) != 0 {
		t.Fatalf("unavailable display timing became application timing: %+v", frames)
	}
}

func TestTimingSourceAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name, header, row string
		app               bool
		want              float64
	}{
		{"display only", "ProcessID,SwapChainAddress,TimeInMs,MsBetweenDisplayChange", "42,main,100,8.5", false, 8.5},
		{"legacy present", "ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents", "42,main,100,10", true, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := newStats()
			start := time.Unix(100, 0)
			if err := readFrames(strings.NewReader(tc.header+"\n"+tc.row+"\n"), start, 42, stats.Add); err != nil {
				t.Fatal(err)
			}
			p := stats.Snapshot(start.Add(time.Second))
			if !p.Fresh || p.PresentTiming != tc.app || p.FrameMS != tc.want {
				t.Fatalf("source/interval: %+v", p)
			}
			normal := render(start, "game.exe", Perf{Fresh: true, FPS: 100, PresentTiming: false}, Metrics{})
			fallback := render(start, "game.exe", Perf{Fresh: true, FPS: 100, PresentTiming: true}, Metrics{})
			if normal == fallback {
				t.Fatal("fallback must visibly distinguish APP timing from FPS")
			}
		})
	}
}

package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Catches mixing frame intervals, including unrelated swap chains in a game.
func TestStatsWindowsAndSwapChains(t *testing.T) {
	now := time.Unix(1000, 0)
	s := newStats()
	for i := 0; i < 100; i++ {
		ms := 10.0
		if i == 99 {
			ms = 100
		}
		s.Add(Frame{Chain: "main", MS: ms, At: now.Add(time.Duration(i-100) * 10 * time.Millisecond)})
	}
	for i := 0; i < 10; i++ {
		s.Add(Frame{Chain: "launcher", MS: 200, At: now})
	}
	p := s.Snapshot(now)
	if !p.Fresh || math.Abs(p.FPS-1000/10.9) > 0.01 || p.Low != 10 {
		t.Fatalf("unexpected stats: %+v", p)
	}
	if s.Snapshot(now.Add(4 * time.Second)).Fresh {
		t.Fatal("stale FPS survived")
	}
	s.Reset()
	if s.Snapshot(now).Fresh {
		t.Fatal("history survived game exit")
	}
}

// Catches graph averaging away a single stutter in a time bucket.
func TestGraphPreservesSpikes(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	s.Add(Frame{Chain: "x", MS: 5, At: now.Add(-time.Second)})
	s.Add(Frame{Chain: "x", MS: 90, At: now.Add(-time.Second)})
	s.Add(Frame{Chain: "x", MS: 5, At: now})
	p := s.Snapshot(now)
	max := 0.0
	for _, v := range p.Graph {
		if v > max {
			max = v
		}
	}
	if max != 90 {
		t.Fatalf("lost spike: %v", max)
	}
}

// Catches NA/invalid fields entering the FPS calculation or CSV schema changes.
func TestCSVStream(t *testing.T) {
	input := "Application,ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents\nfoo.exe,42,0x1,100,8.5\nfoo.exe,42,0x1,110,NA\nfoo.exe,42,0x1,120,NaN\nfoo.exe,42,0x1,130,0\n"
	var frames []Frame
	err := readFrames(strings.NewReader(input), time.Unix(0, 0), 42, func(f Frame) { frames = append(frames, f) })
	if err != nil || len(frames) != 1 || frames[0].MS != 8.5 || frames[0].At.UnixNano() != 100000000 {
		t.Fatalf("frames=%+v err=%v", frames, err)
	}
	if err := readFrames(strings.NewReader("not,a,csv,schema\n"), time.Now(), 42, func(Frame) {}); err == nil {
		t.Fatal("missing schema accepted")
	}
}

// Catches prefix paths accepting a neighbouring browser folder and helper exes.
func TestGamesOnly(t *testing.T) {
	c := Config{GameRoots: []string{`D:\SteamLibrary\steamapps\common\Portal 2`}, Games: []string{"custom.exe"}}
	for _, path := range []string{`D:\SteamLibrary\steamapps\common\Portal 2\portal2.exe`, `E:\Other\custom.exe`} {
		if !c.IsGame(path) {
			t.Fatalf("rejected game %s", path)
		}
	}
	for _, path := range []string{`C:\Chrome\chrome.exe`, `D:\SteamLibrary\steamapps\common\Portal 2 Extra\browser.exe`, `D:\SteamLibrary\steamapps\common\Portal 2\bin\steamwebhelper.exe`, `D:\SteamLibrary\steamapps\common\Portal 2\REDlauncher.exe`} {
		if c.IsGame(path) {
			t.Fatalf("accepted helper/non-game %s", path)
		}
	}
}

// Catches incorrect bit order, half offsets or an extra report-ID byte.
func TestReportWireFormat(t *testing.T) {
	var f Bitmap
	f.Set(0, 0)
	f.Set(1, 7)
	f.Set(64, 63)
	f.Set(127, 63)
	r := f.Reports()
	if len(r) != 2 || len(r[0]) != 1024 {
		t.Fatal("wrong report sizes")
	}
	if r[0][0] != 6 || r[0][1] != 0x93 || r[1][2] != 64 || r[0][6] != 1 || r[0][14] != 128 || r[1][13] != 128 || r[1][517] != 128 {
		t.Fatal("wrong column-major encoding")
	}
}

// Catches clipping or a frozen idle clock, and a dashboard left after exit.
func TestIdleClockAndNight(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	a := render(now, "", Perf{}, Metrics{})
	b := render(now.Add(time.Minute), "", Perf{}, Metrics{})
	if a == b {
		t.Fatal("clock did not shift/update")
	}
	night := render(time.Date(2026, 11, 9, 22, 1, 0, 0, time.UTC), "", Perf{}, Metrics{})
	if night != (Bitmap{}) {
		t.Fatal("night screen must be dark outside cameo")
	}
	game := render(now, "portal2.exe", Perf{Fresh: true, FPS: 120, Low: 80}, Metrics{})
	if game == a {
		t.Fatal("dashboard equals clock")
	}
}

// Catches choosing GPU/ACPI temperature instead of CPU package temperature.
func TestCPUTemperatureSelection(t *testing.T) {
	raw := `{"Text":"root","Children":[{"Text":"GPU","Children":[{"Text":"Temperatures","Children":[{"Text":"GPU Core","SensorId":"/gpu-nvidia/0/temperature/0","Value":"70.0 °C"}]}]},{"Text":"CPU","Children":[{"Text":"Temperatures","Children":[{"Text":"Core #1","SensorId":"/intelcpu/0/temperature/0","Value":"42.0 °C"},{"Text":"CPU Package","SensorId":"/intelcpu/0/temperature/8","Value":"51.5 °C"}]}]}]}`
	v, err := parseCPUTemperature(strings.NewReader(raw))
	if err != nil || v != 51.5 {
		t.Fatalf("temp=%v err=%v", v, err)
	}
	if _, err := parseCPUTemperature(strings.NewReader(`{"Children":[]}`)); err == nil {
		t.Fatal("missing temp became zero")
	}
}

func TestInputToDisplayLatency(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	raw := "Application,ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsUntilDisplayed,MsAllInputToPhotonLatency\nfoo.exe,42,1,0,10,4,45\nfoo.exe,42,1,10,10,7,NA\nfoo.exe,42,1,20,10,8,55\n"
	if err := readFrames(strings.NewReader(raw), now, 42, s.Add); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot(now.Add(time.Second)).Latency; got != 50 {
		t.Fatalf("input-to-display mean=%v, want 50ms", got)
	}
}

func TestInputLatencyNeverFallsBackToPresentLatency(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	raw := "Application,ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsUntilDisplayed\nfoo.exe,42,1,0,10,7\n"
	if err := readFrames(strings.NewReader(raw), now, 42, s.Add); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot(now.Add(time.Second)).Latency; !math.IsNaN(got) {
		t.Fatalf("unavailable input latency became %v", got)
	}
}

// PresentMon emits batches ~1 second behind real time. The statistics window
// must end at the latest captured frame, with delivery staleness checked apart.
func TestBatchedFramesDoNotBlinkOrBiasFPS(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	for i := 0; i < 120; i++ {
		s.Add(Frame{Chain: "game", MS: 16.6666667, At: now.Add(-4500*time.Millisecond + time.Duration(i)*time.Second/60)})
	}
	p := s.Snapshot(now)
	if !p.Fresh || math.Abs(p.FPS-60) > .01 {
		t.Fatalf("batch gap must retain 60 FPS: %+v", p)
	}
	if s.Snapshot(now.Add(4 * time.Second)).Fresh {
		t.Fatal("stale samples survived cutoff")
	}
}

func TestGameSelectionStableForSameApplication(t *testing.T) {
	current := Game{PID: 1, Path: `C:\Edge\msedge.exe`}
	next := Game{PID: 2, Path: `C:\Edge\msedge.exe`}
	if captureTargetChanged(current, next, true) {
		t.Fatal("browser child PID change restarted name-based capture")
	}
	if !captureTargetChanged(current, Game{}, true) {
		t.Fatal("process exit did not stop capture")
	}
}

func TestSensorSnapshotRejectsStaleReadings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sensor.json")
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	data := `{"Updated":"2026-11-09T12:00:00Z","Children":[{"SensorId":"/intelcpu/0/temperature/0","Text":"CPU Package","Value":"50.0 C"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if v, err := readSensorFile(path, now); err != nil || v != 50 {
		t.Fatalf("temp=%v err=%v", v, err)
	}
	if _, err := readSensorFile(path, now.Add(10*time.Second)); err == nil {
		t.Fatal("stale sensor value accepted")
	}
}

func TestGamesWithHelperLikeTitles(t *testing.T) {
	c := Config{GameRoots: []string{`D:\Games`}}
	for _, p := range []string{`D:\Games\Crash Bandicoot\CrashBandicoot.exe`, `D:\Games\SpaceFighter\SpaceFighter.exe`} {
		if !c.IsGame(p) {
			t.Fatalf("title mistaken for helper: %s", p)
		}
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("STEELSERIES_TEST_CHILD") == "badcsv" {
		fmt.Print("invalid,csv,header\n")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestCaptureReportsMalformedCSVExit(t *testing.T) {
	t.Setenv("STEELSERIES_TEST_CHILD", "badcsv")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan captureEvent, 1)
	c, err := startCapture(binary, 42, 7, events, "")
	if err != nil {
		t.Fatal(err)
	}
	defer c.cancel()
	select {
	case e := <-events:
		if !e.Done || e.Err == nil || e.Generation != 7 {
			t.Fatalf("bad completion: %+v", e)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("capture parser failure did not report completion")
	}
}

func TestExplicitMissingConfigFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	if _, err := configFileToLoad(path, true); err == nil {
		t.Fatal("explicit missing config silently enabled defaults")
	}
	if got, err := configFileToLoad(path, false); err != nil || got != "" {
		t.Fatalf("default missing config: %q %v", got, err)
	}
}

func TestGraphCompressionRetainsEdgeAndMiddleSpikes(t *testing.T) {
	var input [116]float64
	input[0] = 60
	input[57] = 80
	input[115] = 100
	columns := graphColumns(input, 110)
	if len(columns) != 110 || columns[0] != 60 || columns[109] != 100 {
		t.Fatal("edge spikes clipped during layout compression")
	}
	found := false
	for _, v := range columns {
		if v == 80 {
			found = true
		}
	}
	if !found {
		t.Fatal("middle spike lost during layout compression")
	}
}

func TestSensorSnapshotRetriesBriefSharingConflict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sensor.json")
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	data := `{"Updated":"2026-11-09T12:00:00Z","Children":[{"SensorId":"/amdcpu/0/temperature/0","Text":"Core (Tctl/Tdie)","Value":"50 C"}]}`
	if e := os.WriteFile(path, []byte(data), 0600); e != nil {
		t.Fatal(e)
	}
	p, _ := syscall.UTF16PtrFromString(path)
	h, e := syscall.CreateFile(p, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	released := make(chan struct{})
	go func() { time.Sleep(40 * time.Millisecond); syscall.CloseHandle(h); close(released) }()
	defer func() { <-released }()
	if v, e := readSensorFile(path, now); e != nil || v != 50 {
		t.Fatalf("brief writer lock blanked reading: %v %v", v, e)
	}
}

// Holding a key may leave many frames without a new input-latency sample.
func TestLatencyRetainsLatestMeasurementForTenSeconds(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	s.Add(Frame{Chain: "game", MS: 10, Latency: 40, At: now})
	s.Add(Frame{Chain: "game", MS: 10, Latency: math.NaN(), At: now.Add(3 * time.Second)})
	p := s.Snapshot(now.Add(3 * time.Second))
	if p.Latency != 40 {
		t.Fatalf("input gap blanked latest latency: %v", p.Latency)
	}
	if !p.LatencyHeld {
		t.Fatal("retained measurement must be marked as held")
	}
	s.Add(Frame{Chain: "game", MS: 10, Latency: math.NaN(), At: now.Add(11 * time.Second)})
	if p := s.Snapshot(now.Add(11 * time.Second)); !math.IsNaN(p.Latency) {
		t.Fatalf("expired input measurement survived: %v", p.Latency)
	}
	s.Reset()
	s.Add(Frame{Chain: "next-game", MS: 10, Latency: math.NaN(), At: now.Add(12 * time.Second)})
	if p := s.Snapshot(now.Add(12 * time.Second)); !math.IsNaN(p.Latency) {
		t.Fatal("latency survived game change")
	}
}

func TestRecentLatencyStillUsesTwoSecondAverage(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	s.Add(Frame{Chain: "game", MS: 10, Latency: 30, At: now})
	s.Add(Frame{Chain: "game", MS: 10, Latency: 50, At: now.Add(time.Second)})
	p := s.Snapshot(now.Add(time.Second))
	if p.Latency != 40 || p.LatencyHeld {
		t.Fatalf("recent latency: %+v", p)
	}
}

func TestInstrumentedPCLatencyPreferredWithoutMixing(t *testing.T) {
	now := time.Unix(100, 0)
	s := newStats()
	raw := "Application,ProcessID,SwapChainAddress,TimeInMs,MsBetweenPresents,MsPCLatency,MsAllInputToPhotonLatency\nfoo.exe,42,1,0,10,20,100\nfoo.exe,42,1,1000,10,40,200\n"
	if e := readFrames(strings.NewReader(raw), now, 42, s.Add); e != nil {
		t.Fatal(e)
	}
	p := s.Snapshot(now.Add(time.Second))
	if p.Latency != 30 || p.LatencySource != "pc" || p.LatencyHeld {
		t.Fatalf("mixed PC/input sources: %+v", p)
	}
	s.Add(Frame{Chain: "1", MS: 10, Latency: 70, At: now.Add(12 * time.Second)})
	p = s.Snapshot(now.Add(12 * time.Second))
	if p.Latency != 70 || p.LatencySource != "input" {
		t.Fatalf("expired PC sample blocked fresh input: %+v", p)
	}
}

// A steady frametime should light only the trace, not every pixel underneath it.
func TestFrametimeGraphIsUnfilledLine(t *testing.T) {
	p := Perf{Fresh: true, FPS: 90, Low: 80, Samples: 100}
	for i := range p.Graph {
		p.Graph[i] = 11
	}
	b := render(time.Unix(0, 0), "game.exe", p, missingMetrics())
	lit := 0
	for x := 16; x < 126; x++ {
		for y := 40; y <= 61; y++ {
			if b[y*128+x] {
				lit++
				if y != 54 {
					t.Fatalf("filled pixel below/above steady trace at %d,%d", x, y)
				}
			}
		}
	}
	if lit != 110 {
		t.Fatalf("steady line has %d pixels, want 110", lit)
	}
}

func TestFrametimeLineRetainsSpikesAndLeavesGaps(t *testing.T) {
	p := Perf{Fresh: true}
	p.Graph[0] = 11
	p.Graph[58] = 330 // clipped spike: input bucket 58 maps to output column 55.
	b := render(time.Unix(0, 0), "game.exe", p, missingMetrics())
	if !b[54*128+16] || !b[40*128+71] {
		t.Fatal("lost steady sample or clipped spike tip")
	}
	if b[54*128+40] {
		t.Fatal("line bridged a missing-data interval")
	}
	if b[61*128+71] {
		t.Fatal("spike filled down to baseline")
	}
}

func TestFrametimeLineUsesDashboardPixelShift(t *testing.T) {
	p := Perf{Fresh: true}
	p.Graph[0] = 11
	a := render(time.Unix(0, 0), "game.exe", p, missingMetrics())
	b := render(time.Unix(60, 0), "game.exe", p, missingMetrics())
	c := render(time.Unix(180, 0), "game.exe", p, missingMetrics())
	if !a[54*128+16] || a[54*128+17] || !b[54*128+17] || b[54*128+16] || !c[55*128+16] || c[54*128+16] {
		t.Fatal("graph did not move with dashboard origin")
	}
}

func TestFrametimeLineConnectsAdjacentSamples(t *testing.T) {
	p := Perf{Fresh: true}
	p.Graph[0] = 11
	p.Graph[2] = 33
	b := render(time.Unix(0, 0), "game.exe", p, missingMetrics())
	if !b[54*128+16] || !b[40*128+17] {
		t.Fatal("line endpoints missing")
	}
	for y := 40; y <= 54; y++ {
		if !b[y*128+16] && !b[y*128+17] {
			t.Fatalf("disconnected trace at row %d", y)
		}
	}
	if b[61*128+16] || b[61*128+17] {
		t.Fatal("connected trace filled to baseline")
	}
}

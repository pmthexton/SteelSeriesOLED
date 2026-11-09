package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

func stopEvent(signal bool) (syscall.Handle, error) {
	name, _ := syscall.UTF16PtrFromString(`Local\SteelSeriesOLEDStop-` + os.Getenv("USERNAME"))
	h, _, e := kernel.NewProc("CreateEventW").Call(0, 1, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return 0, e
	}
	if signal {
		kernel.NewProc("SetEvent").Call(h)
	} else {
		kernel.NewProc("ResetEvent").Call(h)
	}
	return syscall.Handle(h), nil
}
func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
func run() error {
	exe, _ := os.Executable()
	base := filepath.Dir(exe)
	configPath := flag.String("config", filepath.Join(base, "SteelSeriesOLED.json"), "configuration JSON")
	duration := flag.Duration("duration", 0, "exit after duration; 0 keeps running")
	preview := flag.String("preview", "", "write a simulated dashboard PNG, without touching hardware")
	diagnose := flag.Bool("diagnose", false, "print device, game and sensor diagnostics then exit")
	stop := flag.Bool("stop", false, "stop the running app and restore factory display")
	dry := flag.Bool("dry-run", false, "capture real games without writing to OLED")
	testProcess := flag.String("test-process", "", "temporarily allow this executable for hardware testing; not saved")
	logPath := flag.String("log", filepath.Join(os.Getenv("LOCALAPPDATA"), "SteelSeriesOLED", "app.log"), "log file")
	flag.Parse()
	if *duration < 0 {
		return fmt.Errorf("duration cannot be negative")
	}
	if *stop {
		h, e := stopEvent(true)
		if e == nil {
			syscall.CloseHandle(h)
		}
		return e
	}
	if *preview != "" {
		p := Perf{Fresh: true, FPS: 144, Low: 98, Samples: 2000, Peak: 24, Latency: 35}
		for i := range p.Graph {
			p.Graph[i] = 6.5 + float64(i%5)/3
		}
		p.Graph[75] = 24
		return savePreview(*preview, render(time.Date(2026, 11, 9, 12, 0, 0, 0, time.Local), "sample-game.exe", p, Metrics{34, 92, 57, 68}))
	}
	if e := os.MkdirAll(filepath.Dir(*logPath), 0755); e != nil {
		return e
	}
	if info, e := os.Stat(*logPath); e == nil && info.Size() > 2<<20 {
		_ = os.Remove(*logPath + ".old")
		_ = os.Rename(*logPath, *logPath+".old")
	}
	file, e := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	defer file.Close()
	log.SetOutput(io.MultiWriter(file, os.Stderr))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	explicitConfig := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicitConfig = true
		}
	})
	*configPath, e = configFileToLoad(*configPath, explicitConfig)
	if e != nil {
		return e
	}
	cfg, e := loadConfig(*configPath)
	if e != nil {
		return e
	}
	if *testProcess != "" {
		cfg.Games = []string{*testProcess}
		cfg.GameRoots = nil
	}
	gpu, gpuErr := newGPU(cfg.GPUIndex)
	defer gpu.close()
	if *diagnose {
		devices, de := enumerate()
		games, ge := runningGames(cfg)
		temp, te := configuredCPUTemp(cfg)
		usage, gtemp := gpu.sample()
		var cpu cpuSampler
		cpu.sample()
		time.Sleep(250 * time.Millisecond)
		if te != nil {
			temp = math.NaN()
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"presentmon": cfg.PresentMon, "game_roots": cfg.GameRoots, "games": games, "games_error": fmt.Sprint(ge), "devices": devices, "device_error": fmt.Sprint(de), "cpu_percent": number(cpu.sample()), "gpu_percent": number(usage), "gpu_temp": number(gtemp), "gpu_error": fmt.Sprint(gpuErr), "cpu_temp": number(temp), "cpu_temp_error": fmt.Sprint(te)})
	}
	release, e := singleInstance()
	if e != nil {
		return e
	}
	defer release()
	event, e := stopEvent(false)
	if e != nil {
		return e
	}
	defer syscall.CloseHandle(event)
	log.Printf("Started; PresentMon=%s; game roots=%d; normal-user app", cfg.PresentMon, len(cfg.GameRoots))
	if gpuErr != nil {
		log.Printf("GPU unavailable: %v", gpuErr)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *duration > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, *duration)
		defer stop()
	}
	var oled display
	defer oled.close(true)
	stats := newStats()
	reportedPC := false
	var game Game
	var child *capture
	var generation uint64
	defer func() { child.stop() }()
	events := make(chan captureEvent, 8192)
	metricResults := make(chan Metrics, 1)
	sensorCtx, stopSensors := context.WithCancel(ctx)
	sensorsDone := make(chan struct{})
	defer func() { stopSensors(); <-sensorsDone }()
	go func() {
		defer close(sensorsDone)
		var cpu cpuSampler
		cpu.sample()
		timer := time.NewTicker(2 * time.Second)
		defer timer.Stop()
		lastError := ""
		for {
			m := missingMetrics()
			m.CPU = cpu.sample()
			m.GPU, m.GPUTemp = gpu.sample()
			temp, err := configuredCPUTemp(cfg)
			if err == nil {
				m.CPUTemp = temp
			}
			message := ""
			if err != nil {
				message = err.Error()
			}
			if message != lastError {
				if message != "" {
					log.Printf("CPU temperature unavailable: %s", message)
				} else {
					log.Print("CPU temperature connected")
				}
				lastError = message
			}
			select {
			case metricResults <- m:
			default:
			}
			select {
			case <-sensorCtx.Done():
				return
			case <-timer.C:
			}
		}
	}()
	metrics := missingMetrics()
	numbers := Perf{}
	frameTick := time.NewTicker(200 * time.Millisecond)
	defer frameTick.Stop()
	controlTick := time.NewTicker(time.Second)
	defer controlTick.Stop()
	numberTick := time.NewTicker(2 * time.Second)
	defer numberTick.Stop()
	var last Bitmap
	sent := false
	lastSent := time.Time{}
	lastControlError := ""
	nextRetry := time.Time{}
	control := func() {
		if !*dry {
			was := oled.connected()
			if err := oled.refresh(); err != nil {
				if err.Error() != lastControlError {
					log.Printf("HID discovery: %v", err)
					lastControlError = err.Error()
				}
			} else {
				lastControlError = ""
			}
			if was != oled.connected() {
				sent = false
				log.Printf("Windows OLED ownership=%v", oled.connected())
			}
		}
		games, err := runningGames(cfg)
		if err != nil {
			log.Printf("Process discovery: %v", err)
			return
		}
		next := chooseGame(games, game.PID)
		if !*dry && !oled.connected() {
			next = Game{}
		}
		if captureTargetChanged(game, next, *testProcess != "") {
			child.stop()
			child = nil
			generation++
			stats.Reset()
			reportedPC = false
			numbers = Perf{}
			game = next
			nextRetry = time.Time{}
			if game.PID == 0 {
				log.Print("Idle clock")
			} else {
				log.Printf("Game: %s pid=%d", game.Path, game.PID)
			}
		}
		if game.PID != 0 && child == nil && !time.Now().Before(nextRetry) {
			generation++
			child, err = startCapture(cfg.PresentMon, game.PID, generation, events, *testProcess)
			if err != nil {
				log.Printf("Capture start: %v", err)
				nextRetry = time.Now().Add(10 * time.Second)
			}
		}
	}
	control()
	var received int
	var latest time.Time
	var arrival time.Time
	for {
		select {
		case <-ctx.Done():
			log.Print("Stopping; restoring factory UI")
			return nil
		case e := <-events:
			if e.Generation != generation {
				continue
			}
			if e.Done {
				child = nil
				stats.Reset()
				reportedPC = false
				numbers = Perf{}
				nextRetry = time.Now().Add(5 * time.Second)
				log.Printf("Capture ended: %v", e.Err)
			} else {
				stats.Add(e.Frame)
				if e.Frame.PCValid && !reportedPC {
					log.Printf("Instrumented PC latency active: pid=%d sample=%.1fms", game.PID, e.Frame.PCLatency)
					reportedPC = true
				}
				received++
				latest = e.Frame.At
				arrival = time.Now()
			}
		case metrics = <-metricResults:
		case <-numberTick.C:
			numbers = stats.Snapshot(time.Now())
			if *testProcess != "" {
				log.Printf("Timing: received=%d frame_age=%.3fs delivery_age=%.3fs fresh=%v FPS=%.1f LAT=%sms CPU=%s%%/%sC GPU=%s%%/%sC", received, time.Since(latest).Seconds(), time.Since(arrival).Seconds(), numbers.Fresh, numbers.FPS, number(numbers.Latency), number(metrics.CPU), number(metrics.CPUTemp), number(metrics.GPU), number(metrics.GPUTemp))
			}
		case <-controlTick.C:
			if status, _, _ := kernel.NewProc("WaitForSingleObject").Call(uintptr(event), 0); status == 0 {
				log.Print("Stop requested; restoring factory UI")
				return nil
			}
			control()
		case now := <-frameTick.C:
			if !*dry && !oled.connected() {
				continue
			}
			p := stats.Snapshot(now)
			p.FPS, p.Low, p.Samples = numbers.FPS, numbers.Low, numbers.Samples
			p.Latency = numbers.Latency
			p.LatencyHeld = numbers.LatencyHeld
			p.LatencySource = numbers.LatencySource
			if !numbers.Fresh {
				p.Fresh = false
			}
			name := exeBase(game.Path)
			if game.PID == 0 {
				name = ""
			}
			b := render(now, name, p, metrics)
			if *dry {
				continue
			}
			if !sent || b != last || now.Sub(lastSent) >= 30*time.Second {
				if err := oled.write(b); err != nil {
					log.Printf("Display: %v", err)
					sent = false
				} else {
					last, lastSent, sent = b, now, true
				}
			}
		}
	}
}

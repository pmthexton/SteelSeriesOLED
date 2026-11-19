package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

type captureEvent struct {
	Generation uint64
	Frame      Frame
	Err        error
	Done       bool
}
type capture struct {
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	finished chan struct{}
}

func startCapture(binary string, pid uint32, generation uint64, out chan<- captureEvent, testProcess string) (*capture, error) {
	if binary == "" {
		return nil, fmt.Errorf("PresentMon binary not found; configure presentmon path")
	}
	ctx, cancel := context.WithCancel(context.Background())
	targetFlag, target := "--process_id", strconv.FormatUint(uint64(pid), 10)
	if testProcess != "" {
		targetFlag, target = "--process_name", testProcess
		pid = 0
	}
	cmd := exec.CommandContext(ctx, binary, targetFlag, target, "--output_stdout", "--track_pc_latency", "--no_console_stats", "--session_name", "SteelSeriesOLED", "--terminate_on_proc_exit")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	start := time.Now()
	if err = cmd.Start(); err != nil {
		cancel()
		return nil, err
	}
	c := &capture{cancel: cancel, cmd: cmd, finished: make(chan struct{})}
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			log.Printf("PresentMon: %s", scanner.Text())
		}
	}()
	go func() {
		defer close(c.finished)
		defer cancel()
		reportedTiming := false
		err := readFrames(stdout, start, pid, func(f Frame) {
			if !reportedTiming {
				source := "display-change intervals (FPS)"
				if f.PresentTiming {
					source = "application present intervals (APP fallback)"
				}
				log.Printf("Frame timing: %s", source)
				reportedTiming = true
			}
			select {
			case out <- captureEvent{Generation: generation, Frame: f}:
			case <-ctx.Done():
			}
		})
		if err != nil && err != io.EOF && ctx.Err() == nil {
			log.Printf("PresentMon stream: %v", err)
		}
		// Kill on parse error so a writer cannot block forever after stdout stops draining.
		if err != nil {
			_ = cmd.Process.Kill()
		}
		waitErr := cmd.Wait()
		if err == nil {
			err = waitErr
		}
		select {
		case out <- captureEvent{Generation: generation, Err: err, Done: true}:
		case <-ctx.Done():
		}
	}()
	return c, nil
}
func (c *capture) stop() {
	if c == nil {
		return
	}
	// Tell PresentMon to close its own ETW session before force-killing the child.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stop := exec.CommandContext(ctx, c.cmd.Path, "--session_name", "SteelSeriesOLED", "--terminate_existing_session")
	stop.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = stop.Run()
	c.cancel()
	select {
	case <-c.finished:
	case <-time.After(2 * time.Second):
	}
}

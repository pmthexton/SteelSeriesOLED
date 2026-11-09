# Windows OLED Implementation Plan

> Execute inline with superpowers:executing-plans and test-driven-development.

Goal: ship a normal-user Windows executable with a game dashboard, frametime
graph, idle clock and login launch. Spec: design.md. No Go dependencies/cgo.

## Global constraints

- Native HID contract from Mac; no Swift modifications.
- PresentMon CSV, explicit game classification, separate swap chains.
- 2s numbers, 5Hz graph, 30s lows, 10s graph, 3s stale cutoff.
- Missing sensors display --. LibreHardwareMonitor CPU temperature.
- Final exe in user bin; login launch without elevated application token.

## Review focus

Malformed/NA CSV; browser/helper misclassification; stale game exit samples;
reconnected HID handles; stalled sensor endpoints and child-process cleanup.

## Tasks

1. [x] Write failing behavioral tests for CSV parsing, independent swap chains,
   stale/exit reset, FPS statistics, graph spike preservation, clock position,
   report encoding, game path boundaries and CPU temperature selection.
   Run go test ./... and verify missing implementation failure.
2. [x] Implement core in stats.go, frame.go, config.go, sensors.go. Run tests.
3. [x] Adapt verified HID transport into hid_windows.go; add process discovery,
   PresentMon child lifecycle and NVML/GetSystemTimes providers. Test process
   exit/clock fallback and sensor failure paths with real bounded streams.
4. [x] Add main_windows.go, diagnostic/preview/duration modes and bounded logs.
   Run go test ./..., go vet ./..., build; inspect PNG clock/dashboard previews.
5. [x] Verify real PresentMon capture and OLED writes. Install the verified exe,
   config and a standard-user login launcher into user bin/startup. Configure
   LibreHardwareMonitor local sensor file; verify available CPU reading.
6. [x] Review code and rerun relevant checks. Document commands, limitations,
   provider startup needs and uninstall procedure. Do not commit Mac repo.

Ruling: user's explicit instruction to build the agreed display authorizes
execution; no additional design/plan approval round. New independent folder
provides separation from tracked Mac code; do not branch or edit the Swift app.

User steering incorporated: group usage/temp left and LAT right; taller graph;
LAT is now input-to-display rather than present-to-display, with no fallback;
Steam, Epic and GOG discovery. Installed app now runs without the Edge override.
Live game input-latency availability remains under verification with the user.

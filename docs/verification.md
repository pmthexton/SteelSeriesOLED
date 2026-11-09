# Verification — 2026-11-09

- Go 1.27.3 Windows/amd64. `go test ./...`, `go vet ./...`, GUI-subsystem
  executable build passed after the current Steam/Epic/GOG update.
- Automated checks cover CSV NA/invalid fields, 2-second average/30-second lows,
  swap-chain isolation, batched-delivery gaps, stale readings/history reset,
  input latency vs present latency, OLED bit order/half boundaries, graph spike
  compression, game/helpers/path boundaries, clock movement and night blackout,
  CPU-temperature selection/staleness, capture error completion via a real
  subprocess, and explicit missing config failure.
- A separate PowerShell regression holds a sensor file open while publishing
  the next snapshot; atomic replacement retries and succeeds without partial JSON.
- Two independent code reviews found and prompted fixes for lost capture
  termination events, game-title false negatives, unclear labels and config
  failure behavior. Sensor worker joins before NVML shutdown.
- Hardware source-switch proof: Windows OLED present 17:09:46, absent after
  selecting Mac 17:10:11, present again 17:10:32. Read/write access succeeded.
- User observed actual OLED dashboard and graph. An early waiting-screen blink
  was traced to batch timing and Edge child-PID changes, then regression-tested.
- Edge video live capture held approximately 60 FPS across measured delivery
  gaps. NVIDIA GPU utilization/temperature, CPU utilization and AMD Tctl/Tdie
  temperature were read on this machine. Sensor snapshots continue after fixing
  a reader/file-replacement race.
- Old 7ms latency was present-to-display. User clarified input-to-display;
  the first input build used MsAllInputToPhotonLatency only. Edge supplied no valid samples
  during the live input test, so -- is expected there. At that stage, game availability was not
  claimed until verified with a supported running game.
- Installed executable: C:\Users\paul\bin\SteelSeriesOLED.exe. Startup shortcut
  targets it with no test arguments; app and PresentMon run non-elevated.
- Explicitly approved signed PawnIO driver and elevated sensor login task were
  installed. Task state was Running; CPU JSON output remained current. Provider
  code is in Program Files; there is no sensor network listener.
- Normal game detection finds Steam libraries on C: and D: plus registered GOG
  Quake II Enhanced. Epic manifests are supported, though none were found here.
- Sleep/resume, a fresh Windows login, and physical click-to-photon latency are
  not claimed tested. PC input/display telemetry does not include physical mouse
  switch/polling and panel scanout/pixel-response latency.

## Witcher 3 live validation and sparse-input fix

User confirmed FPS, graph and CPU/GPU metrics work in Witcher 3 DX12, detected
under the D: Steam library. Generic input latency blanked while holding WASD.
A separate 8-second non-elevated capture with --track_pc_latency yielded 982
frames: 969 instrumented PC-latency samples (first ~24.9ms), 99 generic input
samples and no click samples. This establishes PC-latency availability here.
Regression tests now cover source priority without mixing, sparse-sample hold,
explicit LAST state, ten-second expiry and clearing on game changes.

# Windows SteelSeries OLED

Approved conversation design, 2026-11-09. Standalone Go application; do not port
or modify the Swift app. Independent module staged in its own folder, with
the final executable installed into C:\Users\paul\bin and launched at login.

Use native Windows HID, preserving the Mac app's VID 1038 / PID 12e0 / usage
ffc0 selection and two report-6 bitmap transfers. Stop writing when the device
disappears; reacquire on return; restore factory UI on graceful termination.

Capture selected games with PresentMon's console CSV stream, not its overlay.
Use installed Steam/Epic/GOG game folders and explicit executable configuration;
exclude browsers, launchers and helper processes. Stop capture at game exit.
The user's Performance Log Users membership permits non-elevated capture.

Average FPS uses a 2-second history. 1% low uses the mean slowest 1% of frame
times in a 30-second rolling history. Sample per-frame; refresh numbers every
2 seconds. Show unavailable readings as -- and clear stale FPS after 3 seconds.
Choose the busiest swap chain, never mix chains. Clear history on game changes.

128x64 monochrome dashboard: FPS/1% low, CPU/GPU load and temperatures,
frametime graph over 10 seconds with per-column maximum (preserve spikes).
Redraw at 5Hz. Fixed labelled 33ms scale; clipped spikes reach the top. CPU/GPU
usage and temperature sit together on the left; input latency occupies a right
column, freeing 22 pixels of graph height. Shift layout periodically with margins.

LAT prefers instrumented MsPCLatency when supported, with a separate fallback
to MsAllInputToPhotonLatency. Never average those two sources together or
substitute present-to-display. Fresh values use a two-second average; sparse
samples retain their last measurement for at most ten seconds, marked LAST.
Physical peripheral/panel response is outside this software measurement.
When no supported game is running, show the Mac app's moving HH:MM clock;
retain its 21:00-07:00 dark mode with brief moving sleeping-cat cameos.
Pause capture/rendering when Windows does not own the base station.

GPU utilization/temperature: NVIDIA NVML. CPU load: GetSystemTimes deltas.
CPU temperature: user chose LibreHardwareMonitor. Use its library in a small
background helper writing a local JSON snapshot; no HTTP listener is started.
The user explicitly approved signed PawnIO driver installation and an elevated
login sensor task; the Go app remains non-elevated.
Never substitute ACPI thermal-zone readings for CPU package temperature.
Missing sensors are --, with a diagnostic message. Sensor helper installation
and any privileged startup registration must be explicit and reviewable.

Approaches considered: PresentMon CLI vs overlay/service (CLI chosen to avoid
the broken overlay and recurring elevation); LibreHardwareMonitor vs installed
HWiNFO (user chose LibreHardwareMonitor). Keep Go application non-elevated;
any hardware driver access belongs to the sensor provider.

## Sparse-input latency refinement

The Witcher 3 emitted instrumented MsPCLatency on 969/982 captured frames with
--track_pc_latency enabled, versus 99/982 generic input-event samples. Prefer
that PC-latency source when valid; otherwise use generic input-to-display.
Keep sources separate. If neither has a fresh two-second set, retain the latest
sample for at most ten seconds, explicitly labelled LAST. Clear it at game
change, stale capture or expiry. Do not substitute present-to-display latency.

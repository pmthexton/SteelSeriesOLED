# SteelSeriesOLED (Windows / Go)

Independent Windows application for the Arctis Nova Pro Wireless base station.
The original Swift app is unchanged. No Go dependencies or cgo are needed.

For a clean machine, follow the **[complete Windows setup guide](docs/setup.md)**,
including verified dependencies, Performance Log Users membership, PawnIO driver,
sensor helper, build, login startup and uninstall instructions.

## Display

- Games: average FPS (latest 2 seconds), 1% low (slowest 1% of frame intervals
  in the latest 30 seconds), CPU/GPU utilization and temperatures in Celsius.
- Scrolling 10-second frametime graph: a thin connected line, with no filled area
  below it. Maximum per time column retains short
  stutters. Vertical range is 0–33ms; larger spikes clip at the top.
  The graph refreshes at up to 5Hz; numbers refresh every 2 seconds.
- CPU/GPU usage and temperature sit together on the left; latency has its own
  column on the right. The taller graph occupies the bottom of the screen.
- `LAT …MS` prefers PresentMon's instrumented `MsPCLatency`, enabled with
  `--track_pc_latency`. This measures the PC input-to-display pipeline in games
  that emit the required instrumentation, rather than just present-to-display.
  Recent valid samples are averaged over the latest 2s of capture.
- Games without PC instrumentation fall back to the separate
  `MsAllInputToPhotonLatency` input-event measurement; the two sources are never
  averaged together. Neither source falls back to present-to-display latency.
- Sparse input measurements retain the most recent value for up to 10 seconds,
  labelled **LAST** rather than LAT. After expiry, or if no sample exists, the
  value is `--`. Holding a key need not generate new measurable input events.
- This is software-based latency, not physical mouse-switch-to-photon latency:
  peripheral switch/polling delay and physical panel response are not measured.
  It is not a promise to match NVIDIA overlay numbers exactly; its averaging
  window and supported instrumentation may differ.
- FPS represents frame submissions (`MsBetweenPresents`), not a claim about
  generated/displayed FPS. For frame-generation games these can differ.
- When the game process exits: reset capture history and show the moving clock.
  Clock and dashboard pixels shift periodically. The clock follows the Mac app:
  21:00–07:00 is mostly black, with a 10-second sleeping-cat cameo every 5 minutes.
- When switched to Mac: stop writing and stop capture. Reacquire when the OLED
  interface returns to Windows. Presence cannot distinguish switching away from
  unplugging/power-off; the user's source-switch test established this signal.
- A running game that temporarily stops presenting retains the dashboard
  layout, with `--` after stale data expires. This does not show a stale FPS.

PresentMon delivers batches approximately a second behind real time. Statistical
windows end at the latest captured frame, and expire after 3 seconds without
recent frame timestamps; short batch gaps do not switch to a waiting screen.
1% low waits for at least 100 frames. Longer history builds naturally at startup.

## Game detection

Auto-discover installed Steam game directories (including extra library drives),
Epic manifest install locations and GOG's registered installed-game paths.
Only processes within these directories qualify; common
launchers, browsers, crash reporters and anticheat helpers are excluded.
An explicit `games` list can add other games, including Game Pass/standalone
executables, or override an overly broad helper match. To use only an allowlist,
set `auto_discover` to false. Folder discovery is a heuristic: store-installed
non-game tools may need excluding by switching to an explicit allowlist.

The foreground qualifying game is preferred; otherwise keep the existing game.
Per-process capture is restricted to that game; separate swap chains are not
combined. The busiest recent swap chain supplies the dashboard.

`-test-process msedge.exe` temporarily permits Edge for hardware testing and
captures its child processes by name. This flag is never saved or used at login.

## Build and run

```powershell
& 'C:\Program Files\Go\bin\go.exe' test ./...
& 'C:\Program Files\Go\bin\go.exe' vet ./...
& 'C:\Program Files\Go\bin\go.exe' build -ldflags '-H=windowsgui' -o dist/SteelSeriesOLED.exe .
.\dist\SteelSeriesOLED.exe
.\dist\SteelSeriesOLED.exe -diagnose
.\dist\SteelSeriesOLED.exe -stop
```

The app uses a mutex to prevent duplicate instances. `-stop` requests graceful
shutdown and hands the OLED back to its factory UI. `-duration 30s` is useful
for bounded tests; `-dry-run` avoids OLED writes. `-preview dashboard.png`
creates a simulated PNG without hardware access.

Runtime log: `%LOCALAPPDATA%\SteelSeriesOLED\app.log` (2MiB startup rotation).
Configuration: `SteelSeriesOLED.json` beside the executable. An explicitly
specified `-config` file must exist. PresentMon is discovered beside the exe or
in `%USERPROFILE%\bin` unless its full path is configured.

Example:

```json
{
  "presentmon": "C:\\Users\\paul\\bin\\PresentMon-2.6.0-x64.exe",
  "auto_discover": true,
  "games": [],
  "game_roots": [],
  "gpu_index": 0,
  "lhm_file": "C:\\ProgramData\\SteelSeriesOLED\\sensors.json"
}
```

## Permissions and CPU temperature

The Go app and PresentMon run as the normal user. PresentMon requires membership
in Performance Log Users, followed by sign-out/in; this is already configured
on the user's computer. Its warning about identifying other users/short-lived
processes does not mean ordinary capture has failed.

GPU metrics use the installed NVIDIA NVML driver API; CPU utilization uses
Windows GetSystemTimes. CPU temperature is read from **LibreHardwareMonitor**.
The bundled-provider setup uses its official .NET Framework library, not its GUI
or HTTP server. A small PowerShell 5.1 helper polls only CPU hardware every 2s
and atomically writes a local JSON snapshot. Stale sensor files expire after 8s.
No sensor TCP/HTTP listener is started.

The user explicitly approved the one-time sensor setup: a signed PawnIO driver
installation and an elevated `SteelSeriesOLED Sensors` scheduled task. Provider
DLLs and script live under `%ProgramFiles%\SteelSeriesOLED-Sensors`; its output
is `%ProgramData%\SteelSeriesOLED\sensors.json`. Windows starts the provider
at login without recurring UAC. The display app is not elevated.

Official LibreHardwareMonitor v0.9.6 ZIP SHA-256:
`086d9f1b5a99e643edc2cfaaac16051685b551e4c5ac0b32a57c58c0e529c001`.
PawnIO installer is extracted unchanged from that verified release; its valid
Authenticode signer was checked as namazso. Some anticheat software can block
hardware sensor access; unavailable values remain `--`.

## Installation and removal

`install-user.ps1` copies the executable to `%USERPROFILE%\bin`, preserves an
existing config, and creates `SteelSeriesOLED.lnk` in the user's Startup folder.
No elevated task is used for the Go app. Sensor setup is separate in
`install-sensors-admin.ps1`, and requires one-time administrator approval.

To disable the app: run `SteelSeriesOLED.exe -stop` and delete the
`SteelSeriesOLED.lnk` Startup shortcut. To disable the CPU sensor helper, stop
and unregister the `SteelSeriesOLED Sensors` scheduled task from an elevated
PowerShell. Its Program Files directory can then be removed. PawnIO is a shared
driver dependency; uninstall via Windows Installed Apps only if no other tools
need it. No UAC settings or global PowerShell execution policy are changed.

## Sources

- PresentMon: https://github.com/GameTechDev/PresentMon
- NVML: https://docs.nvidia.com/deploy/nvml-api/latest/index.html
- LibreHardwareMonitor: https://github.com/LibreHardwareMonitor/LibreHardwareMonitor
- PawnIO: https://github.com/namazso/PawnIO
- HID format derived from the user's Mac `SteelSeriesClock.swift` / `ClockFrame.swift`.

LibreHardwareMonitor and its DLL dependencies are redistributed unchanged in the
local provider installation. Their licenses remain those of the upstream projects;
this setup is for this user's machine, not a public redistributable installer.

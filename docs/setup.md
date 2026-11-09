# Fresh Windows setup

These instructions reproduce the working setup from a clean clone. On the
original machine, group membership, driver, sensor task and startup are already
configured; an ordinary app update does not require doing them again.

## Prerequisites

- Windows x64. Tested on Windows 11 with a SteelSeries Arctis Nova Pro Wireless
  base (USB VID `1038`, PID `12e0`) and an NVIDIA RTX 4070 SUPER.
- Current NVIDIA driver including NVML. AMD/Intel GPU telemetry is not implemented.
- [Go 1.23 or later](https://go.dev/dl/) to build; tested with Go 1.27.3. No C
  compiler, cgo or third-party Go packages are required. Git is needed to clone.
- **Windows PowerShell 5.1** and **.NET Framework 4.7.2 or newer** for the CPU
  helper. Windows 11 includes a suitable Framework. PowerShell 7 (`pwsh`) and
  modern .NET 8/9 alone are not replacements for this .NET Framework package.
- One-time administrator access for group membership, the signed PawnIO driver
  and the sensor task. Normal app/PresentMon use does **not** require a
  high-integrity token or a UAC prompt at each login.

Run commands from the cloned project directory. `powershell.exe` means the
built-in Windows PowerShell 5.1. `-ExecutionPolicy Bypass` applies only to the
invoked process; these scripts do not change the global execution policy.

## 1. Download verified prerequisites

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\get-dependencies.ps1
```

The script downloads/verifies the official portable releases below:

| Release | Destination | SHA-256 |
| --- | --- | --- |
| [PresentMon 2.6.0 x64 console](https://github.com/GameTechDev/PresentMon/releases/tag/v2.6.0) | `%USERPROFILE%\bin\PresentMon-2.6.0-x64.exe` | `b2a706bc6ad475749e3b7e3409263aa1e6906d45bdcf993f6dbc0f660188f1af` |
| [LibreHardwareMonitor 0.9.6 .NET Framework ZIP](https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/releases/tag/v0.9.6) | `tools\LibreHardwareMonitor.zip` | `086d9f1b5a99e643edc2cfaaac16051685b551e4c5ac0b32a57c58c0e529c001` |

It extracts the full LHM ZIP and its bundled `PawnIO_setup.exe`, verifying the
installer's valid namazso Authenticode signature. It **does not install** a
kernel driver, task or startup entry. An existing file with an unexpected hash
causes an error instead of being overwritten. Downloads/build output are not
committed to Git. The PresentMon overlay/MSI and LHM GUI are not needed.

Optional custom directory: pass `-Bin 'D:\Tools'`; later use the same `-Bin`
with `install-user.ps1`. Keep the extracted LHM package together.

## 2. Let PresentMon capture without elevation

In a **normal** PowerShell terminal, get your everyday account identity:

```powershell
[Security.Principal.WindowsIdentity]::GetCurrent().Name
```

Copy the exact result, e.g. `MY-PC\paul`. Open **Windows PowerShell as
administrator**, then substitute that account below:

```powershell
Add-LocalGroupMember -SID 'S-1-5-32-559' -Member 'MY-PC\paul'
```

The SID identifies **Performance Log Users**, avoiding localized group names.
This works on Windows Home too; Home has no Local Users and Groups panel in
Computer Management. If already a member, no addition is needed. If UAC asks
for another administrator's credentials, still add your everyday account, not
that administrator.

**Sign out of Windows completely and sign back in.** Closing a terminal alone
does not refresh the logon token. Verify from a new normal terminal:

```powershell
whoami /groups | findstr S-1-5-32-559
```

Do not set PresentMon to Run as administrator. Its warning about identifying
other users/short-lived processes can remain; it does not mean ordinary game
capture has failed. Reference: [PresentMon permission troubleshooting](https://github.com/GameTechDev/PresentMon#user-access-denied).

## 3. Install the CPU temperature provider once

CPU usage comes from Windows; GPU readings come from NVIDIA's driver. CPU
**temperature** requires privileged hardware access. The provider combines:

1. LibreHardwareMonitor's .NET Framework library.
2. Its signed **PawnIO** kernel driver.
3. A small PowerShell helper, in the **SteelSeriesOLED Sensors** login task.

From the project folder in **Windows PowerShell as administrator**, run:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install-sensors-admin.ps1
```

The script copies the DLLs, `sensor-helper.ps1` and `sensor-file.ps1` into
`%ProgramFiles%\SteelSeriesOLED-Sensors`, checks the bundled PawnIO installer's
signature, installs the driver if absent, and registers/starts the sensor task.
The task runs at login with highest privileges under the installing user; it
writes `%ProgramData%\SteelSeriesOLED\sensors.json` every two seconds. It opens
no HTTP/TCP listener. The Go app only reads the local file.

Elevate under your everyday administrative account. If you use a separate admin
account, the script currently registers the task for that account; deliberately
configure the task's logon principal before relying on startup for another user.

Verify in the elevated terminal:

```powershell
Get-ScheduledTask -TaskName 'SteelSeriesOLED Sensors' | Select-Object TaskName, State
Get-Service -Name PawnIO
Get-Content "$env:ProgramData\SteelSeriesOLED\sensors.json"
```

The task should be Running; `Updated` should advance and temperature should be
nonzero and plausible. The driver/task require one-time approval; Task Scheduler
starts the configured helper without a new UAC prompt on every login.
**The Go app and PresentMon remain normal-user processes.**

If a security product/anticheat blocks sensor access, leave its protection on;
CPU temperature will show `--`. FPS, usage and the graph can work without CPU
temperature. [PawnIO driver source](https://github.com/namazso/PawnIO).

## 4. Build and verify

In a normal terminal with Go on PATH:

```powershell
go test ./...
go vet ./...
go build -ldflags '-H=windowsgui' -o dist/SteelSeriesOLED.exe .
```

If Go is not on PATH, replace `go` with
`& 'C:\Program Files\Go\bin\go.exe'`, or reopen the terminal after installing Go.
The GUI-subsystem build runs without a console window.

Optional sensor-file concurrency check:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\tests\sensor-file.tests.ps1
```

The original read-only USB probe is in `probe\`, with a separate Go module.
Build/run it from that directory when diagnosing source switching.

## 5. Install app and login startup

In a **normal**, non-elevated terminal:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install-user.ps1
Start-Process "$env:USERPROFILE\bin\SteelSeriesOLED.exe" -WorkingDirectory "$env:USERPROFILE\bin"
```

This copies `dist\SteelSeriesOLED.exe` to user bin, creates its JSON config if
absent, and creates `SteelSeriesOLED.lnk` in your Startup folder. Existing config
is preserved. No elevated task is used for this app. The shortcut has no browser
test arguments. It starts on the next login; Start-Process above runs it now.
A mutex prevents duplicate instances.

Config: `%USERPROFILE%\bin\SteelSeriesOLED.json`.
Log: `%LOCALAPPDATA%\SteelSeriesOLED\app.log`.

## 6. Test the complete setup

- Select Windows USB input. With no game running, expect a moving HH:MM clock.
  At 21:00-07:00 it is intentionally mostly black, with a brief cat cameo every
  five minutes; darkness during those hours is not proof of failure.
- Launch an installed Steam, Epic or GOG game. Expect FPS/1% low, CPU/GPU values
  and the scrolling graph. Add other games through `games` or `game_roots` in the
  config. Restart after editing config or installing a new game to refresh discovery.
- `LAT` prefers instrumented PC latency, else generic input-to-display samples.
  `LAST` marks a sparse retained reading for up to ten seconds; `--` is unavailable.
  Present-to-display is never substituted. Physical peripheral/panel delays are
  outside this software measurement.
- Close the game: the moving clock returns and capture history resets.
- Switch to Mac: Windows stops writing; switching back reconnects. Unplugging or
  powering off also makes the USB interface disappear.

For diagnostics, explicitly wait for the GUI executable and capture its JSON:

```powershell
Start-Process "$env:USERPROFILE\bin\SteelSeriesOLED.exe" -ArgumentList '-diagnose' -Wait -NoNewWindow -RedirectStandardOutput "$env:TEMP\SteelSeriesOLED-diagnostics.json"
Get-Content "$env:TEMP\SteelSeriesOLED-diagnostics.json"
```

Check game roots, the OLED collection, PresentMon path and CPU/GPU errors.
`-diagnose` does not send display reports.

## Updating, stopping and uninstalling

Before replacing a running app, stop it and wait for its executable to release:

```powershell
$bin = "$env:USERPROFILE\bin"
Start-Process "$bin\SteelSeriesOLED.exe" -ArgumentList '-stop' -Wait -WindowStyle Hidden
Get-Process SteelSeriesOLED -ErrorAction SilentlyContinue | Wait-Process -Timeout 10
```

Rebuild, rerun `install-user.ps1`, then start normally. For helper script-only
updates, run `update-sensors-admin.ps1` elevated. For dependency upgrades, stop
the sensor task before rerunning full sensor setup.

To uninstall:

1. Stop the Go app with `-stop`; graceful shutdown restores the factory OLED UI.
2. Delete `SteelSeriesOLED.lnk` from `shell:startup` to disable app login launch.
3. Remove the sensor task in an elevated terminal:

   ```powershell
   Stop-ScheduledTask -TaskName 'SteelSeriesOLED Sensors'
   Unregister-ScheduledTask -TaskName 'SteelSeriesOLED Sensors' -Confirm:$false
   ```

4. With the task stopped, remove `%ProgramFiles%\SteelSeriesOLED-Sensors` and,
   if desired, `%ProgramData%\SteelSeriesOLED` and the app/config in user bin.
5. PawnIO is a shared driver. Uninstall through Windows Installed Apps only if
   no other monitoring tools need it. Removing the Go app does not remove it.
6. If no other tool needs Performance Log Users membership, remove the everyday
   account from the group in an elevated terminal, then sign out/in:

   ```powershell
   Remove-LocalGroupMember -SID 'S-1-5-32-559' -Member 'MY-PC\paul'
   ```

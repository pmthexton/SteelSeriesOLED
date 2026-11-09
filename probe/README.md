# SteelSeries Windows HID proof (throwaway spike)

Independent Go module; no Swift changes, third-party Go packages, cgo, or USB
reports. This folder can be moved out of the Mac repository unchanged. This is
the input-detection experiment, not the performance-display application.

## Run

```powershell
& 'C:/Program Files/Go/bin/go.exe' build -o oled-probe.exe .
.\oled-probe.exe -duration 2m | Tee-Object -FilePath input-switch.jsonl
```

Run from this directory. Without `-duration`, Ctrl+C stops the probe. It polls
every 500 ms and prints JSON only when descriptors, access, or presence changes.
In the Codex sandbox, device handles returned access denied; running the same
executable outside the sandbox succeeded. Elevation to administrator was not
tested or established as necessary.

## Observed hardware proof, 2026-11-09 (London / UTC)

The user started on Windows, selected Mac, then returned to Windows:

| Time | Observation |
| --- | --- |
| 17:09:46 | OLED present; three SteelSeries HID collections |
| 17:10:11 | OLED absent; no SteelSeries HID collections |
| 17:10:32 | OLED present again; same descriptors; read/write open succeeded |

The matching collection has VID `1038`, PID `12e0`, usage page `ffc0`,
FeatureReportByteLength `1024`, OutputReportByteLength `64`.

This supports using Windows HID arrival/removal as the ownership signal for
this base station and firmware, just as the existing Mac code does. It is not
an explicit active-source query: unplugging or powering off the base also makes
it absent. Enumeration/access errors are reported separately from absence.
One source-switch cycle was tested; sleep/resume and display writes were not.

## Display contract from the Mac source

`Sources/DevToolkitApp/SteelSeriesClock.swift` selects the collection above and
uses attach/detach callbacks; it contains no active-input query.
`Sources/DevToolkitApp/ClockFrame.swift` encodes a 128 x 64 monochrome frame:

- Two 1,024-byte HID feature reports, report ID `6`.
- Header `[6, 0x93, x, 0, 64, 64]`, with `x` equal to 0 and 64.
- Payload bit index `column * 64 + row`, least significant bit first.
- Factory UI restore: 64-byte output report beginning `[6, 0x95]`.

Windows feature buffers include the report ID as byte zero and use the length
from HIDP_CAPS. Do not prepend a second report-ID byte.

## Metrics feasibility and proposed next step

`nvidia-smi --query-gpu=name,utilization.gpu,temperature.gpu --format=csv,noheader,nounits`
returned `NVIDIA GeForce RTX 4070 SUPER, 7, 47.22` on this computer. This confirms
GPU utilization and temperature are available from the installed NVIDIA driver.
An eventual app can use NVML directly to avoid launching a process per sample.

Proposed providers (not implemented):

- FPS: PresentMon frame timing, per game/process and swap chain. Keep samples
  separate between games, clear stale data when a game closes, and explicitly
  choose application-rendered versus displayed frames for frame generation.
- CPU utilization: Windows GetSystemTimes deltas.
- GPU utilization/temperature: NVIDIA NVML (nvidia-smi fallback).
- CPU package temperature: LibreHardwareMonitor, or the installed HWiNFO via
  shared memory if the user prefers it. Availability and sensor choice still
  need testing; unavailable temperatures must display `--`, never zero.

Suggested FPS definition for review: rolling 30-second window; average FPS is
1,000 divided by mean frame time in milliseconds; 1% low is 1,000 divided by
the mean of the slowest 1% of frame times. Label warmup/insufficient samples.
This definition is a proposal, not an assertion about NVIDIA's overlay.

No documented live export API for the NVIDIA overlay or Zotac FireStorm was
established during research. Avoid depending on undocumented process memory.

References:

- https://github.com/GameTechDev/PresentMon
- https://docs.nvidia.com/deploy/nvml-api/latest/index.html
- https://github.com/LibreHardwareMonitor/LibreHardwareMonitor
- https://www.hwinfo.com/forum/threads/shared-memory-support-setting-becomes-unchecked-after-a-few-hours.7187/
- https://learn.microsoft.com/en-us/windows-hardware/drivers/ddi/hidsdi/nf-hidsdi-hidd_setfeature

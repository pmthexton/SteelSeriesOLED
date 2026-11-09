$ErrorActionPreference = 'Stop'
$target = Join-Path $env:ProgramFiles 'SteelSeriesOLED-Sensors'
Stop-ScheduledTask -TaskName 'SteelSeriesOLED Sensors'
Copy-Item -LiteralPath "$PSScriptRoot\sensor-helper.ps1" -Destination $target -Force
Copy-Item -LiteralPath "$PSScriptRoot\sensor-file.ps1" -Destination $target -Force
Start-ScheduledTask -TaskName 'SteelSeriesOLED Sensors'

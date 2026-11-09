# One-time elevated setup. Installs only the verified LHM sensor provider and its
# signed PawnIO dependency. The Go application remains a standard-user process.
param([string]$Source = "$PSScriptRoot\tools\LibreHardwareMonitor")
$ErrorActionPreference = 'Stop'
if (-not ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'Run this one-time setup as administrator.'
}
$target = Join-Path $env:ProgramFiles 'SteelSeriesOLED-Sensors'
$data = Join-Path $env:ProgramData 'SteelSeriesOLED'
New-Item -ItemType Directory -Path $target, $data -Force | Out-Null
Copy-Item -LiteralPath "$PSScriptRoot\sensor-helper.ps1" -Destination $target -Force
Copy-Item -LiteralPath "$PSScriptRoot\sensor-file.ps1" -Destination $target -Force
Get-ChildItem -LiteralPath $Source -File | Where-Object Extension -eq '.dll' | Copy-Item -Destination $target -Force
$installer = "$PSScriptRoot\tools\PawnIO_setup.exe"
if (-not (Test-Path 'HKLM:\SYSTEM\CurrentControlSet\Services\PawnIO')) {
    $signature = Get-AuthenticodeSignature -LiteralPath $installer
    if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notlike '*namazso*') { throw 'PawnIO signature verification failed.' }
    $process = Start-Process -FilePath $installer -ArgumentList '-install', '-silent' -WindowStyle Hidden -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "PawnIO installer returned $($process.ExitCode)." }
}
$user = [Security.Principal.WindowsIdentity]::GetCurrent().Name
$powershell = "$env:WINDIR\System32\WindowsPowerShell\v1.0\powershell.exe"
$arguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -WindowStyle Hidden -File "' + $target + '\sensor-helper.ps1"'
$action = New-ScheduledTaskAction -Execute $powershell -Argument $arguments -WorkingDirectory $target
$trigger = New-ScheduledTaskTrigger -AtLogOn -User $user
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType Interactive -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1)
Register-ScheduledTask -TaskName 'SteelSeriesOLED Sensors' -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Description 'LibreHardwareMonitor CPU temperature reader; writes local sensor snapshot, no network listener.' -Force | Out-Null
Start-ScheduledTask -TaskName 'SteelSeriesOLED Sensors'
'Sensor provider installed. No recurring UAC prompt is needed.' | Set-Content -LiteralPath "$data\install-result.txt"

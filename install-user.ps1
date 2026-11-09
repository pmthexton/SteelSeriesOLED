param([string]$Bin = "$env:USERPROFILE\bin", [string]$SourceExecutable = "$PSScriptRoot\dist\SteelSeriesOLED.exe")
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Path $Bin -Force | Out-Null
$targetExe = Join-Path $Bin 'SteelSeriesOLED.exe'
Copy-Item -LiteralPath $SourceExecutable -Destination $targetExe -Force
$config = Join-Path $Bin 'SteelSeriesOLED.json'
if (-not (Test-Path -LiteralPath $config)) {
    $presentmon = Get-ChildItem -LiteralPath $Bin -Filter 'PresentMon*-x64.exe' | Sort-Object Name | Select-Object -Last 1
    if ($null -eq $presentmon) { throw 'PresentMon portable binary not found in bin.' }
    @{ presentmon = $presentmon.FullName; auto_discover = $true; games = @(); game_roots = @(); gpu_index = 0; lhm_file = "$env:ProgramData\SteelSeriesOLED\sensors.json" } | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath $config -Encoding ASCII
}
$startup = [Environment]::GetFolderPath('Startup')
$shell = New-Object -ComObject WScript.Shell
$shortcut = $shell.CreateShortcut((Join-Path $startup 'SteelSeriesOLED.lnk'))
$shortcut.TargetPath = $targetExe
$shortcut.WorkingDirectory = $Bin
$shortcut.Description = 'SteelSeries OLED game dashboard and idle clock'
$shortcut.WindowStyle = 7
$shortcut.Save()
Copy-Item -LiteralPath "$PSScriptRoot\README.md" -Destination (Join-Path $Bin 'SteelSeriesOLED-README.md') -Force
Write-Output "Installed $targetExe and standard-user login shortcut."

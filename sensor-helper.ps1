param(
    [string]$Library = "$PSScriptRoot\LibreHardwareMonitorLib.dll",
    [string]$Output = "$env:ProgramData\SteelSeriesOLED\sensors.json",
    [int]$Samples = 0
)
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\sensor-file.ps1"
# Run with Windows PowerShell 5.1: this official LHM build targets .NET Framework.
$mutex = New-Object System.Threading.Mutex($false, 'Local\SteelSeriesOLED-Sensors')
if (-not $mutex.WaitOne(0)) { $mutex.Dispose(); exit 0 }
$computer = $null
try {
    Add-Type -Path $Library
    $computer = New-Object LibreHardwareMonitor.Hardware.Computer
    $computer.IsCpuEnabled = $true
    $computer.Open()
    $directory = Split-Path -Parent $Output
    if (-not (Test-Path -LiteralPath $directory)) { New-Item -ItemType Directory -Path $directory -Force | Out-Null }
    $iteration = 0
    while ($Samples -eq 0 -or $iteration -lt $Samples) {
        $nodes = @()
        foreach ($hardware in $computer.Hardware) {
            $hardware.Update()
            foreach ($sensor in $hardware.Sensors) {
                if ($sensor.SensorType -eq 'Temperature' -and $null -ne $sensor.Value) {
                    $nodes += @{ Text = $sensor.Name; SensorId = $sensor.Identifier.ToString(); Value = ([double]$sensor.Value).ToString('F2', [Globalization.CultureInfo]::InvariantCulture) + ' C' }
                }
            }
        }
        $snapshot = @{ Updated = [DateTime]::UtcNow.ToString('o'); Children = @($nodes) } | ConvertTo-Json -Depth 8 -Compress
        Write-SensorSnapshot -Path $Output -Json $snapshot
        $iteration++
        if ($Samples -eq 0 -or $iteration -lt $Samples) { Start-Sleep -Seconds 2 }
    }
} catch {
    $_ | Out-String | Set-Content -LiteralPath ($Output + '.error.log')
    throw
} finally {
    if ($null -ne $computer) { $computer.Close() }
    $mutex.ReleaseMutex()
    $mutex.Dispose()
}

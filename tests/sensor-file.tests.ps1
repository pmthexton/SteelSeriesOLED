$ErrorActionPreference = 'Stop'
. "$PSScriptRoot\..\sensor-file.ps1"
$folder = Join-Path $env:TEMP ([Guid]::NewGuid().ToString())
New-Item -ItemType Directory -Path $folder | Out-Null
$path = Join-Path $folder 'sensors.json'
Write-SensorSnapshot -Path $path -Json '{"value":1}'
Add-Type -TypeDefinition @'
using System;
using System.Threading;
public static class SensorReaderLockTest {
  public static void ReleaseLater(IDisposable handle) {
    ThreadPool.QueueUserWorkItem(_ => { Thread.Sleep(250); handle.Dispose(); });
  }
}
'@
$reader = New-Object IO.FileStream($path, [IO.FileMode]::Open, [IO.FileAccess]::Read, [IO.FileShare]::ReadWrite)
[SensorReaderLockTest]::ReleaseLater($reader)
Write-SensorSnapshot -Path $path -Json '{"value":2}'
if ([IO.File]::ReadAllText($path) -ne '{"value":2}') { throw 'Concurrent reader prevented a complete new snapshot.' }
'PASS: snapshot publisher survives a concurrent reader and replaces JSON atomically.'

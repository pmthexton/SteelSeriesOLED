function Write-SensorSnapshot {
    param([string]$Path, [string]$Json)
    [IO.File]::WriteAllText($Path + '.tmp', $Json, (New-Object Text.UTF8Encoding($false)))
    # Go may briefly hold the old file without FILE_SHARE_DELETE. Retry atomic
    # replacement rather than exiting the sensor provider on a transient reader.
    for ($attempt = 0; $attempt -lt 50; $attempt++) {
        try {
            if ([IO.File]::Exists($Path)) { [IO.File]::Replace($Path + '.tmp', $Path, [NullString]::Value) }
            else { [IO.File]::Move($Path + '.tmp', $Path) }
            return
        } catch [IO.IOException] {
            if ($attempt -eq 49) { throw }
            Start-Sleep -Milliseconds 20
        }
    }
}

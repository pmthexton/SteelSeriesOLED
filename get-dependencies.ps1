param([string]$Bin = "$env:USERPROFILE\bin")
$ErrorActionPreference = 'Stop'
if ($PSVersionTable.PSEdition -ne 'Desktop') { throw 'Run with Windows PowerShell 5.1 (powershell.exe), not pwsh.exe.' }
$tools = Join-Path $PSScriptRoot 'tools'
New-Item -ItemType Directory -Path $tools, $Bin -Force | Out-Null
function Get-VerifiedDependency {
    param([string]$Uri, [string]$Path, [string]$SHA256)
    if (Test-Path -LiteralPath $Path) {
        if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $SHA256) { throw "Existing file has unexpected hash; move it aside before retrying: $Path" }
    } else {
        Invoke-WebRequest -UseBasicParsing -Uri $Uri -OutFile ($Path + '.download')
        if ((Get-FileHash -LiteralPath ($Path + '.download') -Algorithm SHA256).Hash -ne $SHA256) { throw "Download hash mismatch: $Uri" }
        Move-Item -LiteralPath ($Path + '.download') -Destination $Path
    }
    Write-Host "Verified $Path"
}
Get-VerifiedDependency -Uri 'https://github.com/GameTechDev/PresentMon/releases/download/v2.6.0/PresentMon-2.6.0-x64.exe' -Path (Join-Path $Bin 'PresentMon-2.6.0-x64.exe') -SHA256 'B2A706BC6AD475749E3B7E3409263AA1E6906D45BDCF993F6DBC0F660188F1AF'
$archive = Join-Path $tools 'LibreHardwareMonitor.zip'
Get-VerifiedDependency -Uri 'https://github.com/LibreHardwareMonitor/LibreHardwareMonitor/releases/download/v0.9.6/LibreHardwareMonitor.zip' -Path $archive -SHA256 '086D9F1B5A99E643EDC2CFAAAC16051685B551E4C5AC0B32A57C58C0E529C001'
$library = Join-Path $tools 'LibreHardwareMonitor'
Expand-Archive -LiteralPath $archive -DestinationPath $library -Force
$assembly = [Reflection.Assembly]::LoadFile((Join-Path $library 'LibreHardwareMonitor.exe'))
$inputStream = $assembly.GetManifestResourceStream('LibreHardwareMonitor.Resources.PawnIO_setup.exe')
if ($null -eq $inputStream) { throw 'Verified release is missing its PawnIO installer.' }
$installer = Join-Path $tools 'PawnIO_setup.exe'
$outputStream = [IO.File]::Create($installer)
try { $inputStream.CopyTo($outputStream) } finally { $outputStream.Dispose(); $inputStream.Dispose() }
$signature = Get-AuthenticodeSignature -LiteralPath $installer
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notlike '*namazso*') { throw 'PawnIO Authenticode verification failed.' }
Write-Host 'Dependencies ready. No driver, service, task or startup entry was installed.'

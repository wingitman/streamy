param([string]$InstallDir = "")
$ErrorActionPreference = "Stop"
$name = "streamy"
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
$resolved = (Get-Command $name -ErrorAction SilentlyContinue).Source
if ([string]::IsNullOrWhiteSpace($InstallDir)) { if ($resolved) { $InstallDir=Split-Path -Parent $resolved } else { $InstallDir=Join-Path $env:USERPROFILE ".local\bin" } }
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
$tmp=Join-Path $InstallDir ".$name.install.$PID.exe"
try {
  if (Get-Command go -ErrorAction SilentlyContinue) { Write-Host "Building $name from source"; Push-Location $root; try { go build -o $tmp ./cmd/streamy } finally { Pop-Location } }
  else { $artifact=Join-Path $root "releases\$name-windows-amd64.exe"; if (!(Test-Path $artifact)) { throw "Matching release artifact not found: $artifact" }; Copy-Item $artifact $tmp }
  Move-Item -Force $tmp (Join-Path $InstallDir "$name.exe")
  if (!(Test-Path (Join-Path $InstallDir "$name.exe"))) { throw "Installation verification failed" }
} finally { Remove-Item -Force -ErrorAction SilentlyContinue $tmp }
Write-Host "Installed $name to $InstallDir\$name.exe"
$after = (Get-Command $name -ErrorAction SilentlyContinue).Source
if (!$after) { Write-Warning "Add $InstallDir to PATH" }
elseif ((Resolve-Path $after).Path -ne (Resolve-Path (Join-Path $InstallDir "$name.exe")).Path) { Write-Warning "PATH resolves $after, not the newly installed command" }

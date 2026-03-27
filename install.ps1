# DBF-SYNC Installer for Windows
# Usage: irm https://raw.githubusercontent.com/Victor0451/dbf-sync/main/install.ps1 | iex

$ErrorActionPreference = "Stop"

$repo    = "Victor0451/dbf-sync"
$appName = "dbf-sync"
$installDir = "$env:LOCALAPPDATA\dbf-sync"

Write-Host ""
Write-Host "  DBF-SYNC Installer" -ForegroundColor Cyan
Write-Host "  ─────────────────────────────────────" -ForegroundColor DarkCyan
Write-Host ""

# Get latest release info from GitHub
Write-Host "  Buscando ultima version..." -ForegroundColor Gray
try {
    $release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
} catch {
    Write-Host "  ERROR: No se pudo conectar a GitHub." -ForegroundColor Red
    exit 1
}

$version = $release.tag_name
$asset   = $release.assets | Where-Object { $_.name -like "*windows*amd64*.exe" } | Select-Object -First 1

if (-not $asset) {
    Write-Host "  ERROR: No se encontro binario para Windows en la release $version." -ForegroundColor Red
    exit 1
}

Write-Host "  Version: $version" -ForegroundColor Green
Write-Host "  Destino: $installDir" -ForegroundColor Gray
Write-Host ""

# Create install directory
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir | Out-Null
}

# Download binary
$destPath = "$installDir\$appName.exe"
Write-Host "  Descargando $($asset.name)..." -ForegroundColor Gray

try {
    Invoke-WebRequest -Uri $asset.browser_download_url -OutFile $destPath -UseBasicParsing
} catch {
    Write-Host "  ERROR: Fallo la descarga." -ForegroundColor Red
    exit 1
}

Write-Host "  Descarga completa." -ForegroundColor Green

# Add to PATH if not already there
$userPath = [Environment]::GetEnvironmentVariable("PATH", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("PATH", "$userPath;$installDir", "User")
    Write-Host "  PATH actualizado." -ForegroundColor Green
} else {
    Write-Host "  PATH ya configurado." -ForegroundColor Gray
}

# Create dbf-sync.bat wrapper so it works in CMD and PowerShell without .exe
$batPath = "$installDir\$appName.bat"
@"
@echo off
"%~dp0dbf-sync.exe" %*
"@ | Set-Content $batPath

Write-Host ""
Write-Host "  ─────────────────────────────────────" -ForegroundColor DarkCyan
Write-Host "  Instalacion completada!" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Reinicia tu terminal y ejecuta:" -ForegroundColor White
Write-Host "    dbf-sync interactive" -ForegroundColor Yellow
Write-Host ""

# build_windows.ps1 — Build Coral for Windows and create MSI installer
#
# Prerequisites:
#   - Go 1.23+ in PATH
#   - WiX Toolset v4+ (dotnet tool install --global wix)
#
# Usage:
#   .\scripts\build_windows.ps1              # build + package
#   .\scripts\build_windows.ps1 -SkipMSI     # build only

param(
    [switch]$SkipMSI,
    [string]$Version = "1.0.0"
)

$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent (Split-Path -Parent $PSCommandPath)
$CoralGoDir = Join-Path $RepoRoot "coral-go"
$BuildDir = Join-Path $RepoRoot "build\windows"
$BinDir = Join-Path $BuildDir "bin"

Write-Host "=== Building Coral $Version for Windows ===" -ForegroundColor Cyan

# Clean build directory
if (Test-Path $BuildDir) { Remove-Item -Recurse -Force $BuildDir }
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null

# Build the Go binary
Write-Host "Building coral.exe..." -ForegroundColor Yellow
Push-Location $CoralGoDir
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CGO_ENABLED = "0"

go build -ldflags "-s -w -X main.version=$Version" -o (Join-Path $BinDir "coral.exe") ./cmd/coral/
if ($LASTEXITCODE -ne 0) { throw "Go build failed" }

Pop-Location
Write-Host "Binary built: $BinDir\coral.exe" -ForegroundColor Green

# Copy icon if available
$IconSrc = Join-Path $RepoRoot "icons\coral.ico"
if (Test-Path $IconSrc) {
    Copy-Item $IconSrc $BuildDir
}

if ($SkipMSI) {
    Write-Host "Skipping MSI (use -SkipMSI:$false to build installer)" -ForegroundColor Yellow
    exit 0
}

# Build MSI with WiX
Write-Host "Building MSI installer..." -ForegroundColor Yellow
$WixSrc = Join-Path $RepoRoot "scripts\coral.wxs"

if (-not (Test-Path $WixSrc)) {
    Write-Host "WiX source not found at $WixSrc — skipping MSI" -ForegroundColor Yellow
    exit 0
}

$MsiPath = Join-Path $BuildDir "Coral-$Version-x64.msi"

wix build $WixSrc `
    -d "Version=$Version" `
    -d "BinDir=$BinDir" `
    -d "BuildDir=$BuildDir" `
    -o $MsiPath

if ($LASTEXITCODE -ne 0) { throw "WiX build failed" }

Write-Host "Installer built: $MsiPath" -ForegroundColor Green
Write-Host "=== Done ===" -ForegroundColor Cyan

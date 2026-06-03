[CmdletBinding()]
param(
    [string]$OutputDir = "dist\windows-amd64",
    [switch]$SkipTests
)

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$RepoRoot = Split-Path -Parent $ScriptDir
Set-Location $RepoRoot

function Add-PathPrefix {
    param([string]$PathToAdd)

    if ($PathToAdd -and (Test-Path $PathToAdd)) {
        $env:Path = "$PathToAdd;$env:Path"
    }
}

$msysCandidates = @()
if ($env:MSYS2_ROOT) {
    $msysCandidates += $env:MSYS2_ROOT
}
$msysCandidates += @("C:\msys64", "C:\tools\msys64")

$msysRoot = $null
foreach ($candidate in $msysCandidates) {
    if ($candidate -and (Test-Path (Join-Path $candidate "mingw64\bin\gcc.exe"))) {
        $msysRoot = $candidate
        break
    }
}

if (-not $msysRoot) {
    throw @"
MSYS2 mingw64 gcc was not found.

Install MSYS2, then run:
  C:\msys64\usr\bin\bash.exe -lc "pacman -S --needed --noconfirm mingw-w64-x86_64-gcc mingw-w64-x86_64-pkgconf mingw-w64-x86_64-opus"

If MSYS2 is installed somewhere else, set MSYS2_ROOT before running this script.
"@
}

Add-PathPrefix (Join-Path $msysRoot "mingw64\bin")
Add-PathPrefix (Join-Path $msysRoot "usr\bin")

$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:PKG_CONFIG_PATH = Join-Path $msysRoot "mingw64\lib\pkgconfig"

foreach ($tool in @("go", "gcc", "pkg-config")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool was not found on PATH"
    }
}

& pkg-config --exists opus
if ($LASTEXITCODE -ne 0) {
    throw @"
pkg-config cannot find opus.

Install it with:
  C:\msys64\usr\bin\bash.exe -lc "pacman -S --needed --noconfirm mingw-w64-x86_64-opus"
"@
}

if (-not $SkipTests) {
    & go test ./...
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed"
    }
}

$out = Join-Path $RepoRoot $OutputDir
New-Item -ItemType Directory -Force -Path $out | Out-Null

$appExe = Join-Path $out "lylatlink.exe"
$consoleExe = Join-Path $out "lylatlink-console.exe"
$launcher = Join-Path $out "Start-LylatLink-Tray.cmd"
$mingwBin = Join-Path $msysRoot "mingw64\bin"

& go build -trimpath -ldflags "-s -w" -o $consoleExe ./cmd/lylatlink
if ($LASTEXITCODE -ne 0) {
    throw "console build failed"
}

& go build -trimpath -ldflags "-s -w -H=windowsgui" -o $appExe ./cmd/lylatlink
if ($LASTEXITCODE -ne 0) {
    throw "app build failed"
}

foreach ($dll in @("libopus-0.dll", "libgcc_s_seh-1.dll", "libwinpthread-1.dll")) {
    $source = Join-Path $mingwBin $dll
    if (Test-Path $source) {
        Copy-Item -Force $source (Join-Path $out $dll)
    }
}

@"
@echo off
start "" "%~dp0lylatlink.exe"
"@ | Set-Content -Encoding ASCII $launcher

Write-Host "Built:"
Write-Host "  $appExe"
Write-Host "  $consoleExe"
Write-Host "  $launcher"

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

function Get-MsysRootFromTool {
    param([string]$Tool)

    $cmd = Get-Command $Tool -ErrorAction SilentlyContinue
    if (-not $cmd) {
        return $null
    }

    $bin = Split-Path -Parent $cmd.Source
    $parent = Split-Path -Parent $bin
    if ((Split-Path -Leaf $bin) -eq "bin" -and (Split-Path -Leaf $parent) -in @("mingw64", "ucrt64", "clang64")) {
        return Split-Path -Parent $parent
    }
    return $null
}

$msysCandidates = @()
if ($env:MSYS2_ROOT) {
    $msysCandidates += $env:MSYS2_ROOT
}
$pathRoot = Get-MsysRootFromTool "gcc"
if ($pathRoot) {
    $msysCandidates += $pathRoot
}
$pathRoot = Get-MsysRootFromTool "pkg-config"
if ($pathRoot) {
    $msysCandidates += $pathRoot
}
$msysCandidates += @("C:\msys64", "C:\tools\msys64")

$msysRoot = $null
$mingwDir = $null
foreach ($candidate in $msysCandidates) {
    foreach ($subdir in @("mingw64", "ucrt64", "clang64")) {
        if ($candidate -and (Test-Path (Join-Path $candidate "$subdir\bin\gcc.exe"))) {
            $msysRoot = $candidate
            $mingwDir = $subdir
            break
        }
    }
    if ($msysRoot) {
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

Add-PathPrefix (Join-Path $msysRoot "$mingwDir\bin")
Add-PathPrefix (Join-Path $msysRoot "usr\bin")

$mingwBin = Join-Path $msysRoot "$mingwDir\bin"
$gccExe = Join-Path $mingwBin "gcc.exe"
$pkgConfigCandidates = @(
    (Join-Path $mingwBin "pkg-config.exe"),
    (Join-Path $mingwBin "pkgconf.exe"),
    (Join-Path $mingwBin "x86_64-w64-mingw32-pkg-config.exe"),
    (Join-Path $mingwBin "x86_64-w64-mingw32-pkgconf.exe"),
    (Join-Path $msysRoot "usr\bin\pkg-config.exe"),
    (Join-Path $msysRoot "usr\bin\pkgconf.exe")
)

$pkgConfigExe = $null
foreach ($candidate in $pkgConfigCandidates) {
    if (Test-Path $candidate) {
        $pkgConfigExe = $candidate
        break
    }
}

if (-not (Test-Path $gccExe)) {
    throw "gcc was not found at $gccExe"
}
if (-not $pkgConfigExe) {
    throw "pkg-config/pkgconf was not found in $mingwBin or $msysRoot\usr\bin"
}

$opusPc = Get-ChildItem -Path $msysRoot -Filter "opus.pc" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $opusPc) {
    throw @"
opus.pc was not found under $msysRoot.

Install it with:
  C:\msys64\usr\bin\bash.exe -lc "pacman -S --needed --noconfirm mingw-w64-x86_64-opus"
"@
}

$env:CGO_ENABLED = "1"
$env:GOOS = "windows"
$env:GOARCH = "amd64"
$env:CC = $gccExe
$env:PKG_CONFIG = $pkgConfigExe
$env:PKG_CONFIG_PATH = Split-Path -Parent $opusPc.FullName

foreach ($tool in @("go")) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        throw "$tool was not found on PATH"
    }
}

Write-Host "MSYS2 root: $msysRoot"
Write-Host "MSYS2 env: $mingwDir"
Write-Host "CC: $env:CC"
Write-Host "PKG_CONFIG: $env:PKG_CONFIG"
Write-Host "PKG_CONFIG_PATH: $env:PKG_CONFIG_PATH"

& $pkgConfigExe --exists opus
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

Write-Host "Built:"
Write-Host "  $appExe"
Write-Host "  $consoleExe"

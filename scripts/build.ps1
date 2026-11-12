# Builds single, standalone, dependency-free executables of DigitalSanDook for
# every supported platform into .\dist. Requires only the Go toolchain (1.25+).
#   usage:  powershell -ExecutionPolicy Bypass -File scripts\build.ps1 [-Version 1.2.3] [-SkipTests]
param([string]$Version = "dev", [switch]$SkipTests)

$ErrorActionPreference = "Stop"
$repo = Split-Path $PSScriptRoot -Parent
Set-Location $repo
$out = Join-Path $repo "dist"
$pkg = "./cmd/digitalsandook"

if (-not $SkipTests) {
  Write-Host "go vet + go test ..."
  & go vet ./...
  if ($LASTEXITCODE -ne 0) { throw "go vet failed" }
  & go test ./...
  if ($LASTEXITCODE -ne 0) { throw "tests failed" }
}

if (Test-Path $out) { Get-ChildItem $out -File | Where-Object { $_.Name -like "DigitalSanDook-*" -or $_.Name -eq "SHA256SUMS.txt" } | Remove-Item -Force }
New-Item -ItemType Directory -Force $out | Out-Null

# os, arch, GOARM, output suffix
$targets = @(
  @("windows","amd64","","windows-amd64.exe"),
  @("windows","arm64","","windows-arm64.exe"),
  @("windows","386","","windows-386.exe"),
  @("linux","amd64","","linux-amd64"),
  @("linux","arm64","","linux-arm64"),
  @("linux","arm","7","linux-armv7"),
  @("linux","386","","linux-386"),
  @("linux","riscv64","","linux-riscv64"),
  @("linux","ppc64le","","linux-ppc64le"),
  @("linux","s390x","","linux-s390x"),
  @("darwin","amd64","","macos-intel"),
  @("darwin","arm64","","macos-apple-silicon"),
  @("freebsd","amd64","","freebsd-amd64"),
  @("freebsd","arm64","","freebsd-arm64"),
  @("openbsd","amd64","","openbsd-amd64"),
  @("netbsd","amd64","","netbsd-amd64")
)

$env:CGO_ENABLED = "0"
$ok = @(); $failed = @()
foreach ($t in $targets) {
  $env:GOOS = $t[0]; $env:GOARCH = $t[1]
  if ($t[2]) { $env:GOARM = $t[2] } else { Remove-Item Env:GOARM -ErrorAction SilentlyContinue }
  $file = Join-Path $out ("DigitalSanDook-" + $t[3])
  Write-Host ("building {0,-26}" -f $t[3]) -NoNewline
  & go build -trimpath -ldflags "-s -w -X main.version=$Version" -o $file $pkg 2>&1 | Out-Null
  if ($LASTEXITCODE -eq 0) { Write-Host "ok  $([math]::Round((Get-Item $file).Length/1MB,1)) MB"; $ok += $t[3] }
  else { Write-Host "FAILED (skipped)"; $failed += $t[3]; Remove-Item $file -ErrorAction SilentlyContinue }
}
Remove-Item Env:GOOS, Env:GOARCH, Env:GOARM, Env:CGO_ENABLED -ErrorAction SilentlyContinue

$sums = Get-ChildItem $out -File | Where-Object { $_.Name -like "DigitalSanDook-*" } | Sort-Object Name |
  ForEach-Object { "{0}  {1}" -f (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower(), $_.Name }
$sums | Set-Content (Join-Path $out "SHA256SUMS.txt") -Encoding ascii
Write-Host "`n$($ok.Count) binaries in $out"
if ($failed.Count) { Write-Host "skipped: $($failed -join ', ')" }
if (Test-Path (Join-Path $out "dsd-data")) {
  Write-Warning "dist\dsd-data is a live data folder (TLS private key, password hashes). Do not ship it with the binaries."
}

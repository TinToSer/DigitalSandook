#!/usr/bin/env bash
# Builds single, standalone, dependency-free executables of DigitalSanDook for
# every supported platform into ./dist. Requires only the Go toolchain (1.25+).
#   usage: scripts/build.sh [version]        (SKIP_TESTS=1 to skip vet/tests)
set -u
cd "$(dirname "$0")/.."
VERSION="${1:-dev}"
OUT=dist
PKG=./cmd/digitalsandook

if [ "${SKIP_TESTS:-0}" != "1" ]; then
  echo "go vet + go test ..."
  go vet ./... && go test ./... || { echo "vet/tests failed - not building"; exit 1; }
fi

mkdir -p "$OUT"
rm -f "$OUT"/DigitalSanDook-* "$OUT"/SHA256SUMS.txt

# os arch goarm suffix
TARGETS="
windows amd64 - windows-amd64.exe
windows arm64 - windows-arm64.exe
windows 386 - windows-386.exe
linux amd64 - linux-amd64
linux arm64 - linux-arm64
linux arm 7 linux-armv7
linux 386 - linux-386
linux riscv64 - linux-riscv64
linux ppc64le - linux-ppc64le
linux s390x - linux-s390x
darwin amd64 - macos-intel
darwin arm64 - macos-apple-silicon
freebsd amd64 - freebsd-amd64
freebsd arm64 - freebsd-arm64
openbsd amd64 - openbsd-amd64
netbsd amd64 - netbsd-amd64
"
echo "$TARGETS" | while read -r os arch arm suffix; do
  [ -z "$os" ] && continue
  printf 'building %-26s' "$suffix"
  if [ "$arm" = "-" ]; then unset GOARM; else export GOARM="$arm"; fi
  if CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/DigitalSanDook-$suffix" "$PKG" 2>/dev/null; then
    echo ok
  else
    echo "FAILED (skipped)"; rm -f "$OUT/DigitalSanDook-$suffix"
  fi
done
( cd "$OUT" && (sha256sum DigitalSanDook-* 2>/dev/null || shasum -a 256 DigitalSanDook-*) > SHA256SUMS.txt )
echo "done -> $OUT"
if [ -d "$OUT/dsd-data" ]; then
  echo "WARNING: $OUT/dsd-data is a live data folder (TLS private key, password hashes). Do not ship it." >&2
fi

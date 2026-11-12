#!/usr/bin/env bash
# Turns the raw binaries in ./dist (from build.sh) into release archives in
# ./dist/release: .zip for Windows, .tar.gz (executable bit preserved) for the
# rest, each containing the program, README and LICENSE, plus SHA256SUMS.txt.
#   usage: scripts/package.sh <version>
set -eu
cd "$(dirname "$0")/.."
VERSION="${1:?usage: package.sh <version>}"
ROOT="$PWD"; OUT="$ROOT/dist/release"
rm -rf "$OUT"; mkdir -p "$OUT"
stage="$(mktemp -d)"; trap 'rm -rf "$stage"' EXIT

for f in dist/DigitalSanDook-*; do
  suffix="${f#dist/DigitalSanDook-}"
  name="DigitalSanDook-${VERSION}-${suffix%.exe}"
  d="$stage/$name"; mkdir -p "$d"
  cp README.md LICENSE "$d/"
  case "$suffix" in
    *.exe) cp "$f" "$d/DigitalSanDook.exe"
           ( cd "$stage" && zip -qrX "$OUT/$name.zip" "$name" ) ;;
    *)     cp "$f" "$d/DigitalSanDook"; chmod +x "$d/DigitalSanDook"
           tar --owner=0 --group=0 --numeric-owner -C "$stage" -czf "$OUT/$name.tar.gz" "$name" ;;
  esac
done

( cd "$OUT" && files=$(ls) && (sha256sum $files 2>/dev/null || shasum -a 256 $files) > ../SHA256SUMS.release && mv ../SHA256SUMS.release SHA256SUMS.txt )
ls -1 "$OUT"

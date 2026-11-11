#!/usr/bin/env bash
# Builds claudedash.exe: the Svelte interface first, then the Go program that embeds it.
# Usage: ./build.sh [output path], default bin/claudedash.exe
set -euo pipefail
cd "$(dirname "$0")"

OUT="${1:-bin/claudedash.exe}"

# Bindings come first: the interface imports them, so they must match the Go code being built.
wails3 generate bindings -clean=true -ts -d frontend/bindings
(cd frontend && npm ci --no-audit --no-fund && npm test && npm run build)
# The icon is drawn by the program's own code, then linked into the exe as a Windows resource.
go run ./tools/genicon
wails3 generate syso -arch amd64 -icon build/windows/icon.ico -manifest build/windows/wails.exe.manifest -info build/windows/info.json -out wails_windows_amd64.syso
mkdir -p "$(dirname "$OUT")"
go build -tags production -trimpath -ldflags "-s -w -H=windowsgui" -o "$OUT" .

echo "Built $OUT"

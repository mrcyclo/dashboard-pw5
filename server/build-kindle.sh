#!/usr/bin/env bash
# Builds the Kindle app and its KPM package folder in kindle/dist/claudedash.
set -euo pipefail
cd "$(dirname "$0")"

rm -rf ../kindle/dist/claudedash
mkdir -p ../kindle/dist/claudedash/claudedash ../kindle/dist/claudedash/scriptlets

GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" \
    -o ../kindle/dist/claudedash/claudedash/claudedash ./cmd/claudedash-kindle
cp ../kindle/package/assets/* ../kindle/dist/claudedash/claudedash/
cp ../kindle/package/manifest.json ../kindle/package/install.sh ../kindle/package/uninstall.sh ../kindle/package/launch.sh \
    ../kindle/dist/claudedash/

# The book's cover on the Kindle home screen: a portrait page with the app icon and name.
mkdir -p build
go run ./tools/kindlecover ../kindle/package/assets/NotoSans-Bold.ttf build/kindlecover.png
icon=$(base64 -w0 build/kindlecover.png)
sed "s|@ICON@|${icon}|" ../kindle/package/scriptlet.sh.in > "../kindle/dist/claudedash/scriptlets/Claude Dashboard.sh"

echo "Built ../kindle/dist/claudedash"

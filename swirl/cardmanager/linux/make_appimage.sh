#!/usr/bin/env bash
# SWIRL: build SWIRL-Card-Manager-linux-x86_64.AppImage from the static Linux binary.
# usage: linux/make_appimage.sh <version> [owner/repo for updates] [output folder]
# Runs on any Linux box with Go and (for packaging) appimagetool on PATH. Put the patched
# Flycast in assets/swirl-vmucap-linux.gz first (swirl/vmucap/build_linux.sh), or Preview and
# VMU capture are left out of the app.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:?version, for example 2.13.0}"
REPO="${2:-TheGlengineer/SWIRL}"
OUT="${3:-dist}"
NAME="SWIRL Card Manager"
BIN="SWIRL-Card-Manager"

mkdir -p "$OUT"
rm -rf "$OUT/appdir"

# the AppDir the packaging tool squashes
APPDIR="$OUT/appdir"
mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/icons/hicolor/256x256/apps"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
  -ldflags "-s -w -X main.updateRepo=$REPO" -o "$APPDIR/usr/bin/$BIN" .
cp linux/AppRun "$APPDIR/AppRun"
chmod +x "$APPDIR/AppRun"
cp linux/SWIRL-Card-Manager.desktop "$APPDIR/"
cp web/icon-256.png "$APPDIR/usr/share/icons/hicolor/256x256/apps/swirl.png"
ln -sf "usr/share/icons/hicolor/256x256/apps/swirl.png" "$APPDIR/swirl.png"
ln -sf "usr/bin/$BIN" "$APPDIR/SWIRL-Card-Manager"

rm -f "$OUT/SWIRL-Card-Manager-linux-x86_64.AppImage"
ARCH=x86_64 appimagetool "$APPDIR" "$OUT/SWIRL-Card-Manager-linux-x86_64.AppImage" >/dev/null
rm -rf "$APPDIR"
ls -l "$OUT/SWIRL-Card-Manager-linux-x86_64.AppImage"

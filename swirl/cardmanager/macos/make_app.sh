#!/usr/bin/env bash
# SWIRL: build SWIRL Card Manager.app for macOS (Apple Silicon and Intel in one app), then zip it for the
# in-app updater and wrap it in a .dmg for people downloading it.
# usage: macos/make_app.sh <version> [owner/repo for updates] [output folder]
# Runs on a Mac (needs lipo, codesign, ditto and hdiutil). Put the patched Flycast zip in
# assets/swirl-vmucap-macos.zip first (swirl/vmucap/build_macos.sh), or Preview and VMU capture are left out.
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION="${1:?version, for example 2.13.0}"
REPO="${2:-TheGlengineer/SWIRL}"
OUT="${3:-dist}"
NAME="SWIRL Card Manager"
APP="$OUT/$NAME.app"

mkdir -p "$OUT"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH=$arch go build -trimpath \
    -ldflags "-s -w -X main.updateRepo=$REPO" -o "$OUT/cm-$arch" .
done
lipo -create -output "$APP/Contents/MacOS/$NAME" "$OUT/cm-arm64" "$OUT/cm-amd64"
rm -f "$OUT/cm-arm64" "$OUT/cm-amd64"
# macOS wants plain numbers here; a preview (2.14.0-preview.1) is 2.14.0 with build number 2.14.0.1
CORE="${VERSION%%-*}"; BUILD="$CORE"
[ "$CORE" != "$VERSION" ] && BUILD="$CORE.${VERSION##*.}"
sed -e "0,/@VERSION@/s//$CORE/" -e "s/@VERSION@/$BUILD/g" macos/Info.plist.in > "$APP/Contents/Info.plist"
cp macos/AppIcon.icns "$APP/Contents/Resources/AppIcon.icns"
printf 'APPL????' > "$APP/Contents/PkgInfo"
# an ad hoc signature: not a Developer ID, so the first launch needs Open Anyway (see docs)
codesign --force --deep --sign - "$APP"
codesign --verify --deep --strict "$APP"
lipo -info "$APP/Contents/MacOS/$NAME"

# the zip is what the in-app updater downloads
rm -f "$OUT/SWIRL-Card-Manager-macOS.zip"
ditto -c -k --keepParent "$APP" "$OUT/SWIRL-Card-Manager-macOS.zip"

# the disk image: the app next to a link to Applications, to drag it across
STAGE="$(mktemp -d)"
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"
rm -f "$OUT/SWIRL-Card-Manager-macOS.dmg"
# GitHub's Mac runners sometimes answer "Resource busy" to the first create: try a few times before giving up
for attempt in 1 2 3 4 5; do
  if hdiutil create -volname "SWIRL Card Manager" -srcfolder "$STAGE" -ov -format UDZO "$OUT/SWIRL-Card-Manager-macOS.dmg" >/dev/null; then
    break
  fi
  if [ "$attempt" = 5 ]; then echo "hdiutil create failed five times" >&2; exit 1; fi
  echo "hdiutil create failed (attempt $attempt), trying again in 10 s" >&2
  sleep 10
done
rm -rf "$STAGE"
ls -l "$OUT"

#!/usr/bin/env bash
# SWIRL: build the patched Flycast for macOS (universal: Apple Silicon and Intel) and zip Flycast.app to
# swirl/cardmanager/assets/swirl-vmucap-macos.zip. Needs a Mac with Xcode and CMake. Homebrew libraries
# are ignored on purpose: they are built for one architecture only, and the app has to run on both.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$HERE/../cardmanager/assets/swirl-vmucap-macos.zip"
WORK="${WORK:-$HERE/build-macos}"
COMMIT=869038f40ac8cddc7741c3a35d545de057cc0dd5

if [ ! -d "$WORK/flycast/.git" ]; then
  git clone --quiet https://github.com/flyinghead/flycast.git "$WORK/flycast"
fi
cd "$WORK/flycast"
git fetch --quiet origin "$COMMIT" || true
git checkout --quiet --force "$COMMIT"
git clean -fdq
git submodule update --init --recursive --quiet
git apply "$HERE/vmucap_flycast.patch"
cp "$HERE/vmucap.cpp" core/vmucap.cpp

cmake -B build -G Xcode \
  -DCMAKE_BUILD_TYPE=Release \
  -DCMAKE_OSX_ARCHITECTURES="x86_64;arm64" \
  -DCMAKE_OSX_DEPLOYMENT_TARGET=10.15 \
  -DUSE_VULKAN=OFF -DUSE_LUA=OFF -DUSE_BREAKPAD=OFF -DUSE_DISCORD=OFF -DUSE_OPENMP=OFF \
  -DUSE_LIBAO=OFF -DUSE_PULSEAUDIO=OFF -DUSE_ALSA=OFF \
  -DUSE_HOST_SDL=OFF -DWITH_SYSTEM_ZSTD=OFF -DUSE_HOST_LIBZIP=OFF \
  -DCMAKE_IGNORE_PREFIX_PATH="/opt/homebrew;/usr/local"
cmake --build build --config Release -- -quiet

APP="$(find build -type d -name Flycast.app -path '*Release*' | head -n 1)"
[ -n "$APP" ] || { echo "Flycast.app was not built"; exit 1; }
lipo -info "$APP/Contents/MacOS/Flycast"
# an ad hoc signature, so Apple Silicon Macs run it
codesign --force --deep --sign - "$APP"
rm -f "$OUT"
ditto -c -k --keepParent "$APP" "$OUT"
ls -l "$OUT"

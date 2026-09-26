#!/usr/bin/env bash
# SWIRL: build the patched Flycast for Linux and gzip it to
# swirl/cardmanager/assets/swirl-vmucap-linux.gz.
#
# Runs on an Ubuntu/Debian x86-64 box with the build deps installed:
#   sudo apt-get install -y build-essential cmake git libgl1-mesa-dev libegl1-mesa-dev \
#     libx11-dev libxrandr-dev libxext-dev libxrender-dev libxi-dev libxinerama-dev \
#     libxcursor-dev libxss-dev libasound2-dev libpulse-dev libcurl4-openssl-dev zlib1g-dev
#
# SDL2 and libzip are built from Flycast's own copies (USE_HOST_SDL=OFF, USE_HOST_LIBZIP=OFF)
# so the binary carries everything except the system libc and OpenGL, matching the self-contained
# goal of the Windows build. Audio is null only (the capture mode runs without sound).
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="$HERE/../cardmanager/assets/swirl-vmucap-linux.gz"
WORK="${WORK:-$HERE/build-linux}"
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

cmake -B build -DCMAKE_BUILD_TYPE=Release \
  -DUSE_VULKAN=OFF -DUSE_OPENGL=ON -DUSE_LUA=OFF -DUSE_BREAKPAD=OFF \
  -DUSE_DISCORD=OFF -DUSE_OPENMP=OFF \
  -DUSE_LIBAO=OFF -DUSE_PULSEAUDIO=OFF -DUSE_ALSA=OFF \
  -DUSE_HOST_SDL=OFF -DUSE_HOST_LIBZIP=OFF -DBUILD_TESTING=OFF
cmake --build build -j"$(nproc)"

BIN="$(find build -type f -name flycast -perm -u+x | head -n 1)"
[ -n "$BIN" ] || { echo "flycast was not built"; exit 1; }
rm -f "$OUT"
gzip -c "$BIN" > "$OUT"
ls -l "$OUT"

#!/usr/bin/env bash
# SWIRL: install the prebuilt Dreamcast toolchain (GCC 15.1 + KallistiOS 2.2.1) on Ubuntu 22.04 / WSL2.
# Source: github.com/drpaneas/dreamcast-toolchain-builds (checksum pinned below).
set -euo pipefail
DEST="${1:-$HOME/dreamcast}"
URL=https://github.com/drpaneas/dreamcast-toolchain-builds/releases/download/gcc15.1.0-kos2.2.1/dreamcast-toolchain-gcc15.1.0-kos2.2.1-linux-x86_64-ubuntu22.tar.gz
SHA=36ee8b174331f53996990327de8df77ff662041989bbafab9e68572383d03b41
mkdir -p "$DEST"; cd "$DEST"
curl -L -C - -o tc.tar.gz "$URL"
echo "$SHA  tc.tar.gz" | sha256sum -c -
tar xzf tc.tar.gz && rm tc.tar.gz
echo "Done. Run: source $DEST/kos/environ.sh"

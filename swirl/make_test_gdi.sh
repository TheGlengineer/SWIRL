#!/usr/bin/env bash
# SWIRL: build a Flycast test disc laid out like a real menu disc (5 track GDI).
# Usage: swirl/make_test_gdi.sh [out_dir]
#   Uses swirl/build/gdemu/1ST_READ.BIN (run swirl/build.sh first) and the sample library in swirl/test
#   (made by swirl/tools/make_test_library.py if it is missing). Set MENU_DATA to a folder of extra menu
#   files (for example openMenu's THEME folder) to include them.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="${1:-$HERE/build/flycast_test}"
BIN="$HERE/build/gdemu/1ST_READ.BIN"
[ -f "$BIN" ] || { echo "Build the menu first: swirl/build.sh"; exit 1; }
[ -f "$HERE/test/OPENMENU.INI" ] || python3 "$HERE/tools/make_test_library.py"
TOOL="$HERE/build/swirl-cardmanager"
(cd "$HERE/cardmanager" && go build -o "$TOOL" .)
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
[ -n "${MENU_DATA:-}" ] && cp -r "$MENU_DATA/." "$STAGE/"
cp "$BIN" "$STAGE/1ST_READ.BIN"          # GD-ROM boot: the binary must be unscrambled
cp "$HERE"/test/OPENMENU.INI "$HERE"/test/*.DAT "$STAGE/"
"$TOOL" -build-menu "$STAGE" -out "$OUT"
echo "Test disc: $OUT/disc.gdi"

#!/usr/bin/env bash
# SWIRL host parser check: builds gd_list.c, inih and dat_reader.c with the sanitizers and runs them over
# every crafted OPENMENU.INI and DAT case. Needs gcc and python3. A base library is taken from swirl/test
# (make it with swirl/tools/make_test_library.py) or from the folder given as the first argument.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
BASE="${1:-$ROOT/swirl/test}"
[ -f "$BASE/OPENMENU.INI" ] && [ -f "$BASE/META.DAT" ] || { echo "no OPENMENU.INI and META.DAT in $BASE"; exit 2; }
OUT="${TMPDIR:-/tmp}/swirl-host-check"
rm -rf "$OUT"; mkdir -p "$OUT"
gcc -std=gnu11 -O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all -DSTANDALONE_BINARY=1 -I"$ROOT" \
  -o "$OUT/parsers_check" "$HERE/parsers_check.c" "$ROOT/backend/gd_list.c" "$ROOT/external/ini.c" "$ROOT/texture/dat_reader.c"
export ASAN_OPTIONS=detect_leaks=0
fail=0
while read -r kind path expected; do
  if [ "$expected" = "-1" ]; then
    "$OUT/parsers_check" "$kind" "$path" > "$path.log" 2>&1 || { echo "FAIL $kind $path"; cat "$path.log"; fail=1; }
  else
    "$OUT/parsers_check" "$kind" "$path" "$expected" > "$path.log" 2>&1 || { echo "FAIL $kind $path"; cat "$path.log"; fail=1; }
  fi
done < <(python3 "$HERE/mkcases.py" "$BASE/OPENMENU.INI" "$BASE/META.DAT" "$OUT/cases")
[ $fail = 0 ] && echo "host parser check: all cases pass" || exit 1

# the SWIRL.DAT reader against a file from every released layout (tests/host/savefiles/README.md)
gcc -std=gnu11 -O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all -I"$ROOT" \
  -o "$OUT/save_check" "$HERE/save_check.c" "$ROOT/ui/swirl/sw_save.c"
"$OUT/save_check" "$HERE"/savefiles/*.bin

# the padded string helper of the Classic styles
gcc -std=gnu11 -O1 -g -fsanitize=address,undefined -fno-sanitize-recover=all -I"$ROOT" -o "$OUT/pad_check" "$HERE/pad_check.c"
"$OUT/pad_check"

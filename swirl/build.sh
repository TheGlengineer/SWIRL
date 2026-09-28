#!/usr/bin/env bash
# SWIRL: build the menu. Outputs to swirl/build/:
#   gdemu/1ST_READ.BIN   unscrambled, for GDEMU via GDMENUCardManager (GD-ROM boot)
#   cdi/1ST_READ.BIN     scrambled, only for burning a CD-R test disc
set -eo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
source "${KOS_ENV:-$HOME/dreamcast/kos/environ.sh}" >/dev/null
set -u
cd "$ROOT"
make clean >/dev/null
# the full version (for the start up log and reports) comes from Card Manager's, which the release checks
VERSION_STR=$(sed -n 's/^const version = "\(.*\)"/\1/p' swirl/cardmanager/main.go)
make OPTIONS="${OPTIONS:-} -DSWIRL_VERSION_STR=\\\"$VERSION_STR\\\""
mkdir -p swirl/build/gdemu swirl/build/cdi
cp themeMenu.bin swirl/build/gdemu/1ST_READ.BIN
cp 1ST_READ.BIN swirl/build/cdi/1ST_READ.BIN
echo "Built swirl/build/gdemu/1ST_READ.BIN"

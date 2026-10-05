# SWIRL.DAT files written by released versions

Each file is the data part of a SWIRL.DAT package (after the VMU package header) written by the menu built from
that release's tag, running in the emulator harness with two favourites (Beats of Rage and Crazy Taxi from the
25 game test library) and no launches. `save_check.c` reads every file here with the current reader and expects
those two favourites back. Add one file per released layout; never regenerate a file by hand from a struct
definition, that is how a bug that was not there got "found" (BACKLOG L27).

| File | Written by | Layout |
|---|---|---|
| swirl_dat_2.13.3.bin | v2.13.3 (build from the tag, 2026-09-30) | SWL2 version 2, no tail |
| swirl_dat_2.14.0.bin | v2.14.0 | SWL2 version 2, SEQ1 tail |
| swirl_dat_2.14.2.bin | v2.14.2 | SWL2 version 3, SEQ1 tail |
| swirl_dat_2.14.3.bin | v2.14.3 (the 1ST_READ.BIN embedded at the tag, the file users have) | SWL2 version 3, SEQ1 tail; the last 16 byte prefs layout |
| swirl_dat_2.16.1.bin | v2.16.1 (the released 1ST_READ.BIN; it took the 2.14.3 file in and wrote it back in its own form) | SWL2 version 4, SEQ1 tail, PRF4 block; the last layout before PRF5 |

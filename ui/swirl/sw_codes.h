/* SWIRL warning codes: problems that do not stop the menu but that a user should be able to report.
   Each has a number (never reused, only added), a short name and plain words for the Diagnostics screen.
   The same table is in swirl/diag/codes.tsv for SWIRL Card Manager's report decoder; keep both in step.
   Codes appear in the trace as "Wnn <detail>" and in a report's w= line as "Wnnx<count>". */
#pragma once

#define SW_CODES(X)                                                                                            \
  X(SW_WARN_SAVE_FAILED, 1, "save-failed", "A save to the VMU failed")                                        \
  X(SW_WARN_DAT_DAMAGED, 2, "swirl-dat-damaged", "SWIRL.DAT on the VMU was damaged")                         \
  X(SW_WARN_PICTURE, 3, "picture-skipped", "A picture on the SD card could not be used")                     \
  X(SW_WARN_INI, 4, "ini-line", "OPENMENU.INI has lines SWIRL could not use")                                \
  X(SW_WARN_GDEMU, 5, "gdemu-no-answer", "The GDEMU did not answer")                                         \
  X(SW_WARN_LAUNCH, 6, "launch-returned", "A game did not start and SWIRL came back")                        \
  X(SW_WARN_LATE_CARD, 7, "late-card", "A memory card answered after start up")                              \
  X(SW_WARN_MEMORY, 8, "memory-line", "SWIRL's memory reached the loader's line")                            \
  X(SW_WARN_CFG_DAMAGED, 9, "cfg-damaged", "OPENMENU.CFG on the VMU was damaged")                            \
  X(SW_WARN_META, 10, "meta-dat", "META.DAT could not be used")                                              \
  X(SW_WARN_THEME, 11, "theme", "A theme file on the SD card could not be used")                            \
  X(SW_WARN_MULTIDISC, 12, "multidisc", "A game has more discs than SWIRL shows")                            \
  X(SW_WARN_VMU_FULL, 13, "vmu-full", "No room on the VMU for SWIRL's save")                                \
  X(SW_WARN_DAT_FILE, 14, "dat-file", "A DAT file on the SD card could not be used")                        \
  X(SW_WARN_VRAM, 15, "video-memory", "Not enough video memory for the pictures")                           \
  X(SW_WARN_SCAN, 16, "controller-scan", "A controller port did not answer at start up")                    \
  X(SW_WARN_STEP, 17, "startup-step", "A start up step failed")                                              \
  X(SW_WARN_STYLE, 18, "style-files", "The saved style needs theme files this disc lacks")                  \
  X(SW_WARN_DISC, 19, "disc-read", "The menu disc could not be read")                                        \
  X(SW_WARN_DAT_SHIFTED, 20, "swirl-dat-shifted", "Favourites and history from 2.13 were put back")

enum sw_code {
#define X(name, num, key, words) name = num,
  SW_CODES(X)
#undef X
  SW_CODE_END
};

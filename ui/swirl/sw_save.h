/* SWIRL.DAT, the file on the VMU that holds favourites, play history and SWIRL's settings: its layout, the
 * reader for every layout ever shipped, and the writer for the current one. No KallistiOS in here, so
 * tests/host/save_check.c builds this on a PC and reads a file saved by each released SWIRL.
 *
 * Layouts, oldest first (the bytes after the VMU package header):
 *   SWL1  magic[4] version=1 count prefs[4]  stats[count]                       2.0 to 2.10
 *   SWL2  magic[4] version=2 count prefs[16] stats[count]                       2.11 to 2.13.3 (no tail)
 *   SWL2  magic[4] version=2 count prefs[16] stats[count] SEQ1 seq              2.14.0 and 2.14.1
 *   SWL2  magic[4] version=3 count prefs[16] stats[count] SEQ1 seq              2.14.2 on (same bytes; the
 *                                                                              version now says so)
 * The prefs block has been 16 bytes since SWL2: 2.13 kept two reserved bytes that 2.14 named. A change to
 * sw_prefs or sw_stat is a new layout: bump SW_SAVE_VERSION, keep a reader for the old size, and add a file
 * written by the previous release to tests/host/savefiles. The static asserts in sw_save.c refuse to build
 * otherwise, and tests/host/save_check.c reads a file from every released layout. */
#pragma once
#include <stdint.h>

#include "sw_lib.h"

#define SW_SAVE_VERSION 3
#define SW_SAVE_MAX_STATS 256

/* what sw_save_parse found */
enum {
  SW_SAVE_LAYOUT_V1 = 1,      /* SWL1 */
  SW_SAVE_LAYOUT_V2 = 2,      /* SWL2 version 2 (2.11 to 2.14.1) */
  SW_SAVE_LAYOUT_CURRENT = 3, /* SWL2 version 3 */
};

/* Reads the data of a SWIRL.DAT package. st holds up to SW_SAVE_MAX_STATS. Returns 0 and fills n, layout and
   seq (0 when the file has no write count), or -1 for a file that is not SWIRL's. */
int sw_save_parse(const uint8_t *data, int len, sw_prefs *p, sw_stat *st, int *n, int *layout, uint32_t *seq);

/* Writes the current layout into out (at least sw_save_size(n) bytes); returns the length. */
int sw_save_build(uint8_t *out, const sw_prefs *p, const sw_stat *st, int n, uint32_t seq);
int sw_save_size(int n);

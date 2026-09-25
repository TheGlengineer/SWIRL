/*
 * SWIRL: VMU screen output (48x32, 1 bit) plus an on-screen preview texture.
 */
#pragma once

#include <stdint.h>

#include "../dc/pvr_texture.h"

void sw_vmu_init(void);
/* up to 4 lines of text; NULL lines are blank. Pushed to real VMUs when it changes. */
void sw_vmu_text(const char *l1, const char *l2, const char *l3, const char *l4);
/* a ready made 48x32 image (192 bytes, row major, MSB = left, 1 = dark); key avoids re-sending */
void sw_vmu_bitmap(const uint8_t *bits, const char *key);
void sw_vmu_tick(void);            /* call once per frame */
const image *sw_vmu_preview(void); /* 64x32 texture, first 48 columns used */
void sw_vmu_show_logo(void); /* the SWIRL logo (or the owner's LOGO.VMU) */
void sw_vmu_boot_logo(void); /* same, drawn immediately at power on */

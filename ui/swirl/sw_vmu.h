/*
 * SWIRL: VMU screen output (48x32, 1 bit) plus an on-screen preview texture. See sw_vmu.c.
 */
#pragma once

#include <stdint.h>

#include "../dc/pvr_texture.h"

void sw_vmu_init(void);
/* up to 4 lines of text; NULL lines are blank */
void sw_vmu_text(const char *l1, const char *l2, const char *l3, const char *l4);
/* a ready made 48x32 image (192 bytes, row major, MSB = left, 1 = dark); key avoids re-sending */
void sw_vmu_bitmap(const uint8_t *bits, const char *key);
void sw_vmu_tick(void);            /* call once per frame */
const image *sw_vmu_preview(void); /* 64x32 texture, first 48 columns used */
void sw_vmu_show_logo(void); /* the SWIRL logo (or the owner's LOGO.VMU), animated */

/* save status, set every frame; it shows over the logo and the game picture */
enum {
  SW_VMU_NONE = 0,
  SW_VMU_COUNTDOWN, /* arg: video frames left of the 3 second countdown */
  SW_VMU_WRITING,   /* the save is writing */
  SW_VMU_SAVING,    /* the "Saving... please wait" hold after it */
  SW_VMU_SAVED,
  SW_VMU_NO_SPACE,
  SW_VMU_CHECK,     /* not saved: check the VMU */
  SW_VMU_BUSY,      /* the card was busy; trying again */
};
void sw_vmu_overlay(int kind, int arg);
void sw_vmu_before_write(void); /* a save is about to write: show "writing" now, then leave the card alone */
void sw_vmu_freeze(int on);     /* 1 while launching a game: nothing more is sent */
void sw_vmu_shutdown(void);     /* ends the VMU thread (a game, the BIOS or another style is next) */

/* power on */
void sw_vmu_boot_logo(void);  /* the still logo, drawn straight away */
void sw_vmu_boot_start(void); /* the logo intro plays while SWIRL loads (after the settings are read) */
void sw_vmu_boot_stop(void);  /* ...and stops before SWIRL.DAT is used */

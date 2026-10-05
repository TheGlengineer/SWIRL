/* SWIRL backdrops (2.17): what is drawn behind everything. The three originals (Cover colour, Night,
   Seasonal), the animated ones designed in BACKDROPS_DESIGN.md (Tide, Spiral, Starfield, Embers, Horizon,
   Pulse), Seasonal's time of day and weather, the Backdrop motion switch, and the owner's own pictures from
   Card Manager (BG/BG.DAT and BG/BGnn.PVR on the menu disc). Everything but a picture is drawn from the
   primitives in sw_gfx.h, so it costs no video memory and no card space; a picture is one VQ texture. */
#pragma once
#include <stdint.h>

#include "sw_lib.h"

enum {
  BACKDROP_COVER = 0, BACKDROP_NIGHT, BACKDROP_SEASONAL, BACKDROP_TIDE, BACKDROP_SPIRAL, BACKDROP_STARFIELD,
  BACKDROP_EMBERS, BACKDROP_HORIZON, BACKDROP_COUNT
};

void sw_bd_init(void);                 /* once, after the menu disc is readable: reads BG/BG.DAT */
void sw_bd_apply(const sw_prefs *p);   /* the settings changed (or at start): season, picture, motion */
int sw_bd_season_accent(uint32_t *c);  /* 1 and the season's accent when the Seasonal backdrop sets the accent */
int sw_bd_season_name(void);           /* S_SEASON_* of the month, for the System row */
int sw_bd_backdrop_name(int backdrop); /* S_BACKDROP_* for a built in backdrop */
int sw_bd_picture_count(void);         /* pictures Card Manager put on the disc, 0 when none */
const char *sw_bd_picture_name(int n); /* 1..count, as named in Card Manager */
int sw_bd_picture_failed(void);        /* 1 when the chosen picture could not be used (Cover colour is drawn) */
int sw_bd_picture_dim(int n);          /* the darkening Card Manager chose for picture n */
void sw_bd_set_accent(uint32_t c);     /* the accent in use (Spiral, Starfield, Embers, Horizon, Pulse draw in it) */
uint32_t sw_bd_ambient(uint32_t cover); /* the glow colour for the current backdrop, from the cover's */
void sw_bd_draw(uint32_t base, uint32_t cover, int home); /* the whole background of a frame */

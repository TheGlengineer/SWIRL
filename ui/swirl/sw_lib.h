/*
 * SWIRL: game library model, collections and play stats (saved to VMU).
 */
#pragma once

#include <stdint.h>

struct gd_item;
struct db_item;

typedef struct sw_game {
  const struct gd_item *item; /* disc 1 of the set */
  struct db_item *meta;       /* NULL when META.DAT has no entry */
  uint32_t key;               /* stable id for stats */
  uint8_t discs;              /* number of discs in the set */
  uint8_t year_ok;
  uint16_t year;
  uint32_t color;             /* placeholder art colour */
} sw_game;

typedef struct sw_stat {
  uint32_t key;
  uint16_t plays;
  uint8_t fav;
  uint8_t flags;
  uint32_t last; /* unix seconds from the Dreamcast clock */
} sw_stat;

enum sw_sort { SW_SORT_NAME = 0, SW_SORT_RECENT, SW_SORT_PLAYS, SW_SORT_YEAR, SW_SORT_CARD, SW_SORT_COUNT };
/* A GDMENU card divider: a slot whose name is a run of the same mark on both sides, "*****USA*****" or
   "--- Arcade ---". Dividers are not games: they are hidden from every list and become sections (collections
   named after the label, holding the games that follow them on the card) and headings of the Card order sort.
   Returns the label's length and copies it into label (empty for a bare line of marks), or -1 if not one. */
int sw_lib_divider_label(const char *name, char *label, int cap);

typedef struct sw_collection {
  char name[32];
  int count;
  int id;
} sw_collection;

int sw_lib_init(void);
int sw_lib_count(void);
sw_game *sw_lib_game(int idx);

/* stats */
sw_stat *sw_stat_get(const sw_game *g, int create);
int sw_lib_is_fav(const sw_game *g);
void sw_lib_toggle_fav(const sw_game *g);
void sw_lib_mark_played(const sw_game *g);
int sw_lib_save(void);               /* save now and wait until it is done; 0 on success */
void sw_lib_finish(void);            /* let a save in progress finish (before anything else uses the VMU) */
void sw_lib_set_idle(void (*fn)(void)); /* draws a frame while waiting for a save */
int sw_lib_save_async(void);         /* save on a worker thread; 0 started, -1 busy, -7 card not answering */
int sw_lib_save_result(int *rv);     /* 1 once when a save finished, with its result */
int sw_lib_busy(void);               /* a save is running */
void sw_lib_settings_dirty(void);    /* openMenu's settings (style, beep) changed and need saving too */
const char *sw_lib_save_status(char *buf, int len); /* one line for System > Save */
int sw_lib_blocks_short(void);       /* after a save failed for space (-8): blocks missing on the card */
void sw_lib_retrying(int on);        /* the menu will (1) or will not (0) try a failed save again by itself */
int sw_lib_dirty(void);
void sw_lib_mark_dirty(void);
int sw_lib_stats_loaded(void);

/* settings kept in SWIRL.DAT */
typedef struct sw_prefs {
  uint8_t clock24;
  uint8_t rumble;
  uint8_t sort;
  uint8_t attract;
  /* added in SWIRL.DAT version 2 */
  uint8_t accent;    /* index into the accent colour table */
  uint8_t backdrop;  /* 0 cover colour, 1 night, 2 seasonal */
  uint8_t music;     /* menu music on */
  uint8_t sfx;       /* navigation sounds on */
  uint8_t resume;    /* start on the last played game */
  uint8_t music_vol; /* 0..10 */
  uint8_t sfx_vol;   /* 0..10 */
  uint8_t quality;   /* 0 high (32 bit + anti-aliasing), 1 standard (16 bit, as openMenu) */
  /* screen saver: "attract" above turns it on or off */
  uint8_t saver_style; /* index into the screen saver list */
  uint8_t saver_min;   /* minutes without input before it starts, 1..30 */
  uint8_t gameid_off; /* 1: no Game ID for a VM2 / VMU Pro at launch (0, on, in older saves) */
  uint8_t swirl_style_set; /* 1 once SWIRL has switched OPENMENU.CFG's style to SWIRL (done once per card) */
  /* added in SWIRL.DAT version 4 (2.15): the prefs block grew from 16 to 20 bytes, see sw_save.h */
  uint8_t logo;        /* header logo: 0 by the console's region, 1 orange (USA, Japan), 2 blue (Europe) */
  uint8_t reserved[3]; /* zero; room for the next settings without another layout */
} sw_prefs;
enum { SW_LOGO_REGION = 0, SW_LOGO_ORANGE, SW_LOGO_BLUE, SW_LOGO_COUNT };
sw_prefs *sw_lib_prefs(void);
void sw_lib_note_style_set(void); /* SWIRL took the style over: remember it (saved, without touching the other settings) */
int sw_lib_early_quality(void); /* before the screen is set up: 1 for high */
int sw_lib_late_card(void);     /* a card that attached after start up: 1 when its SWIRL.DAT was taken in */

/* per game launch settings, kept in sw_stat.flags */
enum { SW_REGION_AUTO = 0, SW_REGION_JAPAN, SW_REGION_USA, SW_REGION_EUROPE };
enum { SW_BOOT_NONE = 0, SW_BOOT_ANIMATION, SW_BOOT_LICENSE, SW_BOOT_BOTH };
/* The default start: boot animation and SEGA screen, the way the console, GDMENU and openMenu start a game.
   Skipping them is a per game choice, since a game may rely on what the BIOS sets up during the SEGA screen. */
#define SW_BOOT_DEFAULT SW_BOOT_BOTH
typedef struct sw_launch {
  int region; /* SW_REGION_* */
  int vga;    /* 1 force VGA (default), 0 leave it to the game */
  int boot;   /* SW_BOOT_* */
} sw_launch;
sw_launch sw_lib_launch_get(const sw_game *g);
void sw_lib_launch_set(const sw_game *g, sw_launch l);

/* the most recently played game, or -1 */
int sw_lib_last_played(void);

/* views: fill out[] with game indices, return count */
int sw_lib_view_all(int *out, int sort);
int sw_lib_view_recent(int *out, int max);
int sw_lib_collections(sw_collection *out, int max);
int sw_lib_view_collection(int id, int *out, int sort);

/* helpers */
const char *sw_lib_genre_name(const sw_game *g);
int sw_lib_players(const sw_game *g);
void sw_lib_disc_list(const sw_game *g, const struct gd_item **out, int *count, int max);
void sw_lib_format_last_played(uint32_t last, char *out, int len);
uint32_t sw_now(void);

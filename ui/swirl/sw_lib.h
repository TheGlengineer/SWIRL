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

enum sw_sort { SW_SORT_NAME = 0, SW_SORT_RECENT, SW_SORT_PLAYS, SW_SORT_YEAR, SW_SORT_COUNT };

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
int sw_lib_save(void); /* write SWIRL.DAT to the first VMU with space; 0 on success */
int sw_lib_save_async(void);     /* same, on a worker thread; -1 if one is already running */
int sw_lib_save_result(int *rv); /* 1 once when an async save finished, with its result */
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
  uint8_t reserved[2];
} sw_prefs;
sw_prefs *sw_lib_prefs(void);
int sw_lib_early_quality(void); /* before the screen is set up: 1 for high */

/* per game launch settings, kept in sw_stat.flags */
enum { SW_REGION_AUTO = 0, SW_REGION_JAPAN, SW_REGION_USA, SW_REGION_EUROPE };
enum { SW_BOOT_NONE = 0, SW_BOOT_ANIMATION, SW_BOOT_LICENSE, SW_BOOT_BOTH };
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

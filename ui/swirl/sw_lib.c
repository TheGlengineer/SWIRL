/*
 * SWIRL: game library model, collections and play stats (saved to VMU as SWIRL.DAT).
 */
#define _DEFAULT_SOURCE
#include "sw_lib.h"

#include <strings.h>
#include <time.h>

#include <arch/rtc.h>
#include <dc/maple.h>
#include <dc/maple/vmu.h>
#include <dc/vmu_pkg.h>
#include <dc/vmufs.h>
#include <kos/fs.h>
#include <kos/thread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../../backend/db_item.h"
#include "../../backend/db_list.h"
#include "../../backend/gd_item.h"
#include "../../backend/gd_list.h"

#if __has_include("../openmenu_vmu.h") && __has_include("../openmenu_pal.h")
#include "../openmenu_pal.h"
#include "../openmenu_vmu.h"
#define SW_HAVE_ICON 1
#else
#define SW_HAVE_ICON 0
#endif

#define MAX_GAMES 1024
#define MAX_STATS 256
#define SAVE_NAME "SWIRL.DAT"

static sw_game games[MAX_GAMES];
static int num_games;

static sw_stat stats[MAX_STATS];
static int num_stats;
static sw_prefs prefs = {.clock24 = 0, .rumble = 0, .sort = SW_SORT_NAME, .attract = 1, .accent = 0, .backdrop = 0,
                         .music = 1, .sfx = 1, .resume = 0, .music_vol = 7, .sfx_vol = 6, .saver_style = 0, .saver_min = 5};
static int dirty;
static int loaded;

typedef struct __attribute__((packed)) save_blob {
  char magic[4]; /* SWL2 (SWL1 had a 4 byte prefs block) */
  uint16_t version;
  uint16_t count;
  sw_prefs prefs;
  sw_stat stats[MAX_STATS];
} save_blob;

typedef struct __attribute__((packed)) save_blob_v1 {
  char magic[4];
  uint16_t version;
  uint16_t count;
  uint8_t prefs[4];
  sw_stat stats[MAX_STATS];
} save_blob_v1;

static const uint32_t placeholder_colors[] = {
    0xFF1F3F7A, 0xFF7A2E1F, 0xFF1F6B4F, 0xFF5B2A7A, 0xFF7A5A12, 0xFF154F66, 0xFF6B1F3F, 0xFF3F5A1F,
};

static uint32_t fnv1a(const char *a, const char *b) {
  uint32_t h = 2166136261u;
  for (; a && *a; a++) h = (h ^ (uint8_t)*a) * 16777619u;
  h = (h ^ '|') * 16777619u;
  for (; b && *b; b++) h = (h ^ (uint8_t)*b) * 16777619u;
  return h ? h : 1;
}

uint32_t sw_now(void) {
  return (uint32_t)rtc_unix_secs();
}

/* ---------- VMU persistence ---------- */
static maple_device_t *find_vmu(int need_blocks, int *has_file) {
  maple_device_t *dev, *first_free = NULL;
  char path[32];
  *has_file = 0;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_MEMCARD)); i++) {
    snprintf(path, sizeof(path), "/vmu/%c%d/%s", 'a' + dev->port, dev->unit, SAVE_NAME);
    file_t f = fs_open(path, O_RDONLY | O_META);
    if (f != FILEHND_INVALID) {
      fs_close(f);
      *has_file = 1;
      return dev;
    }
    if (!first_free && vmufs_free_blocks(dev) >= need_blocks)
      first_free = dev;
  }
  return first_free;
}

static void load_stats(void) {
  int has_file = 0;
  maple_device_t *dev = find_vmu(0, &has_file);
  if (!dev || !has_file)
    return;
  char path[32];
  snprintf(path, sizeof(path), "/vmu/%c%d/%s", 'a' + dev->port, dev->unit, SAVE_NAME);
  file_t f = fs_open(path, O_RDONLY | O_META);
  if (f == FILEHND_INVALID)
    return;
  int size = fs_total(f);
  uint8_t *buf = malloc(size);
  if (!buf) {
    fs_close(f);
    return;
  }
  fs_read(f, buf, size);
  fs_close(f);
  vmu_pkg_t pkg;
  if (vmu_pkg_parse(buf, size, &pkg) == 0 && pkg.data_len >= (int)(sizeof(save_blob_v1) - sizeof(sw_stat) * MAX_STATS)) {
    const int room_v2 = (pkg.data_len - (int)(sizeof(save_blob) - sizeof(sw_stat) * MAX_STATS)) / (int)sizeof(sw_stat);
    const int room_v1 = (pkg.data_len - (int)(sizeof(save_blob_v1) - sizeof(sw_stat) * MAX_STATS)) / (int)sizeof(sw_stat);
    const save_blob *b = (const save_blob *)pkg.data;
    if (!memcmp(b->magic, "SWL2", 4)) {
      prefs = b->prefs;
      /* saves from before the screen saver settings have zeros there */
      if (prefs.saver_min < 1 || prefs.saver_min > 30) prefs.saver_min = 5;
      num_stats = b->count > MAX_STATS ? MAX_STATS : b->count;
      if (num_stats > room_v2) num_stats = room_v2 < 0 ? 0 : room_v2;
      memcpy(stats, b->stats, num_stats * sizeof(sw_stat));
      loaded = 1;
    } else if (!memcmp(b->magic, "SWL1", 4)) {
      /* older SWIRL: keep favourites, history and the four original settings */
      const save_blob_v1 *o = (const save_blob_v1 *)pkg.data;
      prefs.clock24 = o->prefs[0];
      prefs.rumble = o->prefs[1];
      prefs.sort = o->prefs[2];
      prefs.attract = o->prefs[3];
      num_stats = o->count > MAX_STATS ? MAX_STATS : o->count;
      if (num_stats > room_v1) num_stats = room_v1 < 0 ? 0 : room_v1;
      memcpy(stats, o->stats, num_stats * sizeof(sw_stat));
      for (int i = 0; i < num_stats; i++) stats[i].flags = 0;
      loaded = 1;
      dirty = 1;
    }
  }
  free(buf);
}

/* builds the VMU file for the current stats; caller frees *out */
static int build_save(uint8_t **out, int *out_size) {
  static save_blob blob;
  memset(&blob, 0, sizeof(blob));
  memcpy(blob.magic, "SWL2", 4);
  blob.version = 2;
  blob.count = num_stats;
  blob.prefs = prefs;
  memcpy(blob.stats, stats, num_stats * sizeof(sw_stat));
  int data_len = sizeof(blob) - sizeof(sw_stat) * (MAX_STATS - num_stats);

  vmu_pkg_t pkg;
  memset(&pkg, 0, sizeof(pkg));
  strcpy(pkg.desc_short, "SWIRL");
  strcpy(pkg.desc_long, "SWIRL favorites and history");
  strcpy(pkg.app_id, "SWIRL");
#if SW_HAVE_ICON
  pkg.icon_cnt = 1;
  pkg.icon_anim_speed = 0;
  memcpy(pkg.icon_pal, openmenu_pal, sizeof(pkg.icon_pal));
  pkg.icon_data = openmenu_icon;
#endif
  pkg.eyecatch_type = VMUPKG_EC_NONE;
  pkg.data_len = data_len;
  pkg.data = (const uint8_t *)&blob;
  return vmu_pkg_build(&pkg, out, out_size) < 0 ? -1 : 0;
}

static int write_save(uint8_t *out, int out_size) {
  int has_file = 0;
  maple_device_t *dev = find_vmu((out_size + 511) / 512, &has_file);
  if (!dev)
    return -2;
  char path[32];
  snprintf(path, sizeof(path), "/vmu/%c%d/%s", 'a' + dev->port, dev->unit, SAVE_NAME);
  file_t f = fs_open(path, O_WRONLY | O_META);
  int rv = -3;
  if (f != FILEHND_INVALID) {
    rv = (fs_write(f, out, out_size) == out_size) ? 0 : -4;
    fs_close(f);
  }
  return rv;
}

static volatile int async_busy, async_done, async_rv;

int sw_lib_save(void) {
  while (async_busy) thd_sleep(10);
  uint8_t *out = NULL;
  int out_size = 0;
  if (build_save(&out, &out_size) < 0)
    return -1;
  int rv = write_save(out, out_size);
  free(out);
  if (rv == 0)
    dirty = 0;
  return rv;
}

static void *save_thread(void *arg) {
  uint8_t *out = arg;
  int size = *(int *)out;
  async_rv = write_save(out + 32, size);
  free(out);
  async_done = 1;
  async_busy = 0;
  return NULL;
}

int sw_lib_save_async(void) {
  if (async_busy)
    return -1;
  uint8_t *pkt = NULL;
  int size = 0;
  if (build_save(&pkt, &size) < 0)
    return -1;
  /* hand the thread one buffer: size, then the file 32 bytes in (keeps it aligned) */
  uint8_t *buf = malloc(size + 32);
  if (!buf) {
    free(pkt);
    return -1;
  }
  *(int *)buf = size;
  memcpy(buf + 32, pkt, size);
  free(pkt);
  dirty = 0; /* set again below if the write fails */
  async_busy = 1;
  async_done = 0;
  if (!thd_create(1, save_thread, buf)) {
    async_busy = 0;
    free(buf);
    return sw_lib_save();
  }
  return 0;
}

int sw_lib_save_result(int *rv) {
  if (!async_done)
    return 0;
  async_done = 0;
  *rv = async_rv;
  if (async_rv != 0)
    dirty = 1;
  return 1;
}

int sw_lib_early_quality(void) {
  load_stats();
  return prefs.quality == 0;
}

int sw_lib_dirty(void) { return dirty; }
void sw_lib_mark_dirty(void) { dirty = 1; }
int sw_lib_stats_loaded(void) { return loaded; }
sw_prefs *sw_lib_prefs(void) { return &prefs; }

/* ---------- custom collections (COLLECT.TXT, written by SWIRL Card Manager) ---------- */
#define MAX_CUSTOM 24
#define MAX_CUSTOM_ITEMS 256
typedef struct custom_col {
  char name[32];
  int count;
  char (*products)[12];
} custom_col;
static custom_col custom[MAX_CUSTOM];
static int num_custom;

static void load_custom(void) {
  num_custom = 0;
  file_t f = fs_open("/cd/COLLECT.TXT", O_RDONLY);
  if (f == FILEHND_INVALID)
    return;
  int size = fs_total(f);
  if (size <= 0 || size > 256 * 1024) {
    fs_close(f);
    return;
  }
  char *buf = malloc(size + 1);
  if (!buf) {
    fs_close(f);
    return;
  }
  fs_read(f, buf, size);
  fs_close(f);
  buf[size] = 0;
  custom_col *cur = NULL;
  char *save = NULL;
  for (char *line = strtok_r(buf, "\r\n", &save); line; line = strtok_r(NULL, "\r\n", &save)) {
    while (*line == ' ' || *line == '\t') line++;
    int len = strlen(line);
    while (len && (line[len - 1] == ' ' || line[len - 1] == '\t')) line[--len] = 0;
    if (!len || line[0] == '#')
      continue;
    if (line[0] == '[' && line[len - 1] == ']') {
      if (num_custom >= MAX_CUSTOM) {
        cur = NULL;
        continue;
      }
      cur = &custom[num_custom++];
      line[len - 1] = 0;
      snprintf(cur->name, sizeof(cur->name), "%s", line + 1);
      cur->count = 0;
      if (!cur->products)
        cur->products = malloc(MAX_CUSTOM_ITEMS * 12);
      continue;
    }
    if (cur && cur->products && cur->count < MAX_CUSTOM_ITEMS)
      snprintf(cur->products[cur->count++], 12, "%s", line);
  }
  free(buf);
}

/* ---------- library ---------- */
/* Discs of one game: the same disc count and the same serial or, for games whose discs have different
 * serials, the same name (SWIRL Card Manager gives every disc of a game the same proper name). */
static int same_set(const gd_item *a, const gd_item *b) {
  if (a->disc[2] != b->disc[2])
    return 0;
  return !strcmp(a->product, b->product) || !strcasecmp(a->name, b->name);
}

int sw_lib_init(void) {
  num_games = 0;
  int slots = list_slot_count();
  for (int i = 1; i <= slots && num_games < MAX_GAMES; i++) {
    const gd_item *it = list_slot_get(i);
    if (!it || !it->slot_num || !it->name[0])
      continue;
    int disc_num = it->disc[0] - '0';
    int disc_set = it->disc[2] - '0';
    if (disc_set < 1 || disc_set > 9) disc_set = 1;
    if (disc_num < 1 || disc_num > 9) disc_num = 1;
    if (disc_set > 1 && disc_num > 1) {
      /* only skip when disc 1 of the same set exists */
      int found = 0;
      for (int j = 1; j <= slots; j++) {
        const gd_item *o = list_slot_get(j);
        if (o && o != it && o->slot_num && same_set(o, it) && o->disc[0] == '1') {
          found = 1;
          break;
        }
      }
      if (found)
        continue;
    }
    sw_game *g = &games[num_games++];
    g->item = it;
    g->meta = NULL;
    db_get_meta(it->product, &g->meta);
    g->key = fnv1a(it->product, it->name);
    g->discs = (uint8_t)disc_set;
    g->year = 0;
    g->year_ok = 0;
    if (it->date[0] >= '1' && it->date[0] <= '2') {
      g->year = (uint16_t)atoi((char[5]){it->date[0], it->date[1], it->date[2], it->date[3], 0});
      g->year_ok = (g->year >= 1998 && g->year <= 2099);
    }
    g->color = placeholder_colors[g->key % (sizeof(placeholder_colors) / sizeof(placeholder_colors[0]))];
  }
  load_stats();
  load_custom();
  return 0;
}

/* ---------- per game launch settings ---------- */
sw_launch sw_lib_launch_get(const sw_game *g) {
  sw_launch l = {SW_REGION_AUTO, 1, SW_BOOT_NONE};
  sw_stat *s = sw_stat_get(g, 0);
  if (s) {
    l.region = s->flags & 3;
    l.vga = !(s->flags & 4);
    l.boot = (s->flags >> 3) & 3;
  }
  return l;
}

void sw_lib_launch_set(const sw_game *g, sw_launch l) {
  uint8_t f = (uint8_t)((l.region & 3) | (l.vga ? 0 : 4) | ((l.boot & 3) << 3));
  sw_stat *s = sw_stat_get(g, f != 0);
  if (s && s->flags != f) {
    s->flags = f;
    dirty = 1;
  }
}

int sw_lib_last_played(void) {
  int best = -1;
  uint32_t t = 0;
  for (int i = 0; i < num_games; i++) {
    sw_stat *s = sw_stat_get(&games[i], 0);
    if (s && s->last > t) {
      t = s->last;
      best = i;
    }
  }
  return best;
}

int sw_lib_count(void) { return num_games; }

sw_game *sw_lib_game(int idx) {
  if (idx < 0 || idx >= num_games)
    return NULL;
  return &games[idx];
}

sw_stat *sw_stat_get(const sw_game *g, int create) {
  if (!g)
    return NULL;
  for (int i = 0; i < num_stats; i++)
    if (stats[i].key == g->key)
      return &stats[i];
  if (!create)
    return NULL;
  if (num_stats < MAX_STATS) {
    sw_stat *s = &stats[num_stats++];
    memset(s, 0, sizeof(*s));
    s->key = g->key;
    return s;
  }
  /* full: evict the oldest non favourite */
  int victim = -1;
  for (int i = 0; i < num_stats; i++)
    if (!stats[i].fav && (victim < 0 || stats[i].last < stats[victim].last))
      victim = i;
  if (victim < 0)
    return NULL;
  memset(&stats[victim], 0, sizeof(sw_stat));
  stats[victim].key = g->key;
  return &stats[victim];
}

int sw_lib_is_fav(const sw_game *g) {
  sw_stat *s = sw_stat_get(g, 0);
  return s && s->fav;
}

void sw_lib_toggle_fav(const sw_game *g) {
  sw_stat *s = sw_stat_get(g, 1);
  if (s) {
    s->fav = !s->fav;
    dirty = 1;
  }
}

void sw_lib_mark_played(const sw_game *g) {
  sw_stat *s = sw_stat_get(g, 1);
  if (s) {
    if (s->plays < 65535) s->plays++;
    s->last = sw_now();
    dirty = 1;
  }
}

/* ---------- views ---------- */
static int sort_mode;

static int cmp_name(const sw_game *a, const sw_game *b) {
  return strcasecmp(a->item->name, b->item->name);
}

static int cmp_games(const void *pa, const void *pb) {
  const sw_game *a = &games[*(const int *)pa], *b = &games[*(const int *)pb];
  sw_stat *sa = sw_stat_get(a, 0), *sb = sw_stat_get(b, 0);
  switch (sort_mode) {
    case SW_SORT_RECENT: {
      uint32_t la = sa ? sa->last : 0, lb = sb ? sb->last : 0;
      if (la != lb) return la > lb ? -1 : 1;
    } break;
    case SW_SORT_PLAYS: {
      int pa2 = sa ? sa->plays : 0, pb2 = sb ? sb->plays : 0;
      if (pa2 != pb2) return pb2 - pa2;
    } break;
    case SW_SORT_YEAR: {
      int c = strcmp(a->item->date, b->item->date);
      if (c) return c;
    } break;
    default:
      break;
  }
  return cmp_name(a, b);
}

static int finish(int *out, int n, int sort) {
  sort_mode = sort;
  qsort(out, n, sizeof(int), cmp_games);
  return n;
}

int sw_lib_view_all(int *out, int sort) {
  for (int i = 0; i < num_games; i++) out[i] = i;
  return finish(out, num_games, sort);
}

int sw_lib_view_recent(int *out, int max) {
  int n = 0;
  for (int i = 0; i < num_games; i++) {
    sw_stat *s = sw_stat_get(&games[i], 0);
    if (s && s->last)
      out[n++] = i;
  }
  finish(out, n, SW_SORT_RECENT);
  return n > max ? max : n;
}

enum {
  COL_ALL = 0, COL_FAV, COL_RECENT, COL_MOST, COL_PARTY, COL_ONLINE, COL_LIGHTGUN, COL_VGA, COL_IMPORTS, COL_OTHER,
  COL_GENRE = 100, COL_CUSTOM = 200
};

static const char *genre_names[16] = {"Action", "Racing", "Simulation", "Sports", "Light Gun Games", "Fighting",
                                      "Shooter", "Survival", "Adventure", "Platformer", "RPG", "Shoot 'em Up",
                                      "Strategy", "Puzzle", "Arcade", "Music"};

const char *sw_lib_genre_name(const sw_game *g) {
  if (!g || !g->meta || !g->meta->genre)
    return NULL;
  for (int b = 0; b < 16; b++)
    if (g->meta->genre & (1 << b))
      return b == 4 ? "Light Gun" : genre_names[b];
  return NULL;
}

int sw_lib_players(const sw_game *g) {
  return (g && g->meta) ? g->meta->num_players : 0;
}

static int matches(int id, const sw_game *g) {
  sw_stat *s = sw_stat_get(g, 0);
  switch (id) {
    case COL_ALL: return 1;
    case COL_FAV: return s && s->fav;
    case COL_RECENT: return s && s->last;
    case COL_MOST: return s && s->plays;
    case COL_PARTY: return g->meta && g->meta->num_players >= 3;
    case COL_ONLINE: return g->meta && g->meta->network;
    case COL_LIGHTGUN: return g->meta && (g->meta->accessories & ACCESORIES_LIGHTGUN);
    case COL_VGA: return g->item->vga[0] == '1';
    case COL_IMPORTS: return !strchr(g->item->region, 'U');
    case COL_OTHER: return g->meta == NULL;
    default:
      if (id >= COL_CUSTOM && id < COL_CUSTOM + num_custom) {
        const custom_col *c = &custom[id - COL_CUSTOM];
        for (int i = 0; i < c->count; i++)
          if (!strcmp(c->products[i], g->item->product))
            return 1;
        return 0;
      }
      if (id >= COL_GENRE && id < COL_GENRE + 16)
        return g->meta && (g->meta->genre & (1 << (id - COL_GENRE)));
      return 0;
  }
}

static int count_of(int id) {
  int n = 0;
  for (int i = 0; i < num_games; i++) n += matches(id, &games[i]);
  return n;
}

int sw_lib_collections(sw_collection *out, int max) {
  static const struct { int id; const char *name; } fixed[] = {
      {COL_ALL, "All Games"}, {COL_FAV, "Favorites"}, {COL_RECENT, "Recently Played"}, {COL_MOST, "Most Played"},
      {COL_PARTY, "Party Night (3+)"}, {COL_ONLINE, "Online"}, {COL_LIGHTGUN, "Light Gun"}};
  static const struct { int id; const char *name; } tail[] = {
      {COL_VGA, "VGA Compatible"}, {COL_IMPORTS, "Imports"}, {COL_OTHER, "Homebrew & Other"}};
  int n = 0;
  for (unsigned i = 0; i < sizeof(fixed) / sizeof(fixed[0]) && n < max; i++) {
    int c = count_of(fixed[i].id);
    if (c || fixed[i].id == COL_ALL || fixed[i].id == COL_FAV) {
      out[n].id = fixed[i].id;
      out[n].count = c;
      strncpy(out[n].name, fixed[i].name, sizeof(out[n].name) - 1);
      out[n].name[sizeof(out[n].name) - 1] = 0;
      n++;
    }
    if (fixed[i].id == COL_FAV) {
      /* the owner's own collections (made in SWIRL Card Manager) sit right after Favorites */
      for (int k = 0; k < num_custom && n < max; k++) {
        out[n].id = COL_CUSTOM + k;
        out[n].count = count_of(COL_CUSTOM + k);
        snprintf(out[n].name, sizeof(out[n].name), "%s", custom[k].name);
        n++;
      }
    }
  }
  for (int b = 0; b < 16 && n < max; b++) {
    if (b == 4) continue; /* light gun covered above */
    int c = count_of(COL_GENRE + b);
    if (!c) continue;
    out[n].id = COL_GENRE + b;
    out[n].count = c;
    strncpy(out[n].name, genre_names[b], sizeof(out[n].name) - 1);
    out[n].name[sizeof(out[n].name) - 1] = 0;
    n++;
  }
  for (unsigned i = 0; i < sizeof(tail) / sizeof(tail[0]) && n < max; i++) {
    int c = count_of(tail[i].id);
    if (!c) continue;
    out[n].id = tail[i].id;
    out[n].count = c;
    strncpy(out[n].name, tail[i].name, sizeof(out[n].name) - 1);
    out[n].name[sizeof(out[n].name) - 1] = 0;
    n++;
  }
  return n;
}

int sw_lib_view_collection(int id, int *out, int sort) {
  int n = 0;
  for (int i = 0; i < num_games; i++)
    if (matches(id, &games[i]))
      out[n++] = i;
  if (id == COL_RECENT) sort = SW_SORT_RECENT;
  if (id == COL_MOST) sort = SW_SORT_PLAYS;
  return finish(out, n, sort);
}

void sw_lib_disc_list(const sw_game *g, const gd_item **out, int *count, int max) {
  int n = 0;
  int slots = list_slot_count();
  for (int d = 1; d <= 9 && n < max; d++) {
    for (int j = 1; j <= slots; j++) {
      const gd_item *o = list_slot_get(j);
      if (o && o->slot_num && same_set(o, g->item) && (o->disc[0] - '0') == d) {
        out[n++] = o;
        break;
      }
    }
  }
  if (n == 0) {
    out[0] = g->item;
    n = 1;
  }
  *count = n;
}

void sw_lib_format_last_played(uint32_t last, char *out, int len) {
  static const char *wd[7] = {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"};
  static const char *mo[12] = {"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"};
  if (!last) {
    snprintf(out, len, "Never played");
    return;
  }
  uint32_t now = sw_now();
  int d_now = now / 86400, d_last = last / 86400;
  int diff = d_now - d_last;
  if (diff <= 0) snprintf(out, len, "Today");
  else if (diff == 1) snprintf(out, len, "Yesterday");
  else if (diff < 7) snprintf(out, len, "%s", wd[(d_last + 4) % 7]);
  else {
    time_t t = last;
    struct tm tmv;
    gmtime_r(&t, &tmv);
    snprintf(out, len, "%s %d", mo[tmv.tm_mon % 12], tmv.tm_mday);
  }
}

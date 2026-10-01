/*
 * SWIRL: game library model, collections and play stats (saved to VMU as SWIRL.DAT).
 */
#define _DEFAULT_SOURCE
#include "sw_lib.h"
#include "sw_vmu.h"

#include <strings.h>
#include <time.h>

#include <arch/rtc.h>
#include <arch/timer.h>
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
#include "../global_settings.h"
#include "sw_save.h" /* the file layouts, readable on a PC too */
#include "sw_trace.h"

#if __has_include("../openmenu_vmu.h") && __has_include("../openmenu_pal.h")
#include "../openmenu_pal.h"
#include "../openmenu_vmu.h"
#define SW_HAVE_ICON 1
#else
#define SW_HAVE_ICON 0
#endif

#define MAX_GAMES 4096
#define MAX_STATS SW_SAVE_MAX_STATS
/* Two names take turns (see "VMU persistence" below). A save goes to the name that is not holding the newest
   copy; the older copy is removed only once the new one has been read back. */
static const char *const save_names[2] = {"SWIRL.DAT", "SWIRL.BAK"};

static sw_game games[MAX_GAMES];
static int num_games;

static sw_stat stats[MAX_STATS];
static int num_stats;
static sw_prefs prefs = {.clock24 = 0, .rumble = 0, .sort = SW_SORT_NAME, .attract = 1, .accent = 0, .backdrop = 0,
                         .music = 1, .sfx = 1, .resume = 0, .music_vol = 7, .sfx_vol = 6, .saver_style = 0, .saver_min = 5};
static int dirty;
static int loaded;


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

/* ---------- VMU persistence ----------
   SWIRL.DAT (favourites, history, SWIRL's settings) is written by a worker thread so the menu never waits on a
   memory card. The rules that keep a user's data safe:
   - if SWIRL started without reading SWIRL.DAT (no card yet, a slow card, a VM2 switching cards), the file is
     read and merged before the first write, so a save never replaces it with defaults;
   - if the card holding SWIRL.DAT doesn't answer, nothing is written (the save is tried again later);
   - every write is read back and compared; a failed save is reported and tried again by the caller;
   - a save never writes over the copy it is replacing. KallistiOS gives a file that is written over the blocks
     of the old one, so a card pulled (or a VM2 switching) half way through left SWIRL.DAT damaged and the next
     save replaced it with this session's data. Now the new copy goes to the other name (SWIRL.DAT or
     SWIRL.BAK), is read back, and only then is the old copy removed. At start up the newest valid copy wins
     (each carries a write count); a copy that fails its check is ignored, not replaced.
   Space: the card must hold both copies for a moment, so a save needs free blocks for the new copy on top of
   the one already there. A card without that room is reported as "no space", with the blocks needed. */

enum { LOAD_NONE = 0, LOAD_OK, LOAD_NO_FILE, LOAD_DAMAGED, LOAD_NO_ANSWER };
static int load_state;     /* how the last attempt to read SWIRL.DAT went */
static int prefs_touched;  /* settings changed in this session (they win over a file merged in later) */
static int settings_dirty; /* openMenu's settings file (OPENMENU.CFG) also needs writing */
static char saved_on[4];   /* "A1" once a save worked */
static int last_rv;        /* result of the last save */
static int cur_copy = -1;  /* index into save_names of the newest valid copy on the card, -1 for none */
static uint32_t cur_seq;   /* its write count */
static int blocks_short;   /* blocks missing on the card after a save failed for space (-8) */
static int retrying;       /* the menu will try the failed save again by itself */

static void dev_name(maple_device_t *dev, char *out) {
  out[0] = 'A' + dev->port;
  out[1] = '0' + dev->unit;
  out[2] = 0;
}

/* the i-th memory card on the ports */
static maple_device_t *memcard(int i) {
#ifdef SW_TEST_LATE_VMU
  if (timer_ms_gettime64() < 12000) return NULL; /* test only: the cards attach 12 s after power on */
#endif
  return maple_enum_type(i, MAPLE_FUNC_MEMCARD);
}

/* blocks a copy takes on the card, 0 when the name is not there */
static int copy_blocks(maple_device_t *dev, int which) {
  char path[32];
  snprintf(path, sizeof(path), "/vmu/%c%d/%s", 'a' + dev->port, dev->unit, save_names[which]);
  file_t f = fs_open(path, O_RDONLY | O_META);
  if (f == FILEHND_INVALID)
    return 0;
  int size = fs_total(f);
  fs_close(f);
  return size > 0 ? (size + 511) / 512 : 1;
}

/* The card that holds a copy (has_file: bit 0 SWIRL.DAT, bit 1 SWIRL.BAK), else the first card with
   need_blocks free. */
static maple_device_t *find_vmu(int need_blocks, int *has_file) {
  maple_device_t *dev, *first_free = NULL;
  *has_file = 0;
  for (int i = 0; (dev = memcard(i)); i++) {
    int mask = 0;
    for (int w = 0; w < 2; w++)
      if (copy_blocks(dev, w))
        mask |= 1 << w;
    if (mask) {
      *has_file = mask;
      return dev;
    }
    if (!first_free && vmufs_free_blocks(dev) >= need_blocks)
      first_free = dev;
  }
  return first_free;
}

/* reads one copy from dev into a fresh buffer; 0 on success */
static int read_file(maple_device_t *dev, int which, uint8_t **out, int *out_size) {
  char path[32];
  snprintf(path, sizeof(path), "/vmu/%c%d/%s", 'a' + dev->port, dev->unit, save_names[which]);
  file_t f = fs_open(path, O_RDONLY | O_META);
  if (f == FILEHND_INVALID)
    return -1;
  int size = fs_total(f);
  uint8_t *buf = size > 0 && size <= 64 * 1024 ? malloc(size) : NULL;
  int ok = buf && fs_read(f, buf, size) == size;
  fs_close(f);
  if (!ok) {
    free(buf);
    return -1;
  }
  *out = buf;
  *out_size = size;
  return 0;
}

/* decodes a SWIRL.DAT package into p/st/n; 0 on success. upgraded: 1 for a layout older than the current one
   (written back in the current one at the first save) */
static int parse_save(const uint8_t *buf, int size, sw_prefs *p, sw_stat *st, int *n, int *upgraded,
                      uint32_t *seq) {
  vmu_pkg_t pkg;
  *upgraded = 0;
  *seq = 0;
  if (vmu_pkg_parse((uint8_t *)buf, size, &pkg) != 0)
    return -1;
  int layout = 0;
  if (sw_save_parse(pkg.data, pkg.data_len, p, st, n, &layout, seq) != 0)
    return -1;
  if (layout != SW_SAVE_LAYOUT_CURRENT) *upgraded = 1;
  return 0;
}

/* Reads the newest valid copy of SWIRL.DAT and takes it in. At start up (merge 0) it simply replaces the
   defaults. Later (merge 1) it is combined with what changed since: play counts add up, a favourite on either
   side stays a favourite, and settings changed in this session are kept. Sets load_state; returns it. */
static int take_in_file(int merge) {
  int has_file = 0;
  maple_device_t *dev = find_vmu(0, &has_file);
  if (!dev || !has_file) {
    load_state = LOAD_NO_FILE; /* no memory card at all, or none with a copy */
    return load_state;
  }
  static sw_stat fstats[MAX_STATS];
  sw_prefs fprefs = prefs;
  int fn = 0, upgraded = 0, best = -1, damaged = 0, silent = 0;
  uint32_t best_seq = 0;
  for (int w = 0; w < 2; w++) {
    if (!(has_file & (1 << w)))
      continue;
    uint8_t *buf = NULL;
    int size = 0;
    if (read_file(dev, w, &buf, &size) != 0) {
      printf("SWIRL: %s on %c%d did not answer\n", save_names[w], 'A' + dev->port, dev->unit);
      silent++;
      continue;
    }
    static sw_stat cstats[MAX_STATS];
    sw_prefs cprefs = prefs;
    int cn = 0, cup = 0;
    uint32_t cseq = 0;
    int rc = parse_save(buf, size, &cprefs, cstats, &cn, &cup, &cseq);
    free(buf);
    if (rc != 0) {
      printf("SWIRL: %s on %c%d is damaged\n", save_names[w], 'A' + dev->port, dev->unit);
      damaged++;
      continue;
    }
    /* the newest copy wins; SWIRL.DAT on a tie (two copies from before the write count) */
    if (best < 0 || cseq > best_seq) {
      best = w;
      best_seq = cseq;
      fprefs = cprefs;
      fn = cn;
      upgraded = cup;
      memcpy(fstats, cstats, cn * sizeof(sw_stat));
    }
  }
  if (best < 0) {
    load_state = silent ? LOAD_NO_ANSWER : LOAD_DAMAGED;
    return load_state;
  }
  if (damaged)
    sw_warn(SW_WARN_DAT_DAMAGED, "SWIRL.DAT: one copy damaged, using %s", save_names[best]);
  cur_copy = best;
  cur_seq = best_seq;
  if (upgraded) dirty = 1; /* an older layout: write it back in the current one */
  if (!merge) {
    prefs = fprefs;
    num_stats = fn;
    memcpy(stats, fstats, fn * sizeof(sw_stat));
  } else {
    if (!prefs_touched)
      prefs = fprefs;
    for (int i = 0; i < fn; i++) {
      sw_stat *m = NULL;
      for (int j = 0; j < num_stats; j++)
        if (stats[j].key == fstats[i].key) {
          m = &stats[j];
          break;
        }
      if (m) {
        uint32_t plays = (uint32_t)m->plays + fstats[i].plays;
        m->plays = plays > 65535 ? 65535 : (uint16_t)plays;
        m->fav = m->fav || fstats[i].fav;
        if (!m->flags) m->flags = fstats[i].flags;
        if (fstats[i].last > m->last) m->last = fstats[i].last;
      } else if (num_stats < MAX_STATS) {
        stats[num_stats++] = fstats[i];
      }
    }
    printf("SWIRL: merged %s from %c%d (%d entries)\n", save_names[best], 'A' + dev->port, dev->unit, fn);
  }
  if (upgraded) dirty = 1;
  loaded = 1;
  load_state = LOAD_OK;
  return load_state;
}

static void load_stats(void) {
  static const char *const names[] = {"none", "ok", "no file", "damaged", "no answer"};
  const int st = take_in_file(0);
  if (st == LOAD_OK)
    sw_trace("SWIRL.DAT: ok (%s, write %lu)", save_names[cur_copy], (unsigned long)cur_seq);
  else if (st == LOAD_DAMAGED)
    sw_warn(SW_WARN_DAT_DAMAGED, "SWIRL.DAT: damaged, replaced at the next save");
  else
    sw_trace("SWIRL.DAT: %s", st >= 0 && st <= 4 ? names[st] : "?");
}

/* builds the VMU file for the current stats; caller frees *out */
static int build_save(uint8_t **out, int *out_size, uint32_t seq) {
  static uint8_t raw[8 + 16 + sizeof(sw_stat) * MAX_STATS + 8 + 8]; /* sw_save_size(MAX_STATS) */
  memset(raw, 0, sizeof(raw));
  const int data_len = sw_save_build(raw, &prefs, stats, num_stats, seq);

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
  pkg.data = raw;
  return vmu_pkg_build(&pkg, out, out_size) < 0 ? -1 : 0;
}

/* runs on the worker: write the new copy to the free name, read it back, then remove the old copy.
   0, -2 no card with room, -4 the write failed, -6 read back differs, -8 no room on the card that holds the
   copies (blocks_short says how many blocks are missing) */
static int write_save(uint8_t *out, int out_size, uint32_t seq, char *where) {
  const int need = (out_size + 511) / 512;
  int has_file = 0;
  maple_device_t *dev = find_vmu(need, &has_file);
  if (!dev)
    return -2;
  const int target = cur_copy < 0 ? 0 : 1 - cur_copy;
  const int other = 1 - target;
  if (has_file) {
    /* the old copy stays until the new one is checked, so the new one needs room of its own. A stale file under
       the target name (a removal that did not finish) is written over and its blocks count as free. */
    const int room = vmufs_free_blocks(dev) + ((has_file & (1 << target)) ? copy_blocks(dev, target) : 0);
    if (room < need) {
      blocks_short = need - room;
      printf("SWIRL: %s needs %d more blocks on %c%d\n", save_names[target], blocks_short, 'A' + dev->port, dev->unit);
      return -8;
    }
  }
  sw_trace("save: %s write %lu on %c%d (%d blocks, %d free)", save_names[target], (unsigned long)seq, 'A' + dev->port,
           dev->unit, need, vmufs_free_blocks(dev));
  int rv = vmufs_write(dev, save_names[target], out, out_size, VMUFS_OVERWRITE);
  if (rv == -7) {
    blocks_short = 1;
    return -8;
  }
  if (rv < 0)
    return -4;
  uint8_t *back = NULL;
  int back_size = 0;
  if (read_file(dev, target, &back, &back_size) != 0 || back_size < out_size || memcmp(back, out, out_size) != 0) {
    printf("SWIRL: %s on %c%d did not read back the same\n", save_names[target], 'A' + dev->port, dev->unit);
    free(back);
    return -6;
  }
  free(back);
  cur_copy = target;
  cur_seq = seq;
  dev_name(dev, where);
  if (has_file & (1 << other)) {
    /* the new copy is safe on the card: the old one goes. If this fails both stay, and the write count picks
       the newer one next time. */
    int drv = vmufs_delete(dev, save_names[other]);
    sw_trace("save: removed old %s: %d, %d free", save_names[other], drv, vmufs_free_blocks(dev));
    if (drv < 0)
      printf("SWIRL: could not remove the old %s on %s\n", save_names[other], where);
  }
  return 0;
}

static volatile int async_busy, async_done, async_rv;
static char async_where[4];

typedef struct save_job {
  int size;
  int settings; /* also write openMenu's settings file */
  uint8_t *data;
  uint32_t seq;
} save_job;

static void *save_thread(void *arg) {
  save_job *job = arg;
  sw_trace("save: start (SWIRL.DAT %s, settings %s)", job->data ? "yes" : "no", job->settings ? "yes" : "no");
  int rv = job->data ? write_save(job->data, job->size, job->seq, async_where) : 0;
  sw_trace("save: SWIRL.DAT done (%d)", rv);
  if (job->settings)
    settings_save();
  sw_trace("save: finished");
  free(job->data);
  free(job);
  async_rv = rv;
  async_done = 1;
  async_busy = 0;
  return NULL;
}

/* Starts a save on the worker. Returns 0 when started, -1 if one is running, -7 if SWIRL.DAT couldn't be read
   first (then nothing is written: the file is left as it is). */
static int start_save(void) {
  if (async_busy)
    return -1;
  if (!loaded) {
    /* SWIRL started without its file: take it in before writing over it */
    int st = take_in_file(1);
    if (st == LOAD_NO_ANSWER)
      return -7;
    if (st == LOAD_DAMAGED)
      printf("SWIRL: replacing a damaged SWIRL.DAT\n");
    loaded = 1; /* from now on this session's data is the whole story */
  }
  save_job *job = calloc(1, sizeof(*job));
  if (!job)
    return -1;
  uint8_t *pkt = NULL;
  if (dirty) {
    job->seq = cur_seq + 1;
    if (build_save(&pkt, &job->size, job->seq) < 0) {
      free(job);
      return -1;
    }
    job->data = pkt;
  }
  /* the VMU screen shows "writing" now and is left alone until the save is done */
  sw_vmu_before_write();
  job->settings = settings_dirty;
  dirty = 0;          /* set again if the write fails */
  settings_dirty = 0; /* likewise */
  async_busy = 1;
  async_done = 0;
  if (!thd_create(1, save_thread, job)) {
    async_busy = 0;
    free(job->data);
    free(job);
    dirty = 1;
    return -1;
  }
  return 0;
}

int sw_lib_save_async(void) {
  return start_save();
}

/* A save that finished (or failed) and was not reported yet. */
int sw_lib_save_result(int *rv) {
  if (!async_done)
    return 0;
  async_done = 0;
  *rv = last_rv = async_rv;
  if (async_rv != 0) {
    dirty = 1;
    if (async_rv == -2 || async_rv == -8)
      sw_warn(SW_WARN_VMU_FULL, "save: %s", async_rv == -2 ? "no VMU with room" : "no room on the VMU that holds SWIRL.DAT");
    else
      sw_warn(SW_WARN_SAVE_FAILED, "save: result %d", async_rv);
  } else {
    memcpy(saved_on, async_where, sizeof(saved_on));
  }
  return 1;
}

/* Only one part of SWIRL uses the memory card at a time. Anything else that needs it (leaving SWIRL, the VMU
   manager) first lets a save in progress finish: a save is never cut short, because a half written file is
   worse than a short wait. KallistiOS gives every memory card read and write its own 100 ms limit, so a card
   that stops answering ends the save with an error instead of holding it. While waiting, the menu keeps
   drawing (idle_fn) so the screen doesn't look frozen. */
static void (*idle_fn)(void);
void sw_lib_set_idle(void (*fn)(void)) { idle_fn = fn; }

void sw_lib_finish(void) {
  if (!async_busy)
    return;
  const uint64_t t0 = timer_ms_gettime64();
  while (async_busy) {
    if (idle_fn)
      idle_fn();
    else
      thd_sleep(10);
  }
  sw_trace("waited %u ms for a save to finish", (unsigned)(timer_ms_gettime64() - t0));
}

/* Saves now and waits until it is done: used before leaving SWIRL (a game, the BIOS, another style).
   0 on success. */
int sw_lib_save(void) {
  int rv;
  sw_lib_finish();
  sw_lib_save_result(&rv); /* take in the result of that one (a failure marks the data unsaved again) */
  if (!dirty && !settings_dirty)
    return 0;
  rv = start_save();
  if (rv != 0)
    return rv;
  sw_lib_finish();
  sw_lib_save_result(&rv);
  return rv;
}

int sw_lib_busy(void) { return async_busy; }

void sw_lib_settings_dirty(void) { settings_dirty = 1; }

/* diagnostic A/B: 1 = an automatic save writes exactly what System > Save to VMU writes (both files) */
int sw_autosave_like_manual = 0; /* hardware tests: both work; the lighter SWIRL.DAT-only save stays */

/* one line for System > Save: where the data is, or why it isn't saved */
const char *sw_lib_save_status(char *buf, int len) {
  if (async_busy)
    return "Saving...";
  if (dirty || settings_dirty) {
    if (last_rv == -2) return "No VMU with space";
    if (last_rv == -8) {
      snprintf(buf, len, "Needs %d free block%s", blocks_short, blocks_short == 1 ? "" : "s");
      return buf;
    }
    if (last_rv == -7) return retrying ? "VMU busy, retrying" : "VMU busy, not saved";
    if (last_rv) return retrying ? "Not saved, retrying" : "Not saved: check the VMU";
    return "Unsaved changes";
  }
  if (saved_on[0]) {
    snprintf(buf, len, "Saved on VMU %s", saved_on);
    return buf;
  }
  if (load_state == LOAD_OK) return "Saved";
  if (load_state == LOAD_DAMAGED) return "Damaged, will fix";
  if (load_state == LOAD_NO_ANSWER) return "VMU not answering";
  return "Not saved yet";
}

/* A memory card that attached after the start up scan (see settings_late_card in global_settings.c). While
   SWIRL's file has not been read, a card that turns up is looked at; its file is merged in the same way as
   before a first write. 1 when a file was taken in (the menu rebuilds its views and applies the settings). */
int sw_lib_late_card(void) {
  static int cards_seen = -1;
  int n = 0;
  while (memcard(n)) n++;
  if (cards_seen < 0) {
    cards_seen = n;
    return 0;
  }
  if (n <= cards_seen)
    return 0;
  cards_seen = n;
  if (loaded)
    return 0;
  const int st = take_in_file(1);
  if (st != LOAD_OK) {
    sw_trace("SWIRL.DAT: memory card attached late, %s", st == LOAD_NO_FILE ? "no file on it" : st == LOAD_DAMAGED ? "damaged" : "no answer");
    return 0;
  }
  sw_warn(SW_WARN_LATE_CARD, "SWIRL.DAT read from %s after start up (write %lu)", save_names[cur_copy], (unsigned long)cur_seq);
  return 1;
}

int sw_lib_early_quality(void) {
  load_stats();
  return prefs.quality == 0;
}

int sw_lib_dirty(void) { return dirty || settings_dirty; }
int sw_lib_blocks_short(void) { return blocks_short; }
void sw_lib_retrying(int on) { retrying = on; }
void sw_lib_mark_dirty(void) {
  dirty = 1;
  prefs_touched = 1;
}
int sw_lib_stats_loaded(void) { return loaded; }
sw_prefs *sw_lib_prefs(void) { return &prefs; }
void sw_lib_note_style_set(void) {
  prefs.swirl_style_set = 1;
  dirty = 1; /* not prefs_touched: a file merged in later still brings its own settings */
}
int sw_gameid_enabled(void) { return !prefs.gameid_off; }

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

/* the sections a GDMENU card's dividers make: each holds the games whose slots follow it, up to the next */
#define MAX_SECTIONS 24
static struct {
  char name[32];
  unsigned first, last; /* slot numbers the section covers (last is the slot before the next divider) */
} sections[MAX_SECTIONS];
static int num_sections;

int sw_lib_init(void) {
  num_games = 0;
  num_sections = 0;
  int slots = list_slot_count();
  for (int i = 1; i <= slots && num_games < MAX_GAMES; i++) {
    const gd_item *it = list_slot_get(i);
    if (!it || !it->slot_num || !it->name[0])
      continue;
    char label[32];
    if (sw_lib_divider_label(it->name, label, sizeof(label)) >= 0) {
      if (num_sections < MAX_SECTIONS) {
        if (num_sections) sections[num_sections - 1].last = it->slot_num - 1;
        snprintf(sections[num_sections].name, sizeof(sections[num_sections].name), "%s",
                 label[0] ? label : "Section");
        sections[num_sections].first = it->slot_num + 1;
        sections[num_sections].last = 0xFFFFFFFFu;
        num_sections++;
      }
      continue; /* a divider is not a game */
    }
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
  /* SWIRL.DAT was already read at power on (for the picture quality); read it again only if that didn't work,
     for example a memory card that answered late. Each read takes about half a second. */
  if (!loaded && load_state != LOAD_DAMAGED) /* a damaged file does not heal; it is replaced by the first save */
    load_stats();
  load_custom();
  sw_trace_games(num_games);
  return 0;
}

/* ---------- per game launch settings ---------- */
/* Launch options live in sw_stat.flags: bits 0-1 region, bit 2 "do not force VGA", bits 3-4 the start mode, bit 5
   "the start mode was chosen". Files written before 2.14 have bit 5 clear: a start mode of 1 to 3 there was chosen
   and is kept; 0 was the old default (straight to the game) and now means the default, the full start. */
#define SW_FLAG_BOOT_SET 0x20

sw_launch sw_lib_launch_get(const sw_game *g) {
  sw_launch l = {SW_REGION_AUTO, 1, SW_BOOT_DEFAULT};
  sw_stat *s = sw_stat_get(g, 0);
  if (s) {
    l.region = s->flags & 3;
    l.vga = !(s->flags & 4);
    const int boot = (s->flags >> 3) & 3;
    if ((s->flags & SW_FLAG_BOOT_SET) || boot != SW_BOOT_NONE)
      l.boot = boot;
  }
  return l;
}

void sw_lib_launch_set(const sw_game *g, sw_launch l) {
  uint8_t f = 0;
  if (l.region != SW_REGION_AUTO || !l.vga || l.boot != SW_BOOT_DEFAULT)
    f = (uint8_t)((l.region & 3) | (l.vga ? 0 : 4) | ((l.boot & 3) << 3) | SW_FLAG_BOOT_SET);
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
    case SW_SORT_CARD: /* the order the card was written in (GDMENU slot numbers) */
      if (a->item->slot_num != b->item->slot_num) return a->item->slot_num < b->item->slot_num ? -1 : 1;
      break;
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
  COL_GENRE = 100, COL_CUSTOM = 200, COL_SECTION = 300 /* the card's dividers, see sw_lib_divider_label */
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
      if (id >= COL_SECTION && id < COL_SECTION + num_sections) {
        const unsigned slot = g->item->slot_num;
        return slot >= sections[id - COL_SECTION].first && slot <= sections[id - COL_SECTION].last;
      }
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
      /* then the card's own sections, in card order, so a divided card reads the way its owner laid it out */
      for (int k = 0; k < num_sections && n < max; k++) {
        int c = count_of(COL_SECTION + k);
        if (!c) continue;
        out[n].id = COL_SECTION + k;
        out[n].count = c;
        snprintf(out[n].name, sizeof(out[n].name), "%s", sections[k].name);
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
  if (id >= COL_SECTION && id < COL_SECTION + num_sections && sort == SW_SORT_NAME) sort = SW_SORT_CARD;
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

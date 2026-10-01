/* SWIRL.DAT layouts: see sw_save.h. Plain C, no KallistiOS, so tests/host/save_check.c can build it. */
#include "sw_save.h"

#include <string.h>

/* the first SW_PREFS_HEAD bytes of sw_prefs sit here; the rest follow the tail in a PRF4 block */
#define SW_PREFS_HEAD 16
typedef struct __attribute__((packed)) save_blob {
  char magic[4];
  uint16_t version;
  uint16_t count;
  uint8_t prefs[SW_PREFS_HEAD];
} save_blob;

/* version 4: after the tail, the settings added since the prefs block was fixed at 16 bytes */
typedef struct __attribute__((packed)) save_more {
  char tag[4]; /* "PRF4" */
  uint8_t prefs[4]; /* sw_prefs bytes 16 to 19: logo, start, video, reserved */
} save_more;

typedef struct __attribute__((packed)) save_blob_v1 {
  char magic[4];
  uint16_t version;
  uint16_t count;
  uint8_t prefs[4];
} save_blob_v1;

/* After the stats: which write this is. Shorter than one sw_stat, so a SWIRL from before it never reads it as
   a stat. */
typedef struct __attribute__((packed)) save_tail {
  char tag[4]; /* "SEQ1" */
  uint32_t seq;
} save_tail;

/* The layout is the byte sizes below. Changing sw_prefs or sw_stat changes the file: bump SW_SAVE_VERSION, keep a
   reader for the old size in sw_save_parse, and add a file from the previous release to tests/host/savefiles.
   Then update these numbers. */
_Static_assert(SW_SAVE_VERSION == 4, "new SWIRL.DAT layout: update the readers, the fixtures and this check");
_Static_assert(sizeof(sw_prefs) == SW_PREFS_HEAD + 4, "sw_prefs changed size: that is a new SWIRL.DAT layout (see sw_save.h)");
_Static_assert(sizeof(sw_stat) == 12, "sw_stat changed size: that is a new SWIRL.DAT layout (see sw_save.h)");
_Static_assert(sizeof(save_tail) == 8, "save_tail changed size: that is a new SWIRL.DAT layout (see sw_save.h)");
_Static_assert(sizeof(save_blob) == 8 + SW_PREFS_HEAD, "save_blob is not packed the way the file is");
_Static_assert(sizeof(save_more) == 8, "save_more changed size: that is a new SWIRL.DAT layout (see sw_save.h)");

int sw_save_size(int n) {
  return (int)sizeof(save_blob) + n * (int)sizeof(sw_stat) + (int)sizeof(save_tail) + (int)sizeof(save_more);
}

int sw_save_parse(const uint8_t *data, int len, sw_prefs *p, sw_stat *st, int *n, int *layout, uint32_t *seq) {
  *n = 0;
  *layout = 0;
  *seq = 0;
  memset(p, 0, sizeof(*p));
  if (len < (int)sizeof(save_blob_v1))
    return -1;
  const save_blob *b = (const save_blob *)data;
  const int count = b->count > SW_SAVE_MAX_STATS ? SW_SAVE_MAX_STATS : b->count;
  if (!memcmp(b->magic, "SWL2", 4)) {
    memcpy(p, b->prefs, SW_PREFS_HEAD);
    if (p->saver_min < 1 || p->saver_min > 30) p->saver_min = 5; /* saves from before the screen saver */
    const int room = (len - (int)sizeof(save_blob)) / (int)sizeof(sw_stat);
    *n = count;
    if (*n > room) *n = room < 0 ? 0 : room;
    memcpy(st, data + sizeof(save_blob), *n * sizeof(sw_stat));
    const int tail_at = (int)sizeof(save_blob) + *n * (int)sizeof(sw_stat);
    int has_more = 0;
    if (len >= tail_at + (int)sizeof(save_tail)) {
      save_tail t;
      memcpy(&t, data + tail_at, sizeof(t));
      if (!memcmp(t.tag, "SEQ1", 4)) {
        *seq = t.seq;
        const int more_at = tail_at + (int)sizeof(save_tail);
        if (len >= more_at + (int)sizeof(save_more)) {
          save_more m;
          memcpy(&m, data + more_at, sizeof(m));
          if (!memcmp(m.tag, "PRF4", 4)) {
            memcpy((uint8_t *)p + SW_PREFS_HEAD, m.prefs, sizeof(m.prefs));
            has_more = 1;
          }
        }
      }
    }
    if (p->logo >= SW_LOGO_COUNT) p->logo = SW_LOGO_REGION;
    if (p->start >= SW_START_COUNT) p->start = SW_START_BOTH;
    if (p->video > 1) p->video = 0;
    *layout = b->version < 3 ? SW_SAVE_LAYOUT_V2 : has_more ? SW_SAVE_LAYOUT_CURRENT : SW_SAVE_LAYOUT_V3;
    return 0;
  }
  if (!memcmp(b->magic, "SWL1", 4)) {
    /* older SWIRL: keep favourites, history and the four original settings */
    const save_blob_v1 *o = (const save_blob_v1 *)data;
    p->clock24 = o->prefs[0];
    p->rumble = o->prefs[1];
    p->sort = o->prefs[2];
    p->attract = o->prefs[3];
    p->saver_min = 5;
    const int room = (len - (int)sizeof(save_blob_v1)) / (int)sizeof(sw_stat);
    *n = count;
    if (*n > room) *n = room < 0 ? 0 : room;
    memcpy(st, data + sizeof(save_blob_v1), *n * sizeof(sw_stat));
    for (int i = 0; i < *n; i++) st[i].flags = 0;
    *layout = SW_SAVE_LAYOUT_V1;
    return 0;
  }
  return -1;
}

int sw_save_build(uint8_t *out, const sw_prefs *p, const sw_stat *st, int n, uint32_t seq) {
  save_blob *blob = (save_blob *)out;
  memcpy(blob->magic, "SWL2", 4);
  blob->version = SW_SAVE_VERSION;
  blob->count = (uint16_t)n;
  memcpy(blob->prefs, p, SW_PREFS_HEAD);
  int len = (int)sizeof(save_blob);
  memcpy(out + len, st, n * sizeof(sw_stat));
  len += n * (int)sizeof(sw_stat);
  save_tail tail;
  memcpy(tail.tag, "SEQ1", 4);
  tail.seq = seq;
  memcpy(out + len, &tail, sizeof(tail));
  len += (int)sizeof(tail);
  save_more more;
  memcpy(more.tag, "PRF4", 4);
  memcpy(more.prefs, (const uint8_t *)p + SW_PREFS_HEAD, sizeof(more.prefs));
  memcpy(out + len, &more, sizeof(more));
  return len + (int)sizeof(more);
}

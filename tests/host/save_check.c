/* SWIRL host check for the SWIRL.DAT reader (ui/swirl/sw_save.c): every file in tests/host/savefiles, written by
   a released SWIRL, must read back with its two favourites, and a file written now must read back the same.
   Built by run.sh with the sanitizers. Usage: save_check <file> ... */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "ui/swirl/sw_save.h"

/* keys of the two favourites in the test library: fnv1a(product, name) as sw_lib.c computes it */
static uint32_t fnv1a(const char *a, const char *b) {
  uint32_t h = 2166136261u;
  for (; a && *a; a++) h = (h ^ (uint8_t)*a) * 16777619u;
  h = (h ^ '|') * 16777619u;
  for (; b && *b; b++) h = (h ^ (uint8_t)*b) * 16777619u;
  return h ? h : 1;
}

static int check(const char *path) {
  FILE *f = fopen(path, "rb");
  if (!f) {
    printf("%s: cannot open\n", path);
    return 1;
  }
  uint8_t buf[4096];
  int len = (int)fread(buf, 1, sizeof(buf), f);
  fclose(f);
  sw_prefs p;
  sw_stat st[SW_SAVE_MAX_STATS];
  int n = 0, layout = 0;
  uint32_t seq = 0;
  if (sw_save_parse(buf, len, &p, st, &n, &layout, &seq) != 0) {
    printf("%s: not read\n", path);
    return 1;
  }
  const uint32_t want[2] = {fnv1a("SWT0024", "BEATS OF RAGE"), fnv1a("SWT0002", "CRAZY TAXI")};
  int favs = 0, found = 0;
  for (int i = 0; i < n; i++) {
    favs += st[i].fav != 0;
    for (int w = 0; w < 2; w++)
      if (st[i].key == want[w] && st[i].fav) found++;
  }
  printf("%s: layout %d, %d stats, %d favourites, seq %lu, music %d sfx %d quality %d saver %d min logo %d start %d video %d\n",
         path, layout, n, favs, (unsigned long)seq, p.music, p.sfx, p.quality, p.saver_min, p.logo, p.start, p.video);
  if (layout < SW_SAVE_LAYOUT_V4 && (p.logo != 0 || p.start != 0 || p.video != 0 || p.lang)) {
    printf("  an older layout must read with the version 4 bytes at zero\n");
    return 1;
  }
  if (layout < SW_SAVE_LAYOUT_CURRENT && (p.motion_off != 0 || p.picture != 0 || p.pic_dim != 4 || p.pic_motion != 0)) {
    printf("  an older layout must read with the version 5 bytes at their defaults (dim 4)\n");
    return 1;
  }
  if (found != 2 || favs != 2) {
    printf("  expected the two favourites of the test library\n");
    return 1;
  }
  /* what the current writer makes of it reads back the same */
  uint8_t out[4096];
  const int olen = sw_save_build(out, &p, st, n, seq + 1);
  sw_prefs p2;
  sw_stat st2[SW_SAVE_MAX_STATS];
  int n2 = 0, layout2 = 0;
  uint32_t seq2 = 0;
  if (sw_save_parse(out, olen, &p2, st2, &n2, &layout2, &seq2) != 0 || n2 != n || layout2 != SW_SAVE_LAYOUT_CURRENT ||
      seq2 != seq + 1 || memcmp(&p2, &p, sizeof(p)) != 0 || memcmp(st2, st, n * sizeof(sw_stat)) != 0) {
    printf("  written again, it does not read back the same\n");
    return 1;
  }
  if (olen != sw_save_size(n)) {
    printf("  sw_save_size disagrees with sw_save_build\n");
    return 1;
  }
  /* what an older SWIRL sees in the file written today: the same stats at the same place, the tail after
     them, and nothing it reads moved (2.14.3 and 2.13 both stop at the tail) */
  if (memcmp(out + 8 + 16, st, n * sizeof(sw_stat)) != 0 || memcmp(out + 8 + 16 + n * sizeof(sw_stat), "SEQ1", 4) != 0 ||
      memcmp(out + 8 + 16 + n * sizeof(sw_stat) + 8, "PRF4", 4) != 0) {
    printf("  the stats or the tail moved: an older SWIRL would lose the favourites\n");
    return 1;
  }
  /* the file cut after PRF4 (what a 2.15 or 2.16 menu writes back) reads as a version 4 file with the
     2.17 settings at their defaults */
  {
    sw_prefs p4;
    sw_stat st4[SW_SAVE_MAX_STATS];
    int n4 = 0, layout4 = 0;
    uint32_t seq4 = 0;
    if (sw_save_parse(out, 8 + 16 + n * (int)sizeof(sw_stat) + 16, &p4, st4, &n4, &layout4, &seq4) != 0 || n4 != n ||
        layout4 != SW_SAVE_LAYOUT_V4 || p4.logo != p.logo || p4.lang != p.lang || p4.pic_dim != 4 || p4.picture != 0) {
      printf("  the file without its PRF5 block does not read as a version 4 file\n");
      return 1;
    }
  }
  /* the file cut at the tail (what an older version would write back) still reads, with the new settings at
     their defaults */
  sw_prefs p3;
  sw_stat st3[SW_SAVE_MAX_STATS];
  int n3 = 0, layout3 = 0;
  uint32_t seq3 = 0;
  if (sw_save_parse(out, 8 + 16 + n * (int)sizeof(sw_stat) + 8, &p3, st3, &n3, &layout3, &seq3) != 0 || n3 != n ||
      layout3 != SW_SAVE_LAYOUT_V3 || seq3 != seq + 1 || memcmp(st3, st, n * sizeof(sw_stat)) != 0 || p3.logo != 0) {
    printf("  the file without its PRF4 block does not read as a version 3 file\n");
    return 1;
  }
  return 0;
}

int main(int argc, char **argv) {
  int fail = 0;
  for (int i = 1; i < argc; i++) fail |= check(argv[i]);
  if (!fail) printf("save check: %d files pass\n", argc - 1);
  return fail;
}

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
  printf("%s: layout %d, %d stats, %d favourites, seq %lu, music %d sfx %d quality %d saver %d min\n", path, layout, n,
         favs, (unsigned long)seq, p.music, p.sfx, p.quality, p.saver_min);
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
  return 0;
}

int main(int argc, char **argv) {
  int fail = 0;
  for (int i = 1; i < argc; i++) fail |= check(argv[i]);
  if (!fail) printf("save check: %d files pass\n", argc - 1);
  return fail;
}

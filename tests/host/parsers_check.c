/* SWIRL host check for the inherited parsers: OPENMENU.INI (gd_list.c + inih) and the DAT reader.
   Built with -fsanitize=address,undefined by run.sh; a memory error aborts, so a clean exit is the pass.
   Usage: parsers_check ini <file> [expected games]   |   parsers_check dat <file> */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "backend/gd_item.h"
#include "backend/gd_list.h"
#include "inc/dat_format.h"

static int check_ini(const char *path, int expected) {
  int r = list_read(path);
  int n = list_length();
  printf("%s: ret %d, %d slots, %d games\n", path, r, list_slot_count(), n);
  for (int i = 0; i < n; i++) {
    const gd_item *it = list_item_get(i);
    if (!it)
      return 1;
    (void)strlen(it->name);
  }
  /* the multidisc set for the first game, and a serial that is not there */
  if (n > 0)
    list_set_multidisc(list_item_get(0)->product);
  list_set_multidisc("NOTHERE");
  list_destroy();
  if (expected >= 0 && n != expected) {
    printf("  expected %d games\n", expected);
    return 1;
  }
  return 0;
}

static int check_dat(const char *path) {
  dat_file d;
  DAT_init(&d);
  int r = DAT_load_parse(&d, path);
  printf("%s: ret %d, %u entries of %u bytes\n", path, r, d.num_chunks, d.chunk_size);
  if (r == 0 && d.num_chunks) {
    void *buf = malloc(d.chunk_size);
    if (!buf)
      return 1;
    DAT_read_file_by_ID(&d, d.items[0].ID, buf);
    DAT_read_file_by_ID(&d, "NOTHERE", buf);
    DAT_read_file_by_num(&d, d.num_chunks, buf);
    free(buf);
  }
  if (d.handle)
    fclose((FILE *)d.handle);
  free(d.items);
  return 0;
}

int main(int argc, char **argv) {
  if (argc < 3) {
    fprintf(stderr, "usage: parsers_check ini|dat <file> [expected games]\n");
    return 2;
  }
  if (strcmp(argv[1], "ini") == 0)
    return check_ini(argv[2], argc > 3 ? atoi(argv[3]) : -1);
  if (strcmp(argv[1], "dat") == 0)
    return check_dat(argv[2]);
  return 2;
}

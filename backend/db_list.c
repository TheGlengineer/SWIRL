/*
 * File: db_list.c
 * Project: backend
 * File Created: Wednesday, 16th June 2021 10:32:39 pm
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#include "db_list.h"

#include <stdio.h>
#include <stdlib.h>

#include "../gdrom/gdrom_fs.h"
#include "../inc/dat_format.h"
#include "../texture/serial_sanitize.h"
#include "../ui/swirl/sw_trace.h"
#include "db_item.h"

static dat_file dat_meta;
static db_item *db;
static uint32_t dat_first_index;
static uint32_t db_count; /* SWIRL: records actually read */

int db_load_DAT(void) {
  DAT_init(&dat_meta);
  db = NULL;
  db_count = 0;

  /* SWIRL: a missing, empty or odd META.DAT leaves an empty database ("No description"), never a NULL read.
     The records are read as one block from the first entry's chunk, as before, so the chunk size must be a
     record. */
  if (DAT_load_parse(&dat_meta, "META.DAT") != 0)
    return 0;
  if (dat_meta.num_chunks == 0) {
    sw_trace("META.DAT: no entries");
    fclose((FD_TYPE)dat_meta.handle);
    dat_meta.handle = NULL;
    return 0;
  }
  if (dat_meta.chunk_size != sizeof(db_item)) {
    sw_trace("META.DAT: record size %u, expected %u, not used", (unsigned)dat_meta.chunk_size, (unsigned)sizeof(db_item));
    fclose((FD_TYPE)dat_meta.handle);
    dat_meta.handle = NULL;
    dat_meta.num_chunks = 0;
    return 0;
  }
  dat_first_index = dat_meta.items[0].offset;

  /* Read DAT to db, but use Hash table to quickly search */
  db = malloc(dat_meta.num_chunks * sizeof(db_item));
  if (!db) {
    sw_trace("META.DAT: no memory for %u records", (unsigned)dat_meta.num_chunks);
    fclose((FD_TYPE)dat_meta.handle);
    dat_meta.handle = NULL;
    dat_meta.num_chunks = 0;
    return 0;
  }
  db_count = (uint32_t)(fread(db, 1, dat_meta.num_chunks * sizeof(db_item), (FD_TYPE)dat_meta.handle) / sizeof(db_item));
  fclose((FD_TYPE)dat_meta.handle);
  dat_meta.handle = NULL;
  if (db_count != dat_meta.num_chunks)
    sw_trace("META.DAT: %u of %u records read", (unsigned)db_count, (unsigned)dat_meta.num_chunks);

  /* every description gets a NUL, whatever the file holds */
  for (uint32_t i = 0; i < db_count; i++)
    db[i].description[sizeof(db[i].description) - 1] = '\0';

  DAT_info(&dat_meta);

  return 0;
}

/* Returns 0 on success and places a pointer in item, otherwise returns 1 and item = NULL */
int db_get_meta(const char *id, struct db_item **item) {
  const char *id_santized = serial_santize_meta(id);
  uint32_t index = db ? DAT_get_index_by_ID(&dat_meta, id_santized) : 0xFFFFFFFF;

  /* SWIRL: the record must lie inside what was read */
  if (index == 0xFFFFFFFF || index < dat_first_index || index - dat_first_index >= db_count) {
    *item = NULL;
    return 1;
  }

  *item = &db[index - dat_first_index];
  return 0;
}

const char *db_format_nplayers_str(int nplayers) {
  static char str[16]; /* SWIRL: "255 Players" did not fit in 10 */
  snprintf(str, sizeof(str), "%d Player%c", nplayers, (nplayers > 1 ? 's' : '\0'));
  return str;
}

const char *db_format_vmu_blocks_str(int num_blocks) {
  static char str[16];
  snprintf(str, sizeof(str), "%d Block%c", num_blocks, (num_blocks != 1 ? 's' : '\0'));
  return str;
}
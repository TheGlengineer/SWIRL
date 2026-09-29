/*
 * File: reader.c
 * Project: dat_builder
 * File Created: Tuesday, 8th June 2021 2:47:35 pm
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#ifdef COSMO
#include "../tools/cosmo/cosmopolitan.h"
#else
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#endif

#include <external/uthash.h>

#include "../gdrom/gdrom_fs.h"
#include "../inc/dat_format.h"

/* SWIRL: a DAT that cannot be used is reported in the start up trace on the Dreamcast, on stdout in the tools */
#if defined(_arch_dreamcast) && !defined(STANDALONE_BINARY)
#include "../ui/swirl/sw_trace.h"
#else
#define sw_trace(...)    \
  do {                   \
    printf(__VA_ARGS__); \
    printf("\n");        \
  } while (0)
#define sw_warn(code, ...) sw_trace(__VA_ARGS__)
#endif

/* SWIRL: what a DAT may claim (the biggest shipped DAT is BOX.DAT, 128 KB chunks, one per game) */
#define DAT_MAX_CHUNKS (16384)
#define DAT_MAX_CHUNK_SIZE (1024 * 1024)

/* Define configure constants */
/* only defined when building the binary tool */
#ifdef STANDALONE_BINARY
#define DEBUG
#endif

#ifdef DEBUG
#define DBG_PRINT(...) printf(__VA_ARGS__)
#else
#define DBG_PRINT(...)
#endif

typedef struct bin_item_raw {
  char ID[12];
  uint32_t offset;
} bin_item_raw;

int DAT_init(dat_file *bin) {
  memset(bin, '\0', sizeof(dat_file));
  return 0;
}

int DAT_load_parse(dat_file *bin, const char *path) {
  FD_TYPE bin_fd;
  bin_header file_header;

#ifdef STANDALONE_BINARY
  bin_fd = fopen(path, "rb");
  const char *filename_safe = path;
#else
  char filename_safe[128];
  memcpy(filename_safe, DISC_PREFIX, strlen(DISC_PREFIX) + 1);
  strcat(filename_safe, path);

  bin_fd = fopen(filename_safe, "rb");
#endif

  if (!bin_fd) {
    printf("DAT:Error Cant read input %s!\n", filename_safe);
    return 1;
  }

  printf("DAT:Open %s (%s)\n", filename_safe, path);

  /* SWIRL: nothing in the header is trusted. The counts must fit the file, the table allocation is checked,
     every ID gets a NUL and an entry whose chunk lies outside the file is dropped. */
  fseek(bin_fd, 0, SEEK_END);
  long file_size = ftell(bin_fd);
  fseek(bin_fd, 0, SEEK_SET);

  if (fread(&file_header, 1, sizeof(bin_header), bin_fd) != sizeof(bin_header) ||
      file_header.magic.rich.version != 1) {
    printf("DAT:Error Incorrect input file format!\n");
    fclose(bin_fd);
    return 1;
  }
  if (file_size < 0 || file_header.chunk_size == 0 || file_header.chunk_size > DAT_MAX_CHUNK_SIZE ||
      file_header.num_chunks > DAT_MAX_CHUNKS ||
      (long)(sizeof(bin_header) + (size_t)file_header.num_chunks * sizeof(bin_item_raw)) > file_size) {
    sw_warn(SW_WARN_DAT_FILE, "%s: %u chunks of %u bytes do not fit a %ld byte file, not used", path, (unsigned)file_header.num_chunks,
             (unsigned)file_header.chunk_size, file_size);
    fclose(bin_fd);
    return 1;
  }

  /* setup basic bin file info */
  bin->chunk_size = file_header.chunk_size;
  bin->num_chunks = 0;
  bin->handle = (void *)bin_fd;
  bin->items = file_header.num_chunks ? malloc(file_header.num_chunks * sizeof(bin_item)) : NULL;
  bin->hash = NULL;
  if (file_header.num_chunks && !bin->items) {
    sw_warn(SW_WARN_DAT_FILE, "%s: no memory for %u entries", path, (unsigned)file_header.num_chunks);
    fclose(bin_fd);
    bin->handle = NULL;
    return 1;
  }

  /* Parse file table to Hash table */
  unsigned int dropped = 0;
  for (unsigned int i = 0; i < file_header.num_chunks; i++) {
    bin_item_raw raw;
    if (fread(&raw, 1, sizeof(bin_item_raw), (FD_TYPE)bin->handle) != sizeof(bin_item_raw))
      break;
    raw.ID[sizeof(raw.ID) - 1] = '\0';
    if ((long)((unsigned long long)raw.offset * bin->chunk_size + bin->chunk_size) > file_size ||
        (unsigned long long)raw.offset * bin->chunk_size > 0x7FFFFFFFULL) {
      dropped++;
      continue;
    }
    bin_item *item = &bin->items[bin->num_chunks];
    memcpy(item->ID, raw.ID, sizeof(item->ID));
    item->offset = raw.offset;
    if (item->ID[0] == '\0') {
      dropped++;
      continue;
    }
    bin_item *dup;
    HASH_FIND_STR(bin->hash, item->ID, dup);
    if (dup) {
      dropped++;
      continue;
    }
    HASH_ADD_STR(bin->hash, ID, item);
    bin->num_chunks++;
  }
  if (dropped)
    sw_warn(SW_WARN_DAT_FILE, "%s: %u entries dropped (chunk outside the file, empty or repeated ID)", path, dropped);

  /* Leave our handle in a handy place in case we need to read after */
  if (bin->num_chunks)
    fseek((FD_TYPE)bin->handle, bin->items[0].offset * bin->chunk_size, SEEK_SET);

  return 0;
}

void DAT_info(const dat_file *bin) {
  DBG_PRINT("DAT:Stats\nChunk Size: %u\nNum Chunks: %u\n\n", bin->chunk_size, bin->num_chunks);
  for (unsigned int i = 0; i < bin->num_chunks; i++) {
    DBG_PRINT("Record[%u] %s at 0x%X\n", bin->items[i].offset, bin->items[i].ID, (unsigned int)(bin->items[i].offset * bin->chunk_size));
  }
  DBG_PRINT("\n");
}

uint32_t DAT_get_offset_by_ID(const dat_file *bin, const char *ID) {
  const bin_item *item;
  uint32_t ret;

  HASH_FIND_STR(bin->hash, ID, item);
  if (item) {
    ret = item->offset * bin->chunk_size;
  } else {
    ret = 0;
  }

  return ret;
}

uint32_t DAT_get_index_by_ID(const dat_file *bin, const char *ID) {
  const bin_item *item;
  uint32_t ret;

  HASH_FIND_STR(bin->hash, ID, item);
  if (item) {
    ret = item->offset;
  } else {
    ret = 0xFFFFFFFF;
  }

  return ret;
}

int DAT_read_file_by_ID(const dat_file *bin, const char *ID, void *buf) {
  uint32_t offset = DAT_get_offset_by_ID(bin, ID);
  if (offset && bin->handle) {
    /* SWIRL: a short read is a failure; the buffer would hold the previous picture */
    fseek((FD_TYPE)bin->handle, offset, SEEK_SET);
    return fread(buf, 1, bin->chunk_size, (FD_TYPE)bin->handle) == bin->chunk_size;
  } else {
    return 0;
  }
}

int DAT_read_file_by_num(const dat_file *bin, uint32_t chunk_num, void *buf) {
  uint32_t offset = chunk_num * bin->chunk_size;
  if (chunk_num < bin->num_chunks && bin->handle) { /* SWIRL: was <=, one past the table */
    fseek((FD_TYPE)bin->handle, offset, SEEK_SET);
    return fread(buf, 1, bin->chunk_size, (FD_TYPE)bin->handle) == bin->chunk_size;
  } else {
    return 0;
  }
}

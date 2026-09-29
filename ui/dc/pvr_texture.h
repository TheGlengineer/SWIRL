/*
 * File: common.h
 * Project: ui
 * File Created: Monday, 3rd June 2019 1:01:31 pm
 * Author: Hayden Kowalchuk (hayden@hkowsoftware.com)
 * -----
 * Copyright (c) 2019 Hayden Kowalchuk
 */

#pragma once

#include <dc/pvr.h>
#include <stddef.h>
#include <stdint.h>

/* Offset and dimensions of each sprite within a spritesheet (romdisk/foo.txt file) */
typedef struct image {
  char name[16];
  uint32_t width, height;
  uint32_t format;
  pvr_ptr_t texture;
} image;

void* pvr_get_internal_buffer(void);
unsigned int pvr_internal_buffer_size(void);
/* SWIRL: a size the PowerVR can draw (powers of two, 8 to 1024) */
int pvr_texture_size_ok(uint32_t w, uint32_t h);
/* SWIRL: the first bytes of a file (32 or more) describe a picture that fits in a file of file_size bytes */
int pvr_header_usable(const void* header, size_t header_size, size_t file_size);
/* Convenience functions. SWIRL: every loader is told how much input there is and how big the destination is;
   a picture that does not fit, or whose header is not usable, returns NULL with width and height 0. */
extern pvr_ptr_t load_pvr(const char* filename, uint32_t* w, uint32_t* h, uint32_t* txrFormat);
extern pvr_ptr_t load_pvr_to_buffer(const char* filename, uint32_t* w, uint32_t* h, uint32_t* txrFormat, void* buffer,
                                    size_t buffer_size);
extern pvr_ptr_t load_pvr_from_buffer(const void* input, size_t input_size, uint32_t* w, uint32_t* h, uint32_t* txrFormat);

/* base method */
extern pvr_ptr_t load_pvr_from_buffer_to_buffer(const void* input, size_t input_size, uint32_t* w, uint32_t* h,
                                                uint32_t* txrFormat, void* buffer, size_t buffer_size);

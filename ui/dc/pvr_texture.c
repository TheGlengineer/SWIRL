/*
 * File: common.c
 * Project: ui
 * File Created: Monday, 3rd June 2019 1:25:54 pm
 * Author: Hayden Kowalchuk (hayden@hkowsoftware.com)
 * -----
 * Copyright (c) 2019 Hayden Kowalchuk
 */
#include "pvr_texture.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "../../gdrom/gdrom_fs.h"

#define PVR_HDR_SIZE 0x20 /* GBIX (16 bytes) then PVRT (16 bytes) */
#define PVRT_SIZE 0x10

#include "../swirl/sw_trace.h"

static unsigned char* _internal_buf = NULL;
static char filename_safe[128];

/* SWIRL: a texture the PowerVR can draw is a power of two from 8 to 1024 on each side. KOS asserts on
   anything else when the polygon header is compiled, which would end on the crash report screen. */
int pvr_texture_size_ok(uint32_t w, uint32_t h) {
  if (w < 8 || w > 1024 || h < 8 || h > 1024)
    return 0;
  return (w & (w - 1)) == 0 && (h & (h - 1)) == 0;
}

/* SWIRL: reads the header (GBIX then PVRT, or PVRT alone) and works out how many bytes of texture data follow
   it. Nothing from the header is trusted: the sizes must be drawable and the data must fit in the input. Returns
   the data size, or 0 with a reason when the picture cannot be used. */
static uint32_t pvr_parse(const void* input, size_t header_size, size_t input_size, uint32_t* w, uint32_t* h,
                          uint32_t* txrFormat, uint32_t* data_offset, const char** why) {
  const unsigned char* texBuf = (const unsigned char*)input;
  size_t pvrt = 0;

  *w = *h = 0;
  *txrFormat = 0;
  *data_offset = 0;
  *why = "";

  if (header_size < PVRT_SIZE || input_size < header_size) {
    *why = "too short";
    return 0;
  }
  if (memcmp(texBuf, "GBIX", 4) == 0) {
    /* the GBIX chunk is normally 8 bytes of payload; allow up to 16 */
    uint32_t gbix_len = texBuf[4] | texBuf[5] << 8 | texBuf[6] << 16 | (uint32_t)texBuf[7] << 24;
    if (gbix_len > 16) {
      *why = "bad GBIX";
      return 0;
    }
    pvrt = 8 + gbix_len;
  }
  if (pvrt + PVRT_SIZE > header_size || memcmp(texBuf + pvrt, "PVRT", 4) != 0) {
    *why = "no PVRT header";
    return 0;
  }
  texBuf += pvrt;

  const uint32_t texW = texBuf[12] | texBuf[13] << 8;
  const uint32_t texH = texBuf[14] | texBuf[15] << 8;
  int texFormat = 0, texColor = 0;
  uint32_t txr_size;

  if (!pvr_texture_size_ok(texW, texH)) {
    *why = "size not drawable";
    return 0;
  }

  switch ((unsigned int)texBuf[8]) {
    case 0x00:
      texColor = PVR_TXRFMT_ARGB1555;
      break;  //(bilevel translucent alpha 0,255)

    case 0x01:
      texColor = PVR_TXRFMT_RGB565;
      break;  //(non translucent RGB565 )

    case 0x02:
      texColor = PVR_TXRFMT_ARGB4444;
      break;  //(translucent alpha 0-255)

    case 0x03:
      texColor = PVR_TXRFMT_YUV422;
      break;  //(non translucent UYVY )

    case 0x04:
      texColor = PVR_TXRFMT_BUMP;
      break;  //(special bump-mapping format)

    case 0x05:
      texColor = PVR_TXRFMT_PAL4BPP;
      break;  //(4-bit palleted texture)

    case 0x06:
      texColor = PVR_TXRFMT_PAL8BPP;
      break;  //(8-bit palleted texture)

    default:
      *why = "unknown pixel format";
      return 0;
  }

  /* bytes of data for this pixel format and layout (16 bit formats are 2 bytes per pixel, YUV422 included) */
  switch ((unsigned int)texBuf[8]) {
    case 0x05:
      txr_size = texW * texH / 2;
      break;
    case 0x06:
      txr_size = texW * texH;
      break;
    default:
      txr_size = texW * texH * 2;
      break;
  }

  switch ((unsigned int)texBuf[9]) {
    case 0x01:
      texFormat = PVR_TXRFMT_TWIDDLED;
      break;  //SQUARE TWIDDLED

    case 0x03:
      texFormat = PVR_TXRFMT_VQ_ENABLE;
      txr_size = 256 * 8 + texW * texH / 4; /* 256 entry codebook of 2x2 blocks, one index byte per block */
      break;  //VQ TWIDDLED

    case 0x09:
      texFormat = PVR_TXRFMT_NONTWIDDLED;
      break;  //RECTANGLE

    case 0x0B:
      texFormat = PVR_TXRFMT_STRIDE | PVR_TXRFMT_NONTWIDDLED;
      break;  //RECTANGULAR STRIDE

    case 0x0D:
      texFormat = PVR_TXRFMT_TWIDDLED;
      break;  //RECTANGULAR TWIDDLED

    case 0x10: {
      /* SMALL VQ: the codebook shrinks with the texture */
      uint32_t entries = texW <= 16 ? 16 : texW <= 32 ? 32 : texW <= 64 ? 128 : 256;
      texFormat = PVR_TXRFMT_VQ_ENABLE | PVR_TXRFMT_NONTWIDDLED;
      txr_size = entries * 8 + texW * texH / 4;
      break;
    }

    default:
      texFormat = PVR_TXRFMT_NONE;
      break;
  }

  if (txr_size == 0 || pvrt + PVRT_SIZE + txr_size > input_size) {
    *why = "data past the end of the file";
    return 0;
  }

  *w = texW;
  *h = texH;
  *txrFormat = texFormat | texColor;
  *data_offset = pvrt + PVRT_SIZE;

  return txr_size;
}

static uint32_t pvr_get_texture_size(const void* input, size_t input_size, uint32_t* w, uint32_t* h,
                                     uint32_t* txrFormat, uint32_t* data_offset, const char** why) {
  return pvr_parse(input, input_size, input_size, w, h, txrFormat, data_offset, why);
}

/* SWIRL: whether the first bytes of a file (at least 32) describe a picture the menu can load out of a file of
   file_size bytes. Used to check theme files before a Classic style is started. */
int pvr_header_usable(const void* header, size_t header_size, size_t file_size) {
  uint32_t w, h, fmt, off;
  const char* why;
  return pvr_parse(header, header_size, file_size, &w, &h, &fmt, &off, &why) != 0;
}

pvr_ptr_t load_pvr_from_buffer_to_buffer(const void* input, size_t input_size, uint32_t* w, uint32_t* h,
                                         uint32_t* txrFormat, void* buffer, size_t buffer_size) {
  const unsigned char* texBuf = (const unsigned char*)input;
  uint32_t data_offset;
  const char* why;
  uint32_t txr_size = pvr_get_texture_size(input, input_size, w, h, txrFormat, &data_offset, &why);

  if (!txr_size) {
    sw_trace("picture: %s, not loaded", why);
    return NULL;
  }
  /* SWIRL: the destination is a fixed slot in video memory (32 KB icon, 128 KB cover, or what is left of the
     scratch); a picture larger than it used to spill into the slots after it */
  if (txr_size > buffer_size) {
    sw_trace("picture: %ux%u (%u bytes) does not fit its %u byte slot, not loaded", (unsigned)*w, (unsigned)*h,
             (unsigned)txr_size, (unsigned)buffer_size);
    *w = *h = 0;
    return NULL;
  }

  pvr_txr_load(texBuf + data_offset, (pvr_ptr_t)buffer, txr_size);

  return buffer;
}

pvr_ptr_t load_pvr_from_buffer(const void* input, size_t input_size, uint32_t* w, uint32_t* h, uint32_t* txrFormat) {
  pvr_ptr_t rv;
  const unsigned char* texBuf = (const unsigned char*)input;
  uint32_t data_offset;
  const char* why;
  uint32_t txr_size = pvr_get_texture_size(input, input_size, w, h, txrFormat, &data_offset, &why);

  if (!txr_size) {
    sw_trace("picture: %s, not loaded", why);
    return NULL;
  }

  if (!(rv = pvr_mem_malloc(txr_size))) {
    printf("PVR: Couldn't allocate memory for texture!\n");
    *w = *h = 0;
    return NULL;
  }
  pvr_txr_load(texBuf + data_offset, rv, txr_size);

  return rv;
}

/* SWIRL: room for the largest picture (512 x 512, 16 bit) plus its header. openMenu's buffer had no room for the
   header, so a full size picture (the theme backgrounds, the header logo: 524 320 bytes) wrote 32 bytes past the
   end, into the memory allocator's own records; a later free() could then crash (the preview 2 launch crash). */
#define PVR_INTERNAL_SIZE (512 * 512 * 2 + PVR_HDR_SIZE)

unsigned int pvr_internal_buffer_size(void) { return PVR_INTERNAL_SIZE; }

void* pvr_get_internal_buffer(void) {
  if (!_internal_buf) {
    _internal_buf = malloc(PVR_INTERNAL_SIZE);
  }
  return _internal_buf;
}

/* SWIRL: reads the whole file into the internal buffer; returns the bytes read, 0 when it could not be used */
static size_t pvr_read_to_internal(const char* filename) {
  long texSize;
  FD_TYPE tex_fd = (FD_TYPE)NULL;
  snprintf(filename_safe, sizeof(filename_safe), "%s%s", DISC_PREFIX, filename);

  /* replace all - with _ */
  char* iter = filename_safe;
  while (*iter++) {
    if (*iter == '-')
      *iter = '_';
  }

  unsigned char* texBuf = pvr_get_internal_buffer();
  if (!texBuf)
    return 0;
  memset(texBuf, '\0', PVR_HDR_SIZE);

  tex_fd = fopen(filename_safe, "rb");
  if (!tex_fd) {
    printf("PVR:Error opening %s!\n", filename_safe);
    return 0;
  }

  fseek(tex_fd, 0, SEEK_END);
  texSize = ftell(tex_fd);

  fseek(tex_fd, 0, SEEK_SET);
  if (texSize <= 0 || texSize > PVR_INTERNAL_SIZE) {
    /* too big for any texture SWIRL uses: never read past the buffer */
    printf("PVR: %s is %ld bytes, larger than %u; not loaded\n", filename_safe, texSize, (unsigned)PVR_INTERNAL_SIZE);
    fclose(tex_fd);
    return 0;
  }
  size_t got = fread(texBuf, 1, (size_t)texSize, tex_fd);
  fclose(tex_fd);
  if (got != (size_t)texSize) {
    printf("PVR: %s short read (%u of %ld)\n", filename_safe, (unsigned)got, texSize);
    return 0;
  }
  return got;
}

pvr_ptr_t load_pvr(const char* filename, uint32_t* w, uint32_t* h, uint32_t* txrFormat) {
  size_t size = pvr_read_to_internal(filename);
  if (!size) {
    *w = *h = 0;
    return NULL;
  }
  return load_pvr_from_buffer(_internal_buf, size, w, h, txrFormat);
}

pvr_ptr_t load_pvr_to_buffer(const char* filename, uint32_t* w, uint32_t* h, uint32_t* txrFormat, void* buffer,
                             size_t buffer_size) {
  size_t size = pvr_read_to_internal(filename);
  if (!size) {
    *w = *h = 0;
    return NULL;
  }
  return load_pvr_from_buffer_to_buffer(_internal_buf, size, w, h, txrFormat, buffer, buffer_size);
}

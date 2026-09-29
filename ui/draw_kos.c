/*
 * File: draw_kos.c
 * Project: ui
 * File Created: Wednesday, 19th May 2021 9:33:03 pm
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#include "draw_kos.h"

#include <stdio.h>

#include "../inc/dat_format.h"
#include "../texture/simple_texture_allocator.h"
#include "dc/pvr_texture.h"
#include "draw_prototypes.h"
#include "font_prototypes.h"
#include "swirl/sw_trace.h"

extern int round(float x);

image img_empty_boxart;

static int current_list;
void draw_set_list(int list) {
  current_list = list;
}
int draw_get_list(void) {
  return current_list;
}

static float z_depth;
float z_get(void) {
  return z_depth;
}
float z_set(float z) {
  z_depth = z;
  return z_depth;
}
float z_set_cond(float z) {
  if (z > z_depth)
    z_depth = z;
  else
    z_inc();
  return z_depth;
}
void z_reset(void) {
  z_depth = 1.0f;
}
float z_inc(void) {
  z_depth += 1.0f;

  /* 512 Puts Blit infront of everything*/
  if (z_depth > 512.0f) {
    z_depth = 512.0f;
  }
  return z_depth;
}

static void* pvr_scratch_buf;
/* Called only once at start */
void draw_init(void) {
  pvr_scratch_buf = pvr_mem_malloc(TEXMAN_BUFFER_SIZE);
  if (!pvr_scratch_buf) {
    /* SWIRL: no room for the scratch; the allocator stays empty and every scratch picture is refused */
    sw_trace("draw_init: no video memory for the %d KB scratch", TEXMAN_BUFFER_SIZE / 1024);
    texman_reset(NULL, 0);
  } else {
    texman_reset(pvr_scratch_buf, TEXMAN_BUFFER_SIZE);
  }

  z_reset();
}

/* called at the start of each frame */
void draw_setup(void) {
}

void* draw_load_missing_icon(void* user) {
  image* img = (image*)user;
  img->texture = img_empty_boxart.texture;
  img->width = img_empty_boxart.width;
  img->height = img_empty_boxart.height;
  img->format = img_empty_boxart.format;
  return img;
}

/* Throws ID into id and returns something if needs to*/
void* draw_load_texture(const char* filename, void* user) {
  image* img = (image*)user;

  pvr_ptr_t txr;

  if (!(txr = load_pvr(filename, &img->width, &img->height, &img->format))) {
    img->texture = img_empty_boxart.texture;
    img->width = img_empty_boxart.width;
    img->height = img_empty_boxart.height;
    img->format = img_empty_boxart.format;
    return img;
  }
  img->texture = txr;

  return user;
}

void* draw_load_texture_buffer(const char* filename, void* user, void* buffer) {
  image* img = (image*)user;

  pvr_ptr_t txr;

  /* SWIRL: the buffer is a place in the scratch (texman); the picture may use what is left after it */
  size_t room = texman_space_after(buffer);
  if (!(txr = load_pvr_to_buffer(filename, &img->width, &img->height, &img->format, buffer, room))) {
    img->texture = img_empty_boxart.texture;
    img->width = img_empty_boxart.width;
    img->height = img_empty_boxart.height;
    img->format = img_empty_boxart.format;
    return img;
  }
  img->texture = txr;

  return user;
}

void* draw_load_texture_from_DAT_to_buffer(const struct dat_file* bin, const char* ID, void* user, void* buffer,
                                           size_t buffer_size) {
  image* img = (image*)user;
  pvr_ptr_t txr;
  /* SWIRL: an entry larger than the buffer would be read past its end */
  int ret = bin->chunk_size <= pvr_internal_buffer_size() ? DAT_read_file_by_ID(bin, ID, pvr_get_internal_buffer()) : 0;
  if (ret) {
    /* SWIRL: the picture must fit its slot in video memory (buffer_size) and its chunk on the card */
    txr = load_pvr_from_buffer_to_buffer(pvr_get_internal_buffer(), bin->chunk_size, &img->width, &img->height,
                                         &img->format, buffer, buffer_size);
    if (txr) {
      img->texture = txr;
      return user;
    }
    sw_trace("picture: %.12s in a DAT not loaded", ID);
  }
  img->texture = img_empty_boxart.texture;
  img->width = img_empty_boxart.width;
  img->height = img_empty_boxart.height;
  img->format = img_empty_boxart.format;
  return img;
}

/* draws an image at coords of a given size */
void draw_draw_image(int x, int y, float width, float height, uint32_t color, void* user) {
  image* img = (image*)user;
  const dimen_RECT uv_01 = {.x = 0, .y = 0, .w = img->width, .h = img->height};
  draw_draw_sub_image(x, y, width, height, color, user, &uv_01);
}

void draw_draw_sub_image(int x, int y, float width, float height, uint32_t color, void* user, const dimen_RECT* rect) {
  image* img = (image*)user;

  /* SWIRL: KOS asserts on a size the PowerVR cannot draw; such a picture is skipped instead */
  if (!img->texture || !pvr_texture_size_ok(img->width, img->height)) {
    return;
  }

  /* Upper left */
  const float x1 = round((float)x);
  const float y1 = round((float)y);
  const float u1 = (float)rect->x / img->width;
  const float v1 = (float)rect->y / img->height;

  /* Lower right */
  const float x2 = round((float)x + width);
  const float y2 = round((float)y + height);
  const float u2 = (float)(rect->x + rect->w) / img->width;
  const float v2 = (float)(rect->y + rect->h) / img->height;

  const float z = z_inc();

#ifdef KOS_SPRITE
  pvr_sprite_cxt_t context;
  pvr_sprite_hdr_t header;

  pvr_sprite_cxt_txr(&context, draw_get_list(), img->format, img->width, img->height, img->texture, PVR_FILTER_BILINEAR);
  pvr_sprite_compile(&header, &context);

  pvr_prim(&header, sizeof(header));

  pvr_sprite_txr_t vert = {
      .flags = PVR_CMD_VERTEX_EOL, /* Always? */
      /*  upper left */
      .ax = x1,
      .ay = y1,
      .az = z,
      /* upper right */
      .bx = x2,
      .by = y1,
      .bz = z,
      /* lower left */
      .cx = x2,
      .cy = y2,
      .cz = z,
      /* interpolated */
      .dx = x1,
      .dy = y2,
      .auv = PVR_PACK_16BIT_UV(u1, v1), /* UVS */
      .buv = PVR_PACK_16BIT_UV(u2, v1), /* UVS */
      .cuv = PVR_PACK_16BIT_UV(u2, v2), /* UVS */
  };
  pvr_prim(&vert, sizeof(vert));

#else
  pvr_poly_cxt_t context;
  pvr_poly_hdr_t header;

  pvr_poly_cxt_txr(&context, draw_get_list(), img->format, img->width, img->height, img->texture, PVR_FILTER_BILINEAR);
  pvr_poly_compile(&header, &context);

  pvr_prim(&header, sizeof(header));

  pvr_vertex_t vert = {
      .argb = color,
      .oargb = 0,
      .flags = PVR_CMD_VERTEX,
      .z = 1};

  vert.x = x1;
  vert.y = y2;
  vert.z = z;
  vert.u = u1;
  vert.v = v2;
  pvr_prim(&vert, sizeof(vert));

  vert.x = x1;
  vert.y = y1;
  vert.u = u1;
  vert.v = v1;
  pvr_prim(&vert, sizeof(vert));

  vert.x = x2;
  vert.y = y2;
  vert.u = u2;
  vert.v = v2;
  pvr_prim(&vert, sizeof(vert));

  vert.flags = PVR_CMD_VERTEX_EOL;
  vert.x = x2;
  vert.y = y1;
  vert.u = u2;
  vert.v = v1;
  pvr_prim(&vert, sizeof(vert));
#endif
}

/* Draws untextured quad at coords with size and color(rgba) */
void draw_draw_quad(int x, int y, float width, float height, uint32_t color) {
  /* Upper left */
  const float x1 = round((float)x);
  const float y1 = round((float)y);

  /* Lower right */
  const float x2 = round((float)x + width);
  const float y2 = round((float)y + height);

  const float z = z_inc();

#ifdef KOS_SPRITE
  pvr_sprite_cxt_t context;
  pvr_sprite_hdr_t header;

  pvr_sprite_cxt_col(&context, draw_get_list());
  pvr_sprite_compile(&header, &context);

  header.argb = color;

  pvr_prim(&header, sizeof(header));

  pvr_sprite_col_t vert = {
      .flags = PVR_CMD_VERTEX_EOL, /* Always? */
      /*  upper left */
      .ax = x1,
      .ay = y1,
      .az = z,
      /* upper right */
      .bx = x2,
      .by = y1,
      .bz = z,
      /* lower left */
      .cx = x2,
      .cy = y2,
      .cz = z,
      /* interpolated */
      .dx = x1,
      .dy = y2};

  pvr_prim(&vert, sizeof(vert));

#else
  pvr_poly_cxt_t context;
  pvr_poly_hdr_t header;

  pvr_poly_cxt_col(&context, draw_get_list());
  pvr_poly_compile(&header, &context);

  pvr_prim(&header, sizeof(header));

  pvr_vertex_t vert = {
      .argb = color,
      .oargb = 0,
      .flags = PVR_CMD_VERTEX,
      .z = z,
      .u = 0,
      .v = 0};

  vert.x = x1;
  vert.y = y2;
  pvr_prim(&vert, sizeof(vert));

  vert.x = x1;
  vert.y = y1;
  pvr_prim(&vert, sizeof(vert));

  vert.x = x2;
  vert.y = y2;
  pvr_prim(&vert, sizeof(vert));

  vert.flags = PVR_CMD_VERTEX_EOL;
  vert.x = x2;
  vert.y = y1;
  pvr_prim(&vert, sizeof(vert));
#endif
}

/* draws an image at coords as a square */
void draw_draw_square(int x, int y, float size, uint32_t color, void* user) {
  draw_draw_image(x, y, size, size, color, user);
}

void draw_draw_image_centered(int x, int y, float width, float height, uint32_t color, void* user) {
  const int x_extent = width / 2;
  const int y_extent = height / 2;
  draw_draw_image(x - x_extent, y - y_extent, width, height, color, user);
}
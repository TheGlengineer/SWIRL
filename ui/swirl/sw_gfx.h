/*
 * SWIRL: drawing primitives and text for the dashboard UI.
 * All drawing goes to the list currently selected by draw_set_list().
 */
#pragma once

#include <stdint.h>

#include "../dc/pvr_texture.h"

enum sw_font_id { SWF_TITLE = 0, SWF_HEAD, SWF_UI, SWF_BODY, SWF_SMALL, SWF_COUNT };

#define SW_ARGB(a, r, g, b) ((((uint32_t)(a)) << 24) | (((uint32_t)(r)) << 16) | (((uint32_t)(g)) << 8) | ((uint32_t)(b)))
#define SW_RGB(hex) (0xFF000000u | (uint32_t)(hex))
#define SW_ALPHA(c, a) (((c) & 0x00FFFFFFu) | (((uint32_t)(a)) << 24))

int sw_gfx_init(void); /* once: fonts + generated textures */
void sw_gfx_frame(void); /* every frame before drawing */

/* shapes */
void sw_rect(float x, float y, float w, float h, uint32_t c);
void sw_rect4(float x, float y, float w, float h, uint32_t tl, uint32_t tr, uint32_t bl, uint32_t br);
void sw_grad_v(float x, float y, float w, float h, uint32_t top, uint32_t bottom);
void sw_grad_h(float x, float y, float w, float h, uint32_t left, uint32_t right);
void sw_rrect(float x, float y, float w, float h, float r, uint32_t c);
void sw_rrect_outline(float x, float y, float w, float h, float r, float t, uint32_t c);
void sw_shadow(float x, float y, float w, float h, float spread, uint32_t c);
void sw_circle(float cx, float cy, float rad, uint32_t c);
void sw_glow(float cx, float cy, float rad, uint32_t c);

/* textures */
void sw_image(const image *img, float x, float y, float w, float h, uint32_t c);
void sw_image_uv(const image *img, float x, float y, float w, float h, float u0, float v0, float u1, float v1,
                 uint32_t tl, uint32_t tr, uint32_t bl, uint32_t br);
void sw_image_rounded(const image *img, float x, float y, float w, float h, float r, uint32_t c);

/* text: y is the top of the line box */
float sw_text(int font, float x, float y, float size, uint32_t c, const char *s);
float sw_text_n(int font, float x, float y, float size, uint32_t c, const char *s, int n);
float sw_text_width(int font, float size, const char *s);
float sw_text_width_n(int font, float size, const char *s, int n);
float sw_text_clip(int font, float x, float y, float size, uint32_t c, const char *s, float max_w);
void sw_text_center(int font, float cx, float y, float size, uint32_t c, const char *s);
void sw_text_right(int font, float rx, float y, float size, uint32_t c, const char *s);
int sw_text_wrap(int font, float x, float y, float size, uint32_t c, const char *s, float max_w, float line_h, int max_lines);
float sw_font_line(int font, float size);

/* global alpha multiplier for everything drawn after (1 = opaque) */
void sw_set_fade(float a);
float sw_get_fade(void);

/* low level */
float sw_znext(void);

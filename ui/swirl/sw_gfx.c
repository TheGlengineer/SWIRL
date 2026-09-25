/*
 * SWIRL: drawing primitives and text for the dashboard UI.
 */
#include "sw_gfx.h"

#include <dc/pvr.h>
#include <math.h>
#include <malloc.h>
#include <stdlib.h>
#include <stdio.h>
#include <string.h>

#include "../draw_prototypes.h"

/* ---------- fonts (baked by swirl/tools/gen_fonts.py, linked by assets.S) ---------- */
extern const uint8_t swirl_font_title[], swirl_font_head[], swirl_font_ui[], swirl_font_body[], swirl_font_small[];

typedef struct __attribute__((packed)) swf_header {
  char magic[4];
  uint16_t tex_w, tex_h, size;
  int16_t ascent, descent, line_h;
  uint16_t first, count;
} swf_header;

typedef struct __attribute__((packed)) swf_glyph {
  uint16_t x, y;
  uint8_t w, h;
  int8_t xoff, yoff;
  uint8_t adv, pad;
} swf_glyph;

typedef struct sw_font {
  const swf_header *hdr;
  const swf_glyph *glyphs;
  pvr_ptr_t tex;
  float inv_w, inv_h;
} sw_font;

static sw_font fonts[SWF_COUNT];

/* generated textures */
static pvr_ptr_t tex_circle, tex_soft;
#define GEN_TEX 64

static float zcur;
static float fade = 1.0f;

void sw_set_fade(float a) {
  fade = a < 0.f ? 0.f : (a > 1.f ? 1.f : a);
}

float sw_get_fade(void) {
  return fade;
}

static inline uint32_t fadec(uint32_t c) {
  if (fade >= 0.999f)
    return c;
  uint32_t a = (uint32_t)(((c >> 24) & 0xFF) * fade);
  return (c & 0x00FFFFFFu) | (a << 24);
}

float sw_znext(void) {
  zcur += 1.0f;
  return zcur;
}

void sw_gfx_frame(void) {
  zcur = 1.0f;
}

/* Fonts and glow shapes are 8 bit textures drawn through palette bank 0, which holds white at 256 alpha
 * levels in full 32 bit colour. That gives soft edges 16 times finer than ARGB4444 and half the memory. */
#define TXR_ALPHA8 (PVR_TXRFMT_PAL8BPP | PVR_TXRFMT_8BPP_PAL(0) | PVR_TXRFMT_TWIDDLED)

static pvr_ptr_t load_alpha8(const uint8_t *src, int w, int h) {
  size_t bytes = (size_t)w * h;
  pvr_ptr_t tex = pvr_mem_malloc(bytes);
  if (!tex)
    return NULL;
  void *tmp = memalign(32, bytes);
  if (!tmp) {
    pvr_mem_free(tex);
    return NULL;
  }
  memcpy(tmp, src, bytes);
  pvr_txr_load_ex(tmp, tex, w, h, PVR_TXRLOAD_8BPP); /* twiddles on the way in */
  free(tmp);
  return tex;
}

static int load_font(int id, const uint8_t *blob) {
  const swf_header *h = (const swf_header *)blob;
  if (memcmp(h->magic, "SWF2", 4) != 0)
    return -1;
  fonts[id].hdr = h;
  fonts[id].glyphs = (const swf_glyph *)(blob + sizeof(swf_header));
  const uint8_t *pix = blob + sizeof(swf_header) + sizeof(swf_glyph) * h->count;
  fonts[id].tex = load_alpha8(pix, h->tex_w, h->tex_h);
  if (!fonts[id].tex)
    return -1;
  fonts[id].inv_w = 1.0f / h->tex_w;
  fonts[id].inv_h = 1.0f / h->tex_h;
  return 0;
}

static pvr_ptr_t make_round_tex(int soft) {
  uint8_t *buf = malloc(GEN_TEX * GEN_TEX);
  if (!buf)
    return NULL;
  const float c = (GEN_TEX - 1) / 2.0f;
  const float r = GEN_TEX / 2.0f;
  for (int y = 0; y < GEN_TEX; y++) {
    for (int x = 0; x < GEN_TEX; x++) {
      float dx = x - c, dy = y - c;
      float d = sqrtf(dx * dx + dy * dy);
      float a;
      if (soft) {
        float t = 1.0f - (d / r);
        if (t < 0.f) t = 0.f;
        a = t * t * (3.f - 2.f * t); /* smoothstep falloff */
      } else {
        a = r - d + 0.5f; /* 1px anti aliased edge */
        if (a < 0.f) a = 0.f;
        if (a > 1.f) a = 1.f;
      }
      buf[y * GEN_TEX + x] = (uint8_t)(a * 255.f + 0.5f);
    }
  }
  pvr_ptr_t tex = load_alpha8(buf, GEN_TEX, GEN_TEX);
  free(buf);
  return tex;
}

static void setup_palette(void) {
  pvr_set_pal_format(PVR_PAL_ARGB8888);
  for (int i = 0; i < 256; i++)
    pvr_set_pal_entry(i, ((uint32_t)i << 24) | 0x00FFFFFFu);
}

int sw_gfx_init(void) {
  static int done = 0;
  if (done)
    return 0;
  int ret = 0;
  setup_palette();
  ret |= load_font(SWF_TITLE, swirl_font_title);
  ret |= load_font(SWF_HEAD, swirl_font_head);
  ret |= load_font(SWF_UI, swirl_font_ui);
  ret |= load_font(SWF_BODY, swirl_font_body);
  ret |= load_font(SWF_SMALL, swirl_font_small);
  tex_circle = make_round_tex(0);
  tex_soft = make_round_tex(1);
  done = 1;
  return ret;
}

/* ---------- vertex helpers ---------- */
static inline void hdr_col(void) {
  pvr_poly_cxt_t cxt;
  pvr_poly_hdr_t hdr;
  pvr_poly_cxt_col(&cxt, draw_get_list());
  pvr_poly_compile(&hdr, &cxt);
  pvr_prim(&hdr, sizeof(hdr));
}

static inline void hdr_txr(int fmt, int w, int h, pvr_ptr_t tex) {
  pvr_poly_cxt_t cxt;
  pvr_poly_hdr_t hdr;
  pvr_poly_cxt_txr(&cxt, draw_get_list(), fmt, w, h, tex, PVR_FILTER_BILINEAR);
  pvr_poly_compile(&hdr, &cxt);
  pvr_prim(&hdr, sizeof(hdr));
}

static inline void quad(float x1, float y1, float x2, float y2, float u0, float v0, float u1, float v1,
                        uint32_t tl, uint32_t tr, uint32_t bl, uint32_t br, float z) {
  pvr_vertex_t v;
  v.oargb = 0;
  v.z = z;
  v.flags = PVR_CMD_VERTEX;
  tl = fadec(tl); tr = fadec(tr); bl = fadec(bl); br = fadec(br);
  v.x = x1; v.y = y2; v.u = u0; v.v = v1; v.argb = bl;
  pvr_prim(&v, sizeof(v));
  v.x = x1; v.y = y1; v.u = u0; v.v = v0; v.argb = tl;
  pvr_prim(&v, sizeof(v));
  v.x = x2; v.y = y2; v.u = u1; v.v = v1; v.argb = br;
  pvr_prim(&v, sizeof(v));
  v.flags = PVR_CMD_VERTEX_EOL;
  v.x = x2; v.y = y1; v.u = u1; v.v = v0; v.argb = tr;
  pvr_prim(&v, sizeof(v));
}

/* ---------- shapes ---------- */
void sw_rect4(float x, float y, float w, float h, uint32_t tl, uint32_t tr, uint32_t bl, uint32_t br) {
  if (w <= 0 || h <= 0)
    return;
  hdr_col();
  quad(x, y, x + w, y + h, 0, 0, 0, 0, tl, tr, bl, br, sw_znext());
}

void sw_rect(float x, float y, float w, float h, uint32_t c) {
  sw_rect4(x, y, w, h, c, c, c, c);
}

void sw_grad_v(float x, float y, float w, float h, uint32_t top, uint32_t bottom) {
  sw_rect4(x, y, w, h, top, top, bottom, bottom);
}

void sw_grad_h(float x, float y, float w, float h, uint32_t left, uint32_t right) {
  sw_rect4(x, y, w, h, left, right, left, right);
}

/* 9 slice using a round texture: corners use the quarter circles, edges and centre use the middle line */
static void nine_slice(pvr_ptr_t tex, float x, float y, float w, float h, float r, uint32_t c) {
  if (w <= 0 || h <= 0)
    return;
  if (r * 2 > w) r = w / 2;
  if (r * 2 > h) r = h / 2;
  const float z = sw_znext();
  const float xs[4] = {x, x + r, x + w - r, x + w};
  const float ys[4] = {y, y + r, y + h - r, y + h};
  const float us[4] = {0.f, 0.5f, 0.5f, 1.f};
  hdr_txr(TXR_ALPHA8, GEN_TEX, GEN_TEX, tex);
  for (int j = 0; j < 3; j++) {
    for (int i = 0; i < 3; i++) {
      if (xs[i + 1] - xs[i] <= 0.f || ys[j + 1] - ys[j] <= 0.f)
        continue;
      quad(xs[i], ys[j], xs[i + 1], ys[j + 1], us[i], us[j], us[i + 1], us[j + 1], c, c, c, c, z);
    }
  }
}

void sw_rrect(float x, float y, float w, float h, float r, uint32_t c) {
  if (r < 1.f) {
    sw_rect(x, y, w, h, c);
    return;
  }
  nine_slice(tex_circle, x, y, w, h, r, c);
}

void sw_rrect_outline(float x, float y, float w, float h, float r, float t, uint32_t c) {
  /* thin ring: four straight edges plus corner arcs approximated by circle corners minus inner fill is
     not possible without stencil, so draw straight edges and small corner caps */
  sw_rect(x + r, y, w - 2 * r, t, c);
  sw_rect(x + r, y + h - t, w - 2 * r, t, c);
  sw_rect(x, y + r, t, h - 2 * r, c);
  sw_rect(x + w - t, y + r, t, h - 2 * r, c);
  /* corners: arcs drawn as short diagonal-ish segments using small rounded dots */
  const int steps = 6;
  for (int i = 0; i <= steps; i++) {
    float a = (float)i / steps * 1.5707963f;
    float ca = cosf(a), sa = sinf(a);
    float rr = r - t / 2.f;
    sw_circle(x + r - rr * ca, y + r - rr * sa, t / 2.f + 0.3f, c);
    sw_circle(x + w - r + rr * ca, y + r - rr * sa, t / 2.f + 0.3f, c);
    sw_circle(x + r - rr * ca, y + h - r + rr * sa, t / 2.f + 0.3f, c);
    sw_circle(x + w - r + rr * ca, y + h - r + rr * sa, t / 2.f + 0.3f, c);
  }
}

void sw_shadow(float x, float y, float w, float h, float spread, uint32_t c) {
  nine_slice(tex_soft, x - spread, y - spread, w + spread * 2, h + spread * 2, spread * 2, c);
}

void sw_circle(float cx, float cy, float rad, uint32_t c) {
  nine_slice(tex_circle, cx - rad, cy - rad, rad * 2, rad * 2, rad, c);
}

void sw_glow(float cx, float cy, float rad, uint32_t c) {
  nine_slice(tex_soft, cx - rad, cy - rad, rad * 2, rad * 2, rad, c);
}

/* ---------- textures ---------- */
void sw_image_uv(const image *img, float x, float y, float w, float h, float u0, float v0, float u1, float v1,
                 uint32_t tl, uint32_t tr, uint32_t bl, uint32_t br) {
  if (!img || !img->texture || img->width == 0 || img->height == 0 || w <= 0 || h <= 0)
    return;
  hdr_txr(img->format, img->width, img->height, img->texture);
  quad(x, y, x + w, y + h, u0, v0, u1, v1, tl, tr, bl, br, sw_znext());
}

void sw_image(const image *img, float x, float y, float w, float h, uint32_t c) {
  sw_image_uv(img, x, y, w, h, 0.f, 0.f, 1.f, 1.f, c, c, c, c);
}

/* Image with rounded corners: draw the image inset, then cover the four corners with the background is not
   possible generically, so we clip by drawing the image as 9 slices where the corner slices are replaced by
   quarter circles filled with the image's corner texel colour. For covers a 4-6px radius reads as rounded. */
void sw_image_rounded(const image *img, float x, float y, float w, float h, float r, uint32_t c) {
  if (r < 1.f) {
    sw_image(img, x, y, w, h, c);
    return;
  }
  /* centre cross */
  float ur = r / w, vr = r / h;
  sw_image_uv(img, x + r, y, w - 2 * r, h, ur, 0.f, 1.f - ur, 1.f, c, c, c, c);
  sw_image_uv(img, x, y + r, r, h - 2 * r, 0.f, vr, ur, 1.f - vr, c, c, c, c);
  sw_image_uv(img, x + w - r, y + r, r, h - 2 * r, 1.f - ur, vr, 1.f, 1.f - vr, c, c, c, c);
  /* corners: modulated by the circle alpha is not available on a 565 cover, so shrink the corner square
     diagonally with two triangles approximating the arc */
  const float k = 0.2929f * r; /* r * (1 - 1/sqrt2) */
  const float corners[4][2] = {{x, y}, {x + w - r, y}, {x, y + h - r}, {x + w - r, y + h - r}};
  for (int i = 0; i < 4; i++) {
    float cx = corners[i][0], cy = corners[i][1];
    float u0 = (cx - x) / w, v0 = (cy - y) / h, u1 = u0 + ur, v1 = v0 + vr;
    /* draw the corner square minus its outer notch as a quad whose outer vertex is pulled inward */
    pvr_vertex_t v;
    hdr_txr(img->format, img->width, img->height, img->texture);
    float z = sw_znext();
    v.oargb = 0;
    v.z = z;
    v.argb = fadec(c);
    float px[4] = {cx, cx + r, cx, cx + r};
    float py[4] = {cy, cy, cy + r, cy + r};
    /* pull the outermost vertex toward the centre of the corner square */
    int outer = i; /* 0 TL, 1 TR, 2 BL, 3 BR */
    float ox = (outer == 0 || outer == 2) ? k : -k;
    float oy = (outer == 0 || outer == 1) ? k : -k;
    px[outer] += ox;
    py[outer] += oy;
    float pu[4] = {u0, u1, u0, u1};
    float pv[4] = {v0, v0, v1, v1};
    pu[outer] += ox / w;
    pv[outer] += oy / h;
    int order[4] = {2, 0, 3, 1}; /* BL TL BR TR */
    for (int n = 0; n < 4; n++) {
      int k2 = order[n];
      v.flags = (n == 3) ? PVR_CMD_VERTEX_EOL : PVR_CMD_VERTEX;
      v.x = px[k2]; v.y = py[k2]; v.u = pu[k2]; v.v = pv[k2];
      pvr_prim(&v, sizeof(v));
    }
  }
}

/* ---------- text ---------- */
static inline const swf_glyph *glyph(const sw_font *f, unsigned char ch) {
  if (ch < f->hdr->first || ch >= f->hdr->first + f->hdr->count)
    ch = '?';
  return &f->glyphs[ch - f->hdr->first];
}

float sw_font_line(int font, float size) {
  const sw_font *f = &fonts[font];
  return f->hdr ? f->hdr->line_h * (size / f->hdr->size) : size;
}

float sw_text_width_n(int font, float size, const char *s, int n) {
  const sw_font *f = &fonts[font];
  if (!f->hdr || !s)
    return 0.f;
  float sc = size / f->hdr->size, w = 0.f;
  for (int i = 0; s[i] && (n < 0 || i < n); i++)
    w += glyph(f, (unsigned char)s[i])->adv * sc;
  return w;
}

float sw_text_width(int font, float size, const char *s) {
  return sw_text_width_n(font, size, s, -1);
}

float sw_text_n(int font, float x, float y, float size, uint32_t c, const char *s, int n) {
  const sw_font *f = &fonts[font];
  if (!f->hdr || !s || !s[0])
    return 0.f;
  const float sc = size / f->hdr->size;
  const int exact = (sc > 0.99f && sc < 1.01f);
  float base = y + f->hdr->ascent * sc;
  if (exact) {
    x = (float)(int)(x + 0.5f);
    base = (float)(int)(base + 0.5f);
  }
  const float x0 = x;
  hdr_txr(TXR_ALPHA8, f->hdr->tex_w, f->hdr->tex_h, f->tex);
  const float z = sw_znext();
  for (int i = 0; s[i] && (n < 0 || i < n); i++) {
    const swf_glyph *g = glyph(f, (unsigned char)s[i]);
    if (g->w && g->h && s[i] != ' ') {
      float gx = x + g->xoff * sc, gy = base + g->yoff * sc;
      quad(gx, gy, gx + g->w * sc, gy + g->h * sc,
           g->x * f->inv_w, g->y * f->inv_h, (g->x + g->w) * f->inv_w, (g->y + g->h) * f->inv_h, c, c, c, c, z);
    }
    x += g->adv * sc;
  }
  return x - x0;
}

float sw_text(int font, float x, float y, float size, uint32_t c, const char *s) {
  return sw_text_n(font, x, y, size, c, s, -1);
}

float sw_text_clip(int font, float x, float y, float size, uint32_t c, const char *s, float max_w) {
  if (!s)
    return 0.f;
  float w = sw_text_width(font, size, s);
  if (w <= max_w)
    return sw_text(font, x, y, size, c, s);
  float dots = sw_text_width(font, size, "...");
  int n = strlen(s);
  while (n > 0 && sw_text_width_n(font, size, s, n) + dots > max_w)
    n--;
  while (n > 0 && s[n - 1] == ' ')
    n--;
  float used = sw_text_n(font, x, y, size, c, s, n);
  return used + sw_text(font, x + used, y, size, c, "...");
}

void sw_text_center(int font, float cx, float y, float size, uint32_t c, const char *s) {
  sw_text(font, cx - sw_text_width(font, size, s) / 2.f, y, size, c, s);
}

void sw_text_right(int font, float rx, float y, float size, uint32_t c, const char *s) {
  sw_text(font, rx - sw_text_width(font, size, s), y, size, c, s);
}

int sw_text_wrap(int font, float x, float y, float size, uint32_t c, const char *s, float max_w, float line_h, int max_lines) {
  if (!s)
    return 0;
  int lines = 0;
  const char *p = s;
  while (*p && lines < max_lines) {
    while (*p == ' ')
      p++;
    /* find the longest run of whole words that fits */
    int best = 0, i = 0;
    while (p[i] && p[i] != '\n') {
      int j = i;
      while (p[j] && p[j] != ' ' && p[j] != '\n')
        j++;
      if (sw_text_width_n(font, size, p, j) > max_w && best > 0)
        break;
      best = j;
      if (sw_text_width_n(font, size, p, j) > max_w)
        break; /* single long word */
      i = j;
      while (p[i] == ' ')
        i++;
      if (!p[i] || p[i] == '\n')
        break;
    }
    if (best == 0)
      best = (int)strlen(p);
    int last = (lines == max_lines - 1) && p[best] && p[best] != '\n';
    char buf[256];
    int n = best < 250 ? best : 250;
    memcpy(buf, p, n);
    buf[n] = 0;
    if (last) {
      int rest = strlen(p);
      int m = rest < 250 ? rest : 250;
      memcpy(buf, p, m);
      buf[m] = 0;
      sw_text_clip(font, x, y + lines * line_h, size, c, buf, max_w);
    } else {
      sw_text(font, x, y + lines * line_h, size, c, buf);
    }
    lines++;
    p += best;
    if (*p == '\n')
      p++;
  }
  return lines;
}

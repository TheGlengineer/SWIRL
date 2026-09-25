/*
 * SWIRL dashboard UI for openMenu.
 *
 * Tabs: Home (hero + carousel), Library (grid), Collections (sidebar + grid), System (settings).
 * Overlay: game detail with disc picker. Favorites and play history persist to the VMU (SWIRL.DAT).
 * Reads the same OPENMENU.INI / BOX.DAT / ICON.DAT / META.DAT as stock openMenu.
 */
#define _DEFAULT_SOURCE
#include "ui_swirl.h"

#include <arch/timer.h>
#include <arch/arch.h>
#include <arch/rtc.h>
#include <ctype.h>
#include <dc/maple.h>
#include <dc/maple/controller.h>
#include <dc/maple/vmu.h>
#include <dc/vmufs.h>
#include <malloc.h>
#include <math.h>
#include <dc/maple/keyboard.h>
#include <dc/maple/purupuru.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "../backend/db_item.h"
#include "../backend/db_list.h"
#include "../backend/gd_item.h"
#include "../backend/gd_list.h"
#include "../backend/gdemu_control.h"
#include "../inc/dat_format.h"
#include "../texture/txr_manager.h"
#include "common.h"
#include "draw_prototypes.h"
#include "global_settings.h"
#include "swirl/sw_audio.h"
#include "swirl/sw_gfx.h"
#include "swirl/sw_lib.h"
#include "swirl/sw_vmu.h"
#include "swirl/sw_version.h"

extern image img_empty_boxart;
extern void reload_ui(void);

/* ---------- palette ---------- */
#define C_DEEP 0xFF05070D
#define C_TEXT 0xFFEEF1F7
#define C_WHITE 0xFFFFFFFF
#define C_DIM SW_ALPHA(C_TEXT, 0xB8)
#define C_FAINT SW_ALPHA(C_TEXT, 0x80)
/* accent colour: chosen in System, or set by the seasonal backdrop */
static uint32_t accent_col = 0xFFF28C28;
#define C_ORANGE accent_col
#define C_ORANGE_SOFT SW_ALPHA(accent_col, 0x40)
#define C_CHIP 0x24EEF1F7
#define C_PANEL 0xD00C1222
#define C_BTN_A 0xFFC8322F
#define C_BTN_B 0xFF2F63C8
#define C_BTN_X 0xFFC9A01C
#define C_BTN_Y 0xFF2C8A47

enum tab { TAB_HOME = 0, TAB_LIBRARY, TAB_COLLECTIONS, TAB_SYSTEM, TAB_COUNT };
static const char *tab_names[TAB_COUNT] = {"Home", "Library", "Collections", "System"};

enum mode { MODE_TABS = 0, MODE_DETAIL, MODE_LAUNCH, MODE_PADTEST, MODE_OPTIONS, MODE_VMU, MODE_RESUME };

#define MAX_LIST 1024
#define MAX_COLS 40

/* ---------- state ---------- */
static int tab = TAB_HOME;
static int mode = MODE_TABS;
static float tab_anim = 1.f;

/* home */
static int home_list[MAX_LIST];
static int home_len;
static int home_recent_len;
static int home_sel;
static float home_scroll; /* animated index of the leftmost carousel slot */

/* library */
static int lib_list[MAX_LIST];
static int lib_len;
static int lib_sel;
static int lib_top_row;
static int lib_sort = SW_SORT_NAME;
#define LIB_COLS 6
#define LIB_ROWS 3

/* collections */
static sw_collection cols[MAX_COLS];
static int num_cols;
static int col_sel;
static int col_top;
static int col_focus_grid;
static int col_list[MAX_LIST];
static int col_len;
static int col_game_sel;
static int col_top_row;
#define COL_GRID_COLS 5
#define COL_GRID_ROWS 2
#define COL_SIDEBAR_ROWS 13

/* system */
static int sys_sel;
static int sys_style;

/* detail */
static int detail_game = -1;
static int detail_disc;
static const gd_item *detail_discs[9];
static int detail_num_discs;
static int detail_return_mode;

/* launch */
static const gd_item *launch_item;
static int launch_frames;
static int launch_from_detail;

/* input */
static unsigned int prev_btn;
static int hold_frames;
static int idle_frames;
static int saver_on; /* screen saver showing (see the screen saver section) */
static int saver_input(unsigned int btn, int pressed);
static void saver_start(void);
enum { SAVER_DRIFT = 0, SAVER_SHOWCASE, SAVER_SWIRL, SAVER_BOUNCE, SAVER_DIM, SAVER_COUNT };
static const char *saver_names[] = {"Cover drift", "Game showcase", "Swirl", "Bouncing logo", "Dim the screen"};

/* focus tracking for large art + ambient colour */
static int focus_game = -1;
static int focus_frames;
static uint32_t amb_cur = 0xFF101828, amb_prev = 0xFF101828;
static float amb_t = 1.f;

/* toast */
static char toast[48];
static int toast_frames;

/* save debounce */
static int save_countdown;

/* optional per game VMU screens made by SWIRL Card Manager */
static dat_file vmu_dat;
static int have_vmu_dat;

/* forward declarations for the new screens */
static void open_vmu_manager(void);
static void load_shots(const struct sw_game *g);
static int launch_is_custom(const struct sw_game *g);
static int dir_pressed(unsigned int btn);

/* surprise me */
static int surprise_frames, surprise_target;

/* quick resume */
static int resume_game = -1, resume_frames, resume_checked;

/* launch options sheet (per game settings, CodeBreaker) */
enum { OPT_PLAY = 0, OPT_CB, OPT_REGION, OPT_VIDEO, OPT_BOOT, OPT_RESET };
static int opt_items[8], opt_count, opt_sel;
static int have_cb = -1, have_bleem = -1;

/* screenshots (SHOT.DAT from SWIRL Card Manager): two 256x256 RGB565 PVRs per game, 4:3 picture on top */
#define SHOT_PVR_BYTES (32 + 256 * 256 * 2)
#define SHOT_CHUNK (2 * SHOT_PVR_BYTES)
static dat_file shot_dat;
static int have_shot_dat;
static image shot_img[2];
static int shot_count, shot_game = -1;
static uint8_t *shot_buf;

/* assets */
static image img_logo_src;
static int have_logo;

/* ---------- helpers ---------- */
static uint32_t mix(uint32_t a, uint32_t b, float t) {
  if (t <= 0.f) return a;
  if (t >= 1.f) return b;
  uint32_t r = 0;
  for (int s = 0; s < 32; s += 8) {
    float ca = (a >> s) & 0xFF, cb = (b >> s) & 0xFF;
    r |= ((uint32_t)(ca + (cb - ca) * t) & 0xFF) << s;
  }
  return r;
}

static float approach(float cur, float target, float rate) {
  float d = target - cur;
  if (d > -0.01f && d < 0.01f) return target;
  return cur + d * rate;
}

/* ---------- themes ---------- */
static const struct {
  const char *name;
  uint32_t col;
} accents[] = {{"Orange", 0xFFF28C28}, {"Blue", 0xFF3A8FF0}, {"Green", 0xFF3FB96B}, {"Pink", 0xFFEA3C8F},
               {"Purple", 0xFF9B6CF0}, {"Red", 0xFFE0584A}, {"Gold", 0xFFE8B822}, {"Teal", 0xFF2EC4B6}};
#define NUM_ACCENTS ((int)(sizeof(accents) / sizeof(accents[0])))
enum { BACKDROP_COVER = 0, BACKDROP_NIGHT, BACKDROP_SEASONAL, BACKDROP_COUNT };
static const char *backdrop_names[BACKDROP_COUNT] = {"Cover colour", "Night", "Seasonal"};
enum { PART_NONE = 0, PART_SNOW, PART_LEAVES, PART_PETALS, PART_SPARKLE };
typedef struct season {
  const char *name;
  uint32_t accent, glow;
  int particles;
} season;
static const season seasons[12] = {
    {"Winter", 0xFF7FC8F8, 0xFF1C3F7A, PART_SNOW},      {"Valentine", 0xFFEA3C8F, 0xFF5A1638, PART_PETALS},
    {"Spring", 0xFF6CCB5F, 0xFF1F5A3A, PART_PETALS},    {"Spring", 0xFF6CCB5F, 0xFF1F5A3A, PART_PETALS},
    {"Early summer", 0xFF2EC4B6, 0xFF0F4A5A, PART_SPARKLE}, {"Summer", 0xFF2EC4B6, 0xFF0F4A5A, PART_SPARKLE},
    {"Summer", 0xFFF2C230, 0xFF12305A, PART_SPARKLE},   {"Summer", 0xFFF2C230, 0xFF12305A, PART_SPARKLE},
    {"Harvest", 0xFFE8B822, 0xFF5A3A12, PART_LEAVES},   {"Halloween", 0xFFF28C28, 0xFF3B0F4F, PART_LEAVES},
    {"Autumn", 0xFFD9772B, 0xFF4A2410, PART_LEAVES},    {"Holidays", 0xFFE0584A, 0xFF123D2A, PART_SNOW}};
static const season *cur_season;

#define NUM_PARTS 36
static struct {
  float x, y, vx, vy, r, ph;
} parts[NUM_PARTS];
static int parts_ready;

static float frand(void) {
  return (float)(rand() & 0xFFFF) / 65535.f;
}

static void reset_part(int i, int top) {
  parts[i].x = frand() * 660.f - 10.f;
  parts[i].y = top ? -10.f - frand() * 40.f : frand() * 480.f;
  parts[i].vx = (frand() - 0.5f) * 0.4f;
  parts[i].vy = 0.25f + frand() * 0.55f;
  parts[i].r = 1.2f + frand() * 2.2f;
  parts[i].ph = frand() * 6.28f;
}

static void apply_theme(void) {
  sw_prefs *p = sw_lib_prefs();
  accent_col = accents[p->accent % NUM_ACCENTS].col;
  cur_season = NULL;
  if (p->backdrop == BACKDROP_SEASONAL) {
    time_t t = rtc_unix_secs();
    struct tm tmv;
    gmtime_r(&t, &tmv);
    cur_season = &seasons[tmv.tm_mon % 12];
    accent_col = cur_season->accent;
  }
  sw_audio_settings(p->music, p->music_vol, p->sfx, p->sfx_vol);
}

static void draw_particles(void) {
  if (!cur_season || cur_season->particles == PART_NONE)
    return;
  if (!parts_ready) {
    for (int i = 0; i < NUM_PARTS; i++) reset_part(i, 0);
    parts_ready = 1;
  }
  const int kind = cur_season->particles;
  for (int i = 0; i < NUM_PARTS; i++) {
    parts[i].ph += 0.03f;
    parts[i].x += parts[i].vx + sinf(parts[i].ph) * (kind == PART_SNOW ? 0.35f : 0.6f);
    parts[i].y += kind == PART_SPARKLE ? -parts[i].vy * 0.4f : parts[i].vy;
    if (parts[i].y > 490.f || parts[i].y < -20.f || parts[i].x < -20.f || parts[i].x > 660.f) {
      reset_part(i, 1);
      if (kind == PART_SPARKLE) parts[i].y = 490.f;
    }
    uint32_t c;
    float a = 0.5f + 0.5f * sinf(parts[i].ph * 1.7f);
    switch (kind) {
      case PART_SNOW: c = SW_ALPHA(0xFFFFFFFF, 0x70 + (int)(a * 0x50)); break;
      case PART_LEAVES: c = SW_ALPHA(i & 1 ? 0xFFD9772B : 0xFFB8452A, 0x90); break;
      case PART_PETALS: c = SW_ALPHA(i & 1 ? 0xFFFFB3D1 : 0xFFFFFFFF, 0x80); break;
      default: c = SW_ALPHA(cur_season->accent, (int)(a * 0x90)); break;
    }
    if (kind == PART_LEAVES || kind == PART_PETALS)
      sw_rrect(parts[i].x, parts[i].y, parts[i].r * 2.2f, parts[i].r * 1.4f, parts[i].r * 0.7f, c);
    else
      sw_circle(parts[i].x, parts[i].y, parts[i].r, c);
  }
}

static void show_toast(const char *msg) {
  strncpy(toast, msg, sizeof(toast) - 1);
  toast[sizeof(toast) - 1] = 0;
  toast_frames = 120;
}

static sw_game *G(int idx) {
  return sw_lib_game(idx);
}

static int get_small(const sw_game *g, image *img) {
  txr_get_small(g->item->product, img);
  return img->texture && img->texture != img_empty_boxart.texture;
}

static int get_large(const sw_game *g, image *img) {
  txr_get_large(g->item->product, img);
  return img->texture && img->texture != img_empty_boxart.texture;
}

/* Box art: the sharp 256x256 cover is read from BOX.DAT only once the selection has settled, so fast
 * scrolling never waits on the SD card. Until then the 128x128 icon stands in. To avoid a visible jump from
 * soft to sharp, covers that are already in memory show sharp at once, the neighbours of the selection are
 * read ahead while the menu is idle, and a cover that still has to load fades in over the icon. */
static int large_cached(const sw_game *g) {
  return g && txr_large_cached(g->item->product);
}

static unsigned frame_no;
static const sw_game *sharp_g;
static int sharp_t;
static unsigned sharp_frame;
#define SHARP_STEPS 8

/* 0 = icon only, 1 = sharp cover only, in between = cross fade */
static float art_sharp(const sw_game *g, int threshold) {
  if (g != sharp_g) {
    sharp_g = g;
    sharp_t = large_cached(g) ? SHARP_STEPS : 0;
    sharp_frame = frame_no;
  } else if (sharp_frame != frame_no) {
    sharp_frame = frame_no;
    if (sharp_t == 0 && focus_frames > threshold)
      sharp_t = 1;
    else if (sharp_t > 0 && sharp_t < SHARP_STEPS)
      sharp_t++;
  }
  return (float)sharp_t / SHARP_STEPS;
}

static void prefetch_large(const sw_game *g) {
  if (g && !large_cached(g)) {
    image img;
    get_large(g, &img);
  }
}

/* average colour of a cover texture sitting in video RAM (RGB565 / ARGB1555 / ARGB4444, not VQ) */
static uint32_t cover_color(const image *img, uint32_t fallback) {
  if (!img->texture || img->texture == img_empty_boxart.texture)
    return fallback;
  if (img->format & PVR_TXRFMT_VQ_ENABLE)
    return fallback;
  /* 32 bit aligned reads only: texture RAM sits behind the 64 bit bus */
  const volatile uint32_t *px32 = (const volatile uint32_t *)((uintptr_t)img->texture & ~3u);
  uint32_t n = img->width * img->height;
  uint32_t r = 0, g = 0, b = 0, cnt = 0;
  uint32_t pf = img->format & (7 << 27);
  for (uint32_t i = 97; i < n; i += n / 61 + 1, cnt++) {
    uint16_t v = (uint16_t)(px32[i >> 1] & 0xFFFF);
    if (pf == PVR_TXRFMT_RGB565) {
      r += (v >> 11 & 31) * 255 / 31; g += (v >> 5 & 63) * 255 / 63; b += (v & 31) * 255 / 31;
    } else if (pf == PVR_TXRFMT_ARGB4444) {
      r += (v >> 8 & 15) * 17; g += (v >> 4 & 15) * 17; b += (v & 15) * 17;
    } else {
      r += (v >> 10 & 31) * 255 / 31; g += (v >> 5 & 31) * 255 / 31; b += (v & 31) * 255 / 31;
    }
  }
  if (!cnt) return fallback;
  r /= cnt; g /= cnt; b /= cnt;
  /* push toward a rich, darker tone so text stays readable */
  uint32_t mx = r > g ? (r > b ? r : b) : (g > b ? g : b);
  if (mx < 40) mx = 40;
  float k = 150.f / mx;
  r = (uint32_t)(r * k); g = (uint32_t)(g * k); b = (uint32_t)(b * k);
  if (r > 255) r = 255;
  if (g > 255) g = 255;
  if (b > 255) b = 255;
  return 0xFF000000 | (r << 16) | (g << 8) | b;
}

static void set_focus(int game_idx) {
  if (game_idx == focus_game)
    return;
  focus_game = game_idx;
  focus_frames = 0;
  sw_game *g = G(game_idx);
  if (!g)
    return;
  image img;
  uint32_t c = g->color;
  if (get_small(g, &img))
    c = cover_color(&img, g->color);
  amb_prev = mix(amb_prev, amb_cur, amb_t);
  amb_cur = c;
  amb_t = 0.f;

  /* VMU: title in up to two lines of ~7 characters */
  char l1[16] = {0}, l2[16] = {0};
  const char *name = g->item->name;
  int n = 0;
  const char *p = name;
  while (*p && n < 7) {
    const char *sp = strchr(p, ' ');
    int wl = sp ? (int)(sp - p) : (int)strlen(p);
    if (n && n + 1 + wl > 8) break;
    if (n) l1[n++] = ' ';
    for (int i = 0; i < wl && n < 15; i++) l1[n++] = p[i];
    p += wl;
    while (*p == ' ') p++;
    if (n >= 8) break;
  }
  strncpy(l2, p, 15);
  if (have_vmu_dat) {
    static uint8_t bits[192] __attribute__((aligned(32)));
    if (DAT_read_file_by_ID(&vmu_dat, g->item->product, bits)) {
      sw_vmu_bitmap(bits, g->item->product);
      return;
    }
  }
  sw_vmu_text("SWIRL", l1, l2[0] ? l2 : NULL, g->discs > 1 ? "MULTI DISC" : "A:PLAY");
}

/* ---------- rumble ---------- */
static void rumble(void) {
  if (!sw_lib_prefs()->rumble)
    return;
  maple_device_t *dev = maple_enum_type(0, MAPLE_FUNC_PURUPURU);
  if (!dev)
    return;
  purupuru_effect_t e;
  e.raw = 0;
  e.motor = 1;
  e.fpow = 4;
  e.conv = true;
  e.freq = 26;
  e.inc = 0;
  purupuru_rumble(dev, &e);
}

/* ---------- building lists ---------- */
static void build_home(void) {
  static int tmp[MAX_LIST];
  home_recent_len = sw_lib_view_recent(home_list, 10);
  int n = sw_lib_view_all(tmp, SW_SORT_NAME);
  home_len = home_recent_len;
  for (int i = 0; i < n && home_len < MAX_LIST; i++) {
    int dup = 0;
    for (int j = 0; j < home_recent_len; j++)
      if (home_list[j] == tmp[i]) { dup = 1; break; }
    if (!dup) home_list[home_len++] = tmp[i];
  }
  if (home_sel >= home_len) home_sel = home_len ? home_len - 1 : 0;
}

static void build_library(void) {
  lib_len = sw_lib_view_all(lib_list, lib_sort);
  if (lib_sel >= lib_len) lib_sel = lib_len ? lib_len - 1 : 0;
}

static void build_collection(void) {
  num_cols = sw_lib_collections(cols, MAX_COLS);
  if (col_sel >= num_cols) col_sel = num_cols ? num_cols - 1 : 0;
  col_len = num_cols ? sw_lib_view_collection(cols[col_sel].id, col_list, SW_SORT_NAME) : 0;
  if (col_game_sel >= col_len) col_game_sel = col_len ? col_len - 1 : 0;
}

static void rebuild_all(void) {
  build_home();
  build_library();
  build_collection();
}

/* ---------- common drawing ---------- */
static void draw_placeholder(const sw_game *g, float x, float y, float w, float h, int text) {
  sw_rrect(x, y, w, h, 6, g->color);
  sw_glow(x + w * 0.5f, y + h * 0.42f, w * 0.42f, 0x55FFFFFF);
  if (text) {
    float size = w >= 150 ? 15 : 12;
    int font = w >= 150 ? SWF_BODY : SWF_SMALL;
    sw_rect(x, y + h - size * 2 - 8, w, size * 2 + 8, 0x70000000);
    sw_text_wrap(font, x + 6, y + h - size * 2 - 5, size, C_WHITE, g->item->name, w - 12, size + 1, 2);
  }
}

/* cover: small icon or large art, rounded, with optional ring. sharp: 0 icon, 1 large art, between = fade */
static void draw_cover(const sw_game *g, float x, float y, float s, float sharp, int ring, float alpha) {
  image sm, lg;
  int okl = sharp > 0.f && get_large(g, &lg);
  int oks = !(okl && sharp >= 0.999f) && get_small(g, &sm);
  const float old_fade = sw_get_fade();
  sw_set_fade(old_fade * alpha);
  sw_shadow(x, y + 5, s, s, s * 0.09f + 4, 0x90000000);
  if (ring)
    sw_rrect(x - 3, y - 3, s + 6, s + 6, 8, C_ORANGE);
  const float rad = s >= 150 ? 6 : 4;
  if (oks)
    sw_image_rounded(&sm, x, y, s, s, rad, C_WHITE);
  if (okl) {
    if (oks)
      sw_set_fade(old_fade * alpha * sharp);
    sw_image_rounded(&lg, x, y, s, s, rad, C_WHITE);
  }
  if (!oks && !okl)
    draw_placeholder(g, x, y, s, s, s >= 60);
  sw_set_fade(old_fade);
}

static float draw_chip(float x, float y, const char *s, uint32_t bg, uint32_t fg) {
  float w = sw_text_width(SWF_SMALL, 12, s) + 16;
  sw_rrect(x, y, w, 20, 10, bg);
  sw_text(SWF_SMALL, x + 8, y + 3, 12, fg, s);
  return w;
}

static float draw_chips(const sw_game *g, float x, float y, float max_w) {
  char buf[32];
  float cx = x;
  const char *genre = sw_lib_genre_name(g);
  const char *items[6];
  int n = 0;
  if (genre) items[n++] = genre;
  int pl = sw_lib_players(g);
  static char pbuf[20];
  if (pl == 1) { items[n++] = "1 Player"; }
  else if (pl > 1) { snprintf(pbuf, sizeof(pbuf), "1 to %d Players", pl); items[n++] = pbuf; }
  static char dbuf[12];
  if (g->discs > 1) { snprintf(dbuf, sizeof(dbuf), "%d Discs", g->discs); items[n++] = dbuf; }
  if (g->item->vga[0] == '1') items[n++] = "VGA";
  if (g->meta && (g->meta->accessories & ACCESORIES_JUMP_PACK)) items[n++] = "Jump Pack";
  if (g->meta && g->meta->network) items[n++] = "Online";
  for (int i = 0; i < n; i++) {
    float w = sw_text_width(SWF_SMALL, 12, items[i]) + 16;
    if (cx + w > x + max_w) break;
    cx += draw_chip(cx, y, items[i], C_CHIP, C_TEXT) + 6;
  }
  if (sw_lib_is_fav(g)) {
    snprintf(buf, sizeof(buf), "Favorite");
    float w = sw_text_width(SWF_SMALL, 12, buf) + 16;
    if (cx + w <= x + max_w)
      cx += draw_chip(cx, y, buf, SW_ALPHA(C_ORANGE, 0x40), mix(C_ORANGE, C_WHITE, 0.55f)) + 6;
  }
  return cx - x;
}

static float draw_button_hint(float x, float y, uint32_t col, const char *btn, const char *label, uint32_t label_col) {
  sw_circle(x + 9, y + 9, 9, col);
  uint32_t tc = (col == C_BTN_X) ? 0xFF1A1A1A : C_WHITE;
  sw_text_center(SWF_SMALL, x + 9, y + 3, 11, tc, btn);
  float w = sw_text(SWF_SMALL, x + 24, y + 2, 12, label_col, label);
  return 24 + w + 16;
}

static void draw_footer(const char *const *items, int n, const char *right) {
  float x = 32;
  for (int i = 0; i < n; i++) {
    x += sw_text(SWF_SMALL, x, 452, 12, C_DIM, items[i]) + 20;
  }
  if (right)
    sw_text_right(SWF_SMALL, 608, 452, 12, C_DIM, right);
}

/* the colour behind everything: from the selected cover, a fixed night blue, or the season */
static uint32_t ambient_color(void) {
  uint32_t c = mix(amb_prev, amb_cur, amb_t);
  switch (sw_lib_prefs()->backdrop) {
    case BACKDROP_NIGHT: return 0xFF16295A;
    case BACKDROP_SEASONAL: return cur_season ? mix(c, cur_season->glow, 0.6f) : c;
    default: return c;
  }
}

static void draw_ambient(uint32_t base) {
  /* soft coloured glow standing in for a blurred cover; full-screen shading on top */
  uint32_t c = ambient_color();
  sw_glow(520, 120, 330, SW_ALPHA(c, 0xB0));
  sw_glow(120, 520, 260, SW_ALPHA(base, 0x60));
  sw_grad_h(0, 0, 640, 480, 0xF005070D, 0x5905070D);
  sw_grad_v(0, 280, 640, 200, 0x0005070D, 0xF505070D);
  draw_particles();
}

static void draw_header(void) {
  /* logo plate: the Sega Dreamcast logo is taken from the NTSC-U theme background on the SD card */
  if (have_logo) {
    sw_rrect(32, 20, 122, 26, 5, 0xFFFFFFFF);
    const float u0 = 4.f / img_logo_src.width, v0 = 34.f / img_logo_src.height;
    const float u1 = 118.f / img_logo_src.width, v1 = 54.f / img_logo_src.height;
    sw_image_uv(&img_logo_src, 36, 23, 114, 20, u0, v0, u1, v1, C_WHITE, C_WHITE, C_WHITE, C_WHITE);
  } else {
    sw_text(SWF_HEAD, 32, 20, 20, C_WHITE, "SWIRL");
  }

  float x = 168;
  sw_rrect(x, 25, 16, 16, 3, 0x24EEF1F7);
  sw_text_center(SWF_SMALL, x + 8, 26, 11, C_TEXT, "L");
  x += 24;
  for (int i = 0; i < TAB_COUNT; i++) {
    int on = (i == tab) && mode != MODE_DETAIL && mode != MODE_OPTIONS;
    float w = sw_text_width(SWF_UI, 14, tab_names[i]);
    sw_text(SWF_UI, x, 25, 14, on ? C_WHITE : C_DIM, tab_names[i]);
    if (on)
      sw_rect(x, 45, w, 2, C_ORANGE);
    x += w + 14;
  }
  sw_rrect(x, 25, 16, 16, 3, 0x24EEF1F7);
  sw_text_center(SWF_SMALL, x + 8, 26, 11, C_TEXT, "R");

  /* controller ports */
  float px = x + 30;
  for (int p = 0; p < 4; p++) {
    maple_device_t *d = maple_enum_dev(p, 0);
    int on = d && (d->info.functions & MAPLE_FUNC_CONTROLLER);
    sw_circle(px + p * 9, 33, 3.f, on ? C_ORANGE : 0x4DEEF1F7);
  }
  /* clock from the Dreamcast system clock */
  time_t t = rtc_unix_secs();
  struct tm tmv;
  gmtime_r(&t, &tmv);
  if (tmv.tm_year + 1900 >= 2000) {
    char buf[16];
    if (sw_lib_prefs()->clock24)
      snprintf(buf, sizeof(buf), "%02d:%02d", tmv.tm_hour, tmv.tm_min);
    else {
      int h = tmv.tm_hour % 12;
      snprintf(buf, sizeof(buf), "%d:%02d %s", h ? h : 12, tmv.tm_min, tmv.tm_hour < 12 ? "AM" : "PM");
    }
    sw_text_right(SWF_UI, 608, 25, 14, C_TEXT, buf);
  }
}

/* ---------- HOME ---------- */
static const char *home_label(int pos, const sw_game *g, char *buf, int len) {
  sw_stat *s = sw_stat_get(g, 0);
  if (pos < home_recent_len && s && s->last) {
    if (pos == 0) return "Continue Playing";
    char d[20];
    sw_lib_format_last_played(s->last, d, sizeof(d));
    snprintf(buf, len, "Last Played %s", d);
    return buf;
  }
  if (s && s->fav) return "Favorite";
  const char *genre = sw_lib_genre_name(g);
  return genre ? genre : "Your Library";
}

static void draw_home(float slide) {
  if (home_len <= 0) {
    sw_text(SWF_HEAD, 32, 120, 20, C_WHITE, "No games found");
    sw_text_wrap(SWF_BODY, 32, 150, 15, C_DIM, "Add games to your SD card with GDMENUCardManager, then rebuild the menu.", 420, 20, 3);
    return;
  }
  sw_game *g = G(home_list[home_sel]);
  set_focus(home_list[home_sel]);
  float ox = slide;

  /* text column */
  char lbuf[40];
  const char *label = home_label(home_sel, g, lbuf, sizeof(lbuf));
  char upper[40];
  int i;
  for (i = 0; label[i] && i < 39; i++) upper[i] = toupper((unsigned char)label[i]);
  upper[i] = 0;
  sw_text(SWF_SMALL, 32 + ox, 78, 12, C_ORANGE, upper);
  int lines = sw_text_wrap(SWF_TITLE, 32 + ox, 96, 30, C_WHITE, g->item->name, 330, 34, 2);
  float y = 96 + lines * 34 + 8;
  draw_chips(g, 32 + ox, y, 340);
  y += 30;
  const char *desc = (g->meta && g->meta->description[0]) ? g->meta->description : NULL;
  if (desc)
    sw_text_wrap(SWF_BODY, 32 + ox, y, 15, SW_ALPHA(C_TEXT, 0xDC), desc, 330, 19, lines > 1 ? 2 : 3);
  else {
    char info[64];
    snprintf(info, sizeof(info), "Slot %02u on your SD card.", g->item->slot_num);
    sw_text(SWF_BODY, 32 + ox, y, 15, C_DIM, info);
  }
  float bx = 32 + ox;
  bx += draw_button_hint(bx, 282, C_BTN_A, "A", "Play", C_TEXT);
  bx += draw_button_hint(bx, 282, C_BTN_Y, "Y", sw_lib_is_fav(g) ? "Unfavorite" : "Favorite", C_TEXT);
  draw_button_hint(bx, 282, C_BTN_X, "X", "Details", C_TEXT);

  /* hero cover + reflection */
  const float sharp = art_sharp(g, 12);
  draw_cover(g, 400, 66, 208, sharp, 0, 1.f);
  {
    image img;
    int ok = sharp >= 0.5f ? get_large(g, &img) : get_small(g, &img);
    if (ok)
      sw_image_uv(&img, 400, 280, 208, 34, 0.f, 1.f, 1.f, 0.84f, 0x48FFFFFF, 0x48FFFFFF, 0x00FFFFFF, 0x00FFFFFF);
    else
      sw_grad_v(400, 280, 208, 34, SW_ALPHA(g->color, 0x48), SW_ALPHA(g->color, 0x00));
  }

  /* carousel */
  const char *row = home_sel < home_recent_len ? "Recently Played" : "All Games";
  float lw = sw_text(SWF_UI, 32, 318, 14, C_TEXT, row);
  char cnt[32];
  if (home_sel < home_recent_len)
    snprintf(cnt, sizeof(cnt), "then All Games %d", sw_lib_count());
  else
    snprintf(cnt, sizeof(cnt), "%d games", sw_lib_count());
  sw_text(SWF_SMALL, 32 + lw + 10, 320, 12, C_FAINT, cnt);

  const float small = 76, big = 96, gap = 12;
  float target = home_sel - 1;
  if (target < 0) target = 0;
  if (target > home_len - 6) target = home_len - 6 > 0 ? home_len - 6 : 0;
  home_scroll = approach(home_scroll, target, 0.25f);
  int first = (int)home_scroll - 1;
  if (first < 0) first = 0;
  float x = 32 - (home_scroll - first) * (small + gap);
  for (int k = first; k < home_len && x < 660; k++) {
    int sel = (k == home_sel);
    float s = sel ? big : small;
    float yy = 436 - s;
    if (x + s > -10) {
      float a = 1.f;
      if (x < 32) a = 1.f - (32 - x) / (small + gap);
      if (x + s > 608) a = 1.f - (x + s - 608) / (small + gap);
      if (a < 0.f) a = 0.f;
      if (k == home_recent_len && home_recent_len > 0)
        sw_rect(x - gap / 2 - 1, 350, 2, 80, 0x30EEF1F7);
      draw_cover(G(home_list[k]), x, yy, s, 0, sel, a);
    }
    x += s + gap;
  }

  char pos[24];
  snprintf(pos, sizeof(pos), "%d of %d", home_sel + 1, home_len);
  const char *foot[] = {"L / R  Tabs", "D-Pad  Browse", "Down  Surprise me", "Start  Settings"};
  draw_footer(foot, 4, pos);
}

/* ---------- LIBRARY ---------- */
static const char *sort_names[SW_SORT_COUNT] = {"Name", "Recently played", "Most played", "Release year"};

static void draw_game_strip(const sw_game *g, float x, float y, float w) {
  sw_rrect(x, y, w, 58, 8, C_PANEL);
  sw_text_clip(SWF_HEAD, x + 14, y + 8, 20, C_WHITE, g->item->name, w - 28);
  float cx = x + 14;
  sw_stat *s = sw_stat_get(g, 0);
  char buf[64];
  if (s && s->plays) {
    char d[20];
    sw_lib_format_last_played(s->last, d, sizeof(d));
    snprintf(buf, sizeof(buf), "Played %u time%s. Last played %s.", s->plays, s->plays == 1 ? "" : "s", d);
  } else if (g->year_ok) {
    snprintf(buf, sizeof(buf), "Released %u. Not played yet.", g->year);
  } else {
    snprintf(buf, sizeof(buf), "Not played yet.");
  }
  float cw = draw_chips(g, cx, y + 33, w * 0.55f);
  sw_text_clip(SWF_SMALL, cx + cw + 6, y + 36, 12, C_DIM, buf, w - 28 - cw - 6);
}

static void draw_library(float slide) {
  char head[48];
  snprintf(head, sizeof(head), "%d games", lib_len);
  float w = sw_text(SWF_HEAD, 32 + slide, 64, 20, C_WHITE, "Library");
  sw_text(SWF_SMALL, 32 + slide + w + 10, 70, 12, C_DIM, head);
  char sbuf[40];
  snprintf(sbuf, sizeof(sbuf), "Sort: %s", sort_names[lib_sort]);
  sw_text_right(SWF_SMALL, 608, 70, 12, C_DIM, sbuf);
  if (lib_len <= 0)
    return;
  int row = lib_sel / LIB_COLS;
  if (row < lib_top_row) lib_top_row = row;
  if (row >= lib_top_row + LIB_ROWS) lib_top_row = row - LIB_ROWS + 1;
  const float tile = 84, gap = 12, x0 = 32, y0 = 98;
  for (int r = 0; r < LIB_ROWS; r++) {
    for (int c = 0; c < LIB_COLS; c++) {
      int idx = (lib_top_row + r) * LIB_COLS + c;
      if (idx >= lib_len) break;
      draw_cover(G(lib_list[idx]), x0 + slide + c * (tile + gap), y0 + r * (tile + gap), tile, 0, idx == lib_sel, 1.f);
    }
  }
  /* scrollbar */
  int rows_total = (lib_len + LIB_COLS - 1) / LIB_COLS;
  if (rows_total > LIB_ROWS) {
    float h = 276.f * LIB_ROWS / rows_total;
    float yy = 98 + 276.f * lib_top_row / rows_total;
    sw_rrect(620, 98, 4, 276, 2, 0x20EEF1F7);
    sw_rrect(620, yy, 4, h, 2, 0x90EEF1F7);
  }
  sw_game *g = G(lib_list[lib_sel]);
  set_focus(lib_list[lib_sel]);
  draw_game_strip(g, 32, 382, 576);
  const char *foot[] = {"L / R  Tabs", "X  Sort", "Y  Favorite", "Keyboard  Type to jump"};
  draw_footer(foot, 4, "A  Open");
}

/* ---------- COLLECTIONS ---------- */
static void draw_collections(float slide) {
  /* sidebar */
  sw_rrect(32 + slide, 64, 176, 372, 8, C_PANEL);
  if (col_sel < col_top) col_top = col_sel;
  if (col_sel >= col_top + COL_SIDEBAR_ROWS) col_top = col_sel - COL_SIDEBAR_ROWS + 1;
  for (int i = 0; i < COL_SIDEBAR_ROWS && col_top + i < num_cols; i++) {
    int ci = col_top + i;
    float y = 74 + i * 27;
    int on = ci == col_sel;
    if (on) {
      sw_rect(32 + slide, y, 176, 26, SW_ALPHA(C_ORANGE, col_focus_grid ? 0x16 : 0x2E));
      sw_rect(32 + slide, y, 3, 26, C_ORANGE);
    }
    sw_text_clip(SWF_SMALL, 46 + slide, y + 6, 13, on ? C_WHITE : SW_ALPHA(C_TEXT, 0xD0), cols[ci].name, 120);
    char n[8];
    snprintf(n, sizeof(n), "%d", cols[ci].count);
    sw_text_right(SWF_SMALL, 198 + slide, y + 7, 12, C_FAINT, n);
  }
  if (num_cols <= 0)
    return;

  /* title */
  float w = sw_text(SWF_HEAD, 226 + slide, 64, 20, C_WHITE, cols[col_sel].name);
  char sub[24];
  snprintf(sub, sizeof(sub), "%d game%s", col_len, col_len == 1 ? "" : "s");
  sw_text(SWF_SMALL, 226 + slide + w + 10, 70, 12, C_DIM, sub);

  if (col_len <= 0) {
    const char *msg = cols[col_sel].id == 1 ? "Press Y on any game to add it to Favorites." : "Nothing here yet.";
    sw_text_wrap(SWF_BODY, 226 + slide, 110, 15, C_DIM, msg, 380, 20, 3);
    const char *foot[] = {"L / R  Tabs", "Up / Down  Collections"};
    draw_footer(foot, 2, NULL);
    return;
  }
  int row = col_game_sel / COL_GRID_COLS;
  if (row < col_top_row) col_top_row = row;
  if (row >= col_top_row + COL_GRID_ROWS) col_top_row = row - COL_GRID_ROWS + 1;
  const float tile = 68, gap = 10, x0 = 226, y0 = 100;
  for (int r = 0; r < COL_GRID_ROWS; r++) {
    for (int c = 0; c < COL_GRID_COLS; c++) {
      int idx = (col_top_row + r) * COL_GRID_COLS + c;
      if (idx >= col_len) break;
      draw_cover(G(col_list[idx]), x0 + slide + c * (tile + gap) + 2, y0 + r * (tile + gap + 8), tile, 0,
                 col_focus_grid && idx == col_game_sel, 1.f);
    }
  }
  /* detail panel */
  sw_game *g = G(col_list[col_game_sel]);
  set_focus(col_list[col_game_sel]);
  float px = 226 + slide, py = 270, pw = 382;
  sw_rrect(px, py, pw, 166, 8, C_PANEL);
  float tw = sw_text_clip(SWF_HEAD, px + 16, py + 12, 17, C_WHITE, g->item->name, pw - 110);
  if (g->year_ok) {
    char yb[8];
    snprintf(yb, sizeof(yb), "%u", g->year);
    sw_text(SWF_SMALL, px + 16 + tw + 10, py + 16, 12, C_FAINT, yb);
  }
  draw_chips(g, px + 16, py + 40, pw - 32);
  const char *desc = (g->meta && g->meta->description[0]) ? g->meta->description : "No description in META.DAT for this title.";
  sw_text_wrap(SWF_BODY, px + 16, py + 68, 14, SW_ALPHA(C_TEXT, 0xDC), desc, pw - 32, 18, 3);
  sw_stat *s = sw_stat_get(g, 0);
  char buf[64];
  if (s && s->plays) {
    char d[20];
    sw_lib_format_last_played(s->last, d, sizeof(d));
    snprintf(buf, sizeof(buf), "Played %u time%s. Last played %s.", s->plays, s->plays == 1 ? "" : "s", d);
  } else
    snprintf(buf, sizeof(buf), "Not played yet.");
  sw_text(SWF_SMALL, px + 16, py + 144, 12, C_FAINT, buf);

  if (col_focus_grid) {
    const char *foot[] = {"L / R  Tabs", "B  Collections", "Y  Favorite"};
    draw_footer(foot, 3, "A  Open");
  } else {
    const char *foot[] = {"L / R  Tabs", "Up / Down  Collections", "Right  Games"};
    draw_footer(foot, 3, "A  Open");
  }
}

/* ---------- SYSTEM ---------- */
enum {
  SYS_STYLE = 0, SYS_ACCENT, SYS_BACKDROP, SYS_QUALITY, SYS_MUSIC, SYS_MUSIC_VOL, SYS_SFX, SYS_SFX_VOL, SYS_RESUME, SYS_CLOCK,
  SYS_RUMBLE, SYS_BEEP, SYS_ATTRACT, SYS_SAVER_STYLE, SYS_SAVER_TIME, SYS_SAVER_TEST, SYS_VMU, SYS_SAVE, SYS_PADTEST,
  SYS_BIOS, SYS_COUNT
};
#define SYS_ROWS 9
static int sys_top;
static const char *style_names[] = {"SWIRL", "Classic list", "Classic grid", "GDMENU"};
static const int style_values[] = {UI_SWIRL, UI_LINE_DESC, UI_GRID3, UI_GDMENU};

static const char *sys_value(int i, char *buf, int len) {
  sw_prefs *p = sw_lib_prefs();
  openmenu_settings *s = settings_get();
  switch (i) {
    case SYS_STYLE: return style_names[sys_style];
    case SYS_ACCENT: return p->backdrop == BACKDROP_SEASONAL ? "Set by season" : accents[p->accent % NUM_ACCENTS].name;
    case SYS_BACKDROP:
      if (p->backdrop == BACKDROP_SEASONAL && cur_season) {
        snprintf(buf, len, "Seasonal: %s", cur_season->name);
        return buf;
      }
      return backdrop_names[p->backdrop % BACKDROP_COUNT];
    case SYS_QUALITY: return p->quality ? "Standard" : "High";
    case SYS_MUSIC: return !sw_audio_has_music() ? "No music on card" : (p->music ? "On" : "Off");
    case SYS_MUSIC_VOL: snprintf(buf, len, "%d / 10", p->music_vol); return buf;
    case SYS_SFX: return p->sfx ? "On" : "Off";
    case SYS_SFX_VOL: snprintf(buf, len, "%d / 10", p->sfx_vol); return buf;
    case SYS_RESUME: return p->resume ? "Last played game" : "Home";
    case SYS_CLOCK: return p->clock24 ? "24 hour" : "12 hour";
    case SYS_RUMBLE: return p->rumble ? "On" : "Off";
    case SYS_BEEP: return s->beep == BEEP_ON ? "On" : "Off";
    case SYS_ATTRACT: return p->attract ? "On" : "Off";
    case SYS_SAVER_STYLE: return saver_names[p->saver_style % SAVER_COUNT];
    case SYS_SAVER_TIME: snprintf(buf, len, "After %d min", p->saver_min); return buf;
    case SYS_SAVER_TEST: return "Press A";
    case SYS_SAVE: return sw_lib_dirty() ? "Unsaved changes" : "Saved";
    default: return "";
  }
}

static const char *sys_names[SYS_COUNT] = {"Menu style", "Accent colour", "Backdrop", "Picture quality", "Menu music", "Music volume",
                                           "Navigation sounds", "Sound volume", "Start on", "Clock",
                                           "Rumble on launch", "VMU beep on save", "Screen saver", "Screen saver style",
                                           "Start screen saver", "Preview screen saver", "VMU saves",
                                           "Save settings to VMU", "Controller test", "Exit to Dreamcast BIOS"};

static void show_logo_on_vmu(void) {
  sw_vmu_show_logo();
  focus_game = -1; /* so the game's picture comes back when you leave */
}

static void draw_system(float slide) {
  show_logo_on_vmu();
  sw_text(SWF_HEAD, 32 + slide, 64, 20, C_WHITE, "System");
  if (sys_sel < sys_top) sys_top = sys_sel;
  if (sys_sel >= sys_top + SYS_ROWS) sys_top = sys_sel - SYS_ROWS + 1;
  sw_rrect(32 + slide, 98, 330, SYS_ROWS * 36 + 14, 8, C_PANEL);
  for (int r = 0; r < SYS_ROWS && sys_top + r < SYS_COUNT; r++) {
    int i = sys_top + r;
    float y = 105 + r * 36;
    int on = i == sys_sel;
    if (on) {
      sw_rrect(38 + slide, y, 318, 32, 6, SW_ALPHA(C_ORANGE, 0x2E));
      sw_rect(38 + slide, y + 6, 3, 20, C_ORANGE);
    }
    sw_text(SWF_UI, 52 + slide, y + 8, 14, on ? C_WHITE : SW_ALPHA(C_TEXT, 0xD0), sys_names[i]);
    char buf[40];
    const char *v = sys_value(i, buf, sizeof(buf));
    if (v[0]) {
      float vw = sw_text_width(SWF_SMALL, 12, v);
      if (i < SYS_VMU && i != SYS_SAVER_TEST && on) {
        sw_text(SWF_SMALL, 342 + slide - vw - 14, y + 10, 12, C_ORANGE, "<");
        sw_text(SWF_SMALL, 342 + slide + 2, y + 10, 12, C_ORANGE, ">");
      }
      if (i == SYS_ACCENT && sw_lib_prefs()->backdrop != BACKDROP_SEASONAL)
        sw_circle(342 + slide - vw - 26, y + 16, 5, C_ORANGE);
      sw_text_right(SWF_SMALL, 342 + slide, y + 10, 12, on ? C_WHITE : C_DIM, v);
    }
  }
  if (SYS_COUNT > SYS_ROWS) {
    float h = (SYS_ROWS * 36.f) * SYS_ROWS / SYS_COUNT;
    float yy = 105 + (SYS_ROWS * 36.f) * sys_top / SYS_COUNT;
    sw_rrect(356 + slide, 105, 3, SYS_ROWS * 36, 1.5f, 0x20EEF1F7);
    sw_rrect(356 + slide, yy, 3, h, 1.5f, 0x90EEF1F7);
  }

  /* library stats card */
  float x = 380 + slide;
  sw_rrect(x, 98, 228, 200, 8, C_PANEL);
  sw_text(SWF_UI, x + 16, 110, 14, C_WHITE, "Your library");
  int total = sw_lib_count(), favs = 0, played = 0, plays = 0;
  for (int i = 0; i < total; i++) {
    sw_stat *s = sw_stat_get(G(i), 0);
    if (s) {
      favs += s->fav ? 1 : 0;
      played += s->plays ? 1 : 0;
      plays += s->plays;
    }
  }
  const char *labels[4] = {"Games on SD card", "Favorites", "Games played", "Total launches"};
  int vals[4] = {total, favs, played, plays};
  for (int i = 0; i < 4; i++) {
    sw_text(SWF_SMALL, x + 16, 140 + i * 36, 12, C_DIM, labels[i]);
    char b[12];
    snprintf(b, sizeof(b), "%d", vals[i]);
    sw_text_right(SWF_HEAD, x + 212, 134 + i * 36, 20, C_WHITE, b);
  }
  sw_rrect(x, 310, 228, 126, 8, C_PANEL);
  sw_text(SWF_UI, x + 16, 322, 14, C_WHITE, "About SWIRL");
  sw_text_right(SWF_SMALL, x + 212, 324, 12, C_DIM, "Version " SWIRL_VERSION);
  sw_text(SWF_SMALL, x + 16, 344, 12, C_TEXT, "Created by Glen Huszar");
  sw_text(SWF_SMALL, x + 16, 360, 12, C_ORANGE, "github.com/TheGlengineer");
  sw_text_wrap(SWF_SMALL, x + 16, 382, 11, C_DIM,
               "Built on openMenu by mrneo240. Fonts: Sora and Barlow (OFL).", 196, 14, 3);

  const char *foot[] = {"L / R  Tabs", "Left / Right  Change", "A  Select"};
  draw_footer(foot, 3, NULL);
}

/* ---------- DETAIL ---------- */
static void open_detail(int game_idx, int ret_mode) {
  detail_game = game_idx;
  detail_return_mode = ret_mode;
  sw_game *g = G(game_idx);
  sw_lib_disc_list(g, detail_discs, &detail_num_discs, 9);
  detail_disc = 0;
  mode = MODE_DETAIL;
  tab_anim = 0.f;
  set_focus(game_idx);
}

static void draw_detail(float slide) {
  sw_game *g = G(detail_game);
  if (!g) return;
  float ox = slide;
  /* cover and VMU preview */
  draw_cover(g, 32 + ox, 64, 176, art_sharp(g, 6), 0, 1.f);
  sw_text(SWF_SMALL, 32 + ox, 258, 12, C_FAINT, "VMU PREVIEW");
  sw_rrect(32 + ox, 276, 150, 102, 8, 0xFF53664F);
  sw_rrect(35 + ox, 279, 144, 96, 6, 0xFF9FB89A);
  sw_image_uv(sw_vmu_preview(), 35 + ox, 279, 144, 96, 0.f, 0.f, 48.f / 64.f, 1.f, 0xFF1D2A1B, 0xFF1D2A1B, 0xFF1D2A1B, 0xFF1D2A1B);
  if (g->meta && g->meta->vmu_blocks) {
    char b[32];
    snprintf(b, sizeof(b), "Needs %d VMU block%s", g->meta->vmu_blocks, g->meta->vmu_blocks == 1 ? "" : "s");
    sw_text(SWF_SMALL, 32 + ox, 386, 12, C_DIM, b);
  }

  /* text column */
  float x = 232 + ox, w = 376;
  int lines = sw_text_wrap(SWF_TITLE, x, 60, 28, C_WHITE, g->item->name, w, 32, 2);
  float y = 60 + lines * 32 + 4;
  char meta[80];
  const char *reg = strchr(g->item->region, 'U') ? "USA" : (strchr(g->item->region, 'J') ? "Japan" : (strchr(g->item->region, 'E') ? "Europe" : g->item->region));
  if (g->year_ok)
    snprintf(meta, sizeof(meta), "Released %u     Region %s     Slot %02u", g->year, reg, g->item->slot_num);
  else
    snprintf(meta, sizeof(meta), "Region %s     Slot %02u", reg, g->item->slot_num);
  sw_text(SWF_SMALL, x, y, 12, C_DIM, meta);
  y += 24;
  draw_chips(g, x, y, w);
  y += 32;
  const char *desc = (g->meta && g->meta->description[0]) ? g->meta->description : "No description in META.DAT for this title.";
  load_shots(g);
  int max_lines = g->discs > 1 ? 6 : 9;
  if (shot_count) max_lines = g->discs > 1 ? 2 : 4;
  int dl = sw_text_wrap(SWF_BODY, x, y, 15, SW_ALPHA(C_TEXT, 0xE0), desc, w, 19, max_lines);
  y += dl * 19 + 10;
  if (shot_count) {
    float sh_w = (w - 12) / 2.f, sh_h = sh_w * 0.75f;
    float room = (g->discs > 1 ? 360.f : 420.f) - (y + 36);
    if (sh_h > room) { sh_h = room; sh_w = sh_h / 0.75f; }
    sw_text(SWF_SMALL, x, y, 12, C_FAINT, "SCREENS");
    for (int i = 0; i < shot_count; i++) {
      float sx = x + i * (sh_w + 12), sy = y + 18;
      sw_shadow(sx, sy + 3, sh_w, sh_h, 8, 0x80000000);
      sw_image_uv(&shot_img[i], sx, sy, sh_w, sh_h, 0.f, 0.f, 1.f, 0.75f, C_WHITE, C_WHITE, C_WHITE, C_WHITE);
    }
    y += 18 + sh_h + 10;
  }

  sw_stat *s = sw_stat_get(g, 0);
  char buf[64];
  if (s && s->plays) {
    char d[20];
    sw_lib_format_last_played(s->last, d, sizeof(d));
    snprintf(buf, sizeof(buf), "Played %u time%s. Last played %s.", s->plays, s->plays == 1 ? "" : "s", d);
  } else
    snprintf(buf, sizeof(buf), "Not played yet.");
  sw_text(SWF_SMALL, x, y, 12, C_FAINT, buf);

  if (detail_num_discs > 1) {
    float dy = 374;
    sw_text(SWF_SMALL, x, dy, 12, C_FAINT, "CHOOSE DISC");
    for (int i = 0; i < detail_num_discs; i++) {
      float bx = x + i * 48;
      int on = i == detail_disc;
      sw_rrect(bx, dy + 18, 40, 32, 5, on ? C_ORANGE : 0x1FEEF1F7);
      char n[4];
      snprintf(n, sizeof(n), "%d", detail_discs[i]->disc[0] - '0');
      sw_text_center(SWF_UI, bx + 20, dy + 26, 14, on ? 0xFF1A0F05 : C_TEXT, n);
    }
  }

  float bx = 32;
  char play[24];
  if (detail_num_discs > 1)
    snprintf(play, sizeof(play), "Play Disc %d", detail_discs[detail_disc]->disc[0] - '0');
  else
    snprintf(play, sizeof(play), "Play");
  if (is_psx_disc(detail_discs[detail_disc]))
    snprintf(play, sizeof(play), "Play in Bleem");
  bx += draw_button_hint(bx, 448, C_BTN_A, "A", play, C_TEXT);
  bx += draw_button_hint(bx, 448, C_BTN_X, "X", launch_is_custom(g) ? "Options (custom)" : "Options", C_TEXT);
  bx += draw_button_hint(bx, 448, C_BTN_Y, "Y", sw_lib_is_fav(g) ? "Unfavorite" : "Favorite", C_TEXT);
  draw_button_hint(bx, 448, C_BTN_B, "B", "Back", C_TEXT);
  char slot[24];
  snprintf(slot, sizeof(slot), "Slot %02u on SD", detail_discs[detail_disc]->slot_num);
  sw_text_right(SWF_SMALL, 608, 452, 12, C_DIM, slot);
}

/* ---------- launch ---------- */
enum { KIND_NORMAL = 0, KIND_CB, KIND_BLEEM };
static int launch_kind;
static launch_opts launch_o;

static void begin_launch_kind(const sw_game *g, const gd_item *disc, int kind) {
  if (is_psx_disc(disc)) {
    if (have_bleem < 0) have_bleem = bleem_available();
    if (!have_bleem) {
      show_toast("BLEEM.BIN is not on the menu disc");
      return;
    }
    kind = KIND_BLEEM;
  }
  sw_launch l = sw_lib_launch_get(g);
  launch_o.region = l.region;
  launch_o.vga = l.vga;
  launch_o.boot = l.boot;
  sw_lib_mark_played(g);
  launch_item = disc;
  launch_kind = kind;
  launch_frames = 0;
  launch_from_detail = (mode == MODE_DETAIL || mode == MODE_OPTIONS);
  mode = MODE_LAUNCH;
  rumble();
  sw_audio_sfx(SW_SFX_SELECT);
  sw_vmu_text("SWIRL", "LOADING", NULL, NULL);
}

static void begin_launch(const sw_game *g, const gd_item *disc) {
  begin_launch_kind(g, disc, KIND_NORMAL);
}

static void draw_launch(void) {
  sw_rect(0, 0, 640, 480, 0xC005070D);
  const char *what = launch_kind == KIND_CB ? "STARTING WITH CODEBREAKER" : (launch_kind == KIND_BLEEM ? "STARTING IN BLEEM" : "STARTING");
  sw_text_center(SWF_SMALL, 320, 214, 12, C_ORANGE, what);
  sw_text_center(SWF_HEAD, 320, 232, 20, C_WHITE, launch_item ? launch_item->name : "");
  sw_text_center(SWF_SMALL, 320, 262, 12, C_DIM, "Saving your history to the VMU");
}

/* Launch in steps so the TV hears nothing sudden: fade the music, stop the sound chip while it is
   silent, save to the VMU (a blocking write), then hand over to the game. */
static void tick_launch(void) {
  static int saved_at;
  launch_frames++;
  if (launch_frames == 1) {
    sw_audio_fade_out();
    saved_at = 0;
  }
  if (launch_frames < 4 || (!sw_audio_quiet() && launch_frames < 40))
    return;
  if (!saved_at) {
    sw_audio_shutdown();
    sw_lib_save();
    settings_save();
    saved_at = launch_frames;
    return;
  }
  if (launch_frames < saved_at + 2 || !launch_item)
    return;
  switch (launch_kind) {
    case KIND_CB: dreamcast_launch_cb((gd_item *)launch_item); break;
    case KIND_BLEEM: bleem_launch((gd_item *)launch_item); break;
    default: dreamcast_launch_disc_ex((gd_item *)launch_item, &launch_o); break;
  }
  /* only returns when a launcher file was missing */
  mode = MODE_TABS;
  sw_audio_init();
  show_toast("That launcher is not on the menu disc");
}

/* ---------- surprise me ---------- */
static void surprise_me(void) {
  if (home_len < 2)
    return;
  surprise_target = rand() % home_len;
  if (surprise_target == home_sel)
    surprise_target = (surprise_target + 1) % home_len;
  surprise_frames = 42;
  sw_audio_sfx(SW_SFX_SURPRISE);
}

static void tick_surprise(void) {
  if (surprise_frames <= 0)
    return;
  surprise_frames--;
  if (surprise_frames > 0) {
    /* spin through a few covers, slowing down */
    int step = surprise_frames > 24 ? 3 : (surprise_frames > 10 ? 5 : 8);
    if (surprise_frames % step == 0)
      home_sel = rand() % home_len;
  } else {
    home_sel = surprise_target;
    show_toast("Surprise pick. Press A to play");
  }
}

/* ---------- quick resume ---------- */
static void draw_resume(void) {
  sw_game *g = G(resume_game);
  if (!g) return;
  sw_rect(0, 0, 640, 480, 0xD005070D);
  draw_cover(g, 232, 92, 176, 1, 0, 1.f);
  sw_text_center(SWF_SMALL, 320, 292, 12, C_ORANGE, "RESUMING YOUR LAST GAME");
  sw_text_center(SWF_HEAD, 320, 310, 20, C_WHITE, g->item->name);
  char b[40];
  snprintf(b, sizeof(b), "Starting in %d", resume_frames / 60 + 1);
  sw_text_center(SWF_BODY, 320, 342, 15, C_DIM, b);
  sw_rrect(220, 370, 200, 4, 2, 0x30EEF1F7);
  sw_rrect(220, 370, 200.f * resume_frames / 300.f, 4, 2, C_ORANGE);
  float x = 226;
  x += draw_button_hint(x, 392, C_BTN_A, "A", "Start now", C_TEXT);
  draw_button_hint(x, 392, C_BTN_B, "B", "Stay in SWIRL", C_TEXT);
}

static void play_game(int game_idx, int ret_mode);

static void tick_resume(void) {
  if (mode != MODE_RESUME)
    return;
  if (--resume_frames <= 0) {
    mode = MODE_TABS;
    play_game(resume_game, MODE_TABS);
  }
}

/* ---------- launch options sheet ---------- */
static const char *region_names[] = {"Game default", "Japan", "USA", "Europe"};
static const char *boot_names[] = {"Straight to the game", "Boot animation", "SEGA screen", "Animation and SEGA"};

static void open_options(void) {
  sw_game *g = G(detail_game);
  if (!g) return;
  if (have_cb < 0) have_cb = codebreaker_available();
  opt_count = 0;
  opt_items[opt_count++] = OPT_PLAY;
  if (have_cb && !is_psx_disc(detail_discs[detail_disc])) opt_items[opt_count++] = OPT_CB;
  if (!is_psx_disc(detail_discs[detail_disc])) {
    opt_items[opt_count++] = OPT_REGION;
    opt_items[opt_count++] = OPT_VIDEO;
    opt_items[opt_count++] = OPT_BOOT;
    opt_items[opt_count++] = OPT_RESET;
  }
  opt_sel = 0;
  mode = MODE_OPTIONS;
}

static void draw_options(void) {
  sw_game *g = G(detail_game);
  if (!g) return;
  sw_launch l = sw_lib_launch_get(g);
  const float w = 400, x = 120, row = 38;
  const float h = 70 + opt_count * row + 40;
  const float y = 240 - h / 2;
  sw_rect(0, 0, 640, 480, 0x9005070D);
  sw_rrect(x, y, w, h, 12, 0xF80C1222);
  sw_text(SWF_HEAD, x + 20, y + 16, 18, C_WHITE, "Launch options");
  sw_text_clip(SWF_SMALL, x + 20, y + 42, 12, C_DIM, g->item->name, w - 40);
  for (int i = 0; i < opt_count; i++) {
    float ry = y + 66 + i * row;
    int on = i == opt_sel;
    if (on) {
      sw_rrect(x + 10, ry, w - 20, row - 4, 6, SW_ALPHA(C_ORANGE, 0x2E));
      sw_rect(x + 10, ry + 7, 3, row - 18, C_ORANGE);
    }
    const char *label = "", *val = "";
    switch (opt_items[i]) {
      case OPT_PLAY: label = "Play"; break;
      case OPT_CB: label = "Play with CodeBreaker cheats"; break;
      case OPT_REGION: label = "Region"; val = region_names[l.region & 3]; break;
      case OPT_VIDEO: label = "Video"; val = l.vga ? "Force VGA" : "Game default"; break;
      case OPT_BOOT: label = "Start with"; val = boot_names[l.boot & 3]; break;
      case OPT_RESET: label = "Reset to defaults"; break;
    }
    sw_text(SWF_UI, x + 24, ry + 9, 14, on ? C_WHITE : SW_ALPHA(C_TEXT, 0xD0), label);
    if (val[0]) {
      float vw = sw_text_width(SWF_SMALL, 12, val);
      if (on) {
        sw_text(SWF_SMALL, x + w - 24 - vw - 14, ry + 11, 12, C_ORANGE, "<");
        sw_text(SWF_SMALL, x + w - 22, ry + 11, 12, C_ORANGE, ">");
      }
      sw_text_right(SWF_SMALL, x + w - 24, ry + 11, 12, on ? C_WHITE : C_DIM, val);
    }
  }
  sw_text_wrap(SWF_SMALL, x + 20, y + h - 34, 11, C_FAINT,
               "Change these only for games that fail to start. Saved to your VMU.", w - 40, 14, 2);
}

static void input_options(unsigned int btn, int pressed) {
  sw_game *g = G(detail_game);
  if (!g) { mode = MODE_TABS; return; }
  if (btn == UP && dir_pressed(btn) && opt_sel > 0) opt_sel--;
  if (btn == DOWN && dir_pressed(btn) && opt_sel < opt_count - 1) opt_sel++;
  if (btn == B && pressed) { mode = MODE_DETAIL; return; }
  int d = 0;
  if (btn == LEFT && pressed) d = -1;
  if ((btn == RIGHT || btn == A) && pressed) d = 1;
  if (!d) return;
  sw_launch l = sw_lib_launch_get(g);
  switch (opt_items[opt_sel]) {
    case OPT_PLAY: if (btn == A) begin_launch(g, detail_discs[detail_disc]); return;
    case OPT_CB: if (btn == A) begin_launch_kind(g, detail_discs[detail_disc], KIND_CB); return;
    case OPT_REGION: l.region = (l.region + d + 4) % 4; break;
    case OPT_VIDEO: l.vga = !l.vga; break;
    case OPT_BOOT: l.boot = (l.boot + d + 4) % 4; break;
    case OPT_RESET:
      if (btn != A) return;
      l.region = SW_REGION_AUTO; l.vga = 1; l.boot = SW_BOOT_NONE;
      show_toast("Launch options reset");
      break;
  }
  sw_lib_launch_set(g, l);
  save_countdown = 180;
}

static int launch_is_custom(const sw_game *g) {
  sw_launch l = sw_lib_launch_get(g);
  return l.region != SW_REGION_AUTO || !l.vga || l.boot != SW_BOOT_NONE;
}

/* ---------- VMU save manager ---------- */
#define VMU_MAX_DEV 8
#define VMU_ROWS 8
static maple_device_t *vmu_devs[VMU_MAX_DEV];
static int vmu_ndev, vmu_dev_sel;
static vmu_dir_t *vmu_dir;
static int vmu_nfiles, vmu_file_sel, vmu_top, vmu_free_blk, vmu_desc_next, vmu_confirm;
static char (*vmu_desc)[34];

static void vmu_load_device(void) {
  if (vmu_dir) { free(vmu_dir); vmu_dir = NULL; }
  vmu_nfiles = vmu_file_sel = vmu_top = vmu_desc_next = vmu_confirm = 0;
  vmu_free_blk = 0;
  if (vmu_dev_sel >= vmu_ndev) return;
  maple_device_t *dev = vmu_devs[vmu_dev_sel];
  vmu_dir_t *all = NULL;
  int n = 0;
  if (vmufs_readdir(dev, &all, &n) < 0 || !all) {
    vmu_free_blk = -1;
    return;
  }
  /* keep real files only */
  int k = 0;
  for (int i = 0; i < n; i++)
    if (all[i].filetype != 0) all[k++] = all[i];
  vmu_dir = all;
  vmu_nfiles = k;
  vmu_free_blk = vmufs_free_blocks(dev);
  if (!vmu_desc) vmu_desc = malloc(200 * sizeof(*vmu_desc));
  if (vmu_desc) memset(vmu_desc, 0, 200 * sizeof(*vmu_desc));
}

static void open_vmu_manager(void) {
  vmu_ndev = 0;
  maple_device_t *d;
  for (int i = 0; vmu_ndev < VMU_MAX_DEV && (d = maple_enum_type(i, MAPLE_FUNC_MEMCARD)); i++)
    vmu_devs[vmu_ndev++] = d;
  vmu_dev_sel = 0;
  vmu_load_device();
  mode = MODE_VMU;
}

static void vmu_name(const vmu_dir_t *e, char *out) {
  memcpy(out, e->filename, 12);
  out[12] = 0;
  for (int i = 11; i >= 0 && (out[i] == ' ' || out[i] == 0); i--) out[i] = 0;
}

/* read one save's description per frame so the screen opens instantly */
static void vmu_tick(void) {
  if (mode != MODE_VMU || !vmu_dir || !vmu_desc || vmu_desc_next >= vmu_nfiles || vmu_desc_next >= 200) return;
  int i = vmu_desc_next++;
  void *buf = NULL;
  int size = 0;
  vmu_desc[i][0] = 1; /* tried */
  if (vmufs_read_dirent(vmu_devs[vmu_dev_sel], &vmu_dir[i], &buf, &size) < 0 || !buf) return;
  int off = vmu_dir[i].hdroff * 512;
  if (off + 48 <= size) {
    const unsigned char *h = (const unsigned char *)buf + off;
    char out[33];
    int n = 0, ascii = 0;
    for (int j = 0; j < 32; j++) {
      unsigned char c = h[16 + j];
      if (c >= 32 && c < 127) { out[n++] = c; ascii++; }
      else if (c == 0) break;
      else out[n++] = ' ';
    }
    out[n] = 0;
    while (n && out[n - 1] == ' ') out[--n] = 0;
    char *p = out;
    while (*p == ' ') p++;
    if (ascii >= 3) snprintf(vmu_desc[i] + 1, 33, "%s", p);
  }
  free(buf);
}

static void draw_vmu_manager(void) {
  show_logo_on_vmu();
  sw_text(SWF_HEAD, 32, 64, 20, C_WHITE, "VMU saves");
  if (vmu_ndev == 0) {
    sw_text_wrap(SWF_BODY, 32, 104, 15, C_DIM, "No VMU or memory card found. Plug one into a controller and open this screen again.", 420, 20, 3);
    const char *foot[] = {"B  Back"};
    draw_footer(foot, 1, NULL);
    return;
  }
  /* device chips */
  float x = 170;
  for (int i = 0; i < vmu_ndev; i++) {
    char n[8];
    snprintf(n, sizeof(n), "%c%d", 'A' + vmu_devs[i]->port, vmu_devs[i]->unit);
    int on = i == vmu_dev_sel;
    sw_rrect(x, 64, 40, 24, 6, on ? C_ORANGE : 0x24EEF1F7);
    sw_text_center(SWF_UI, x + 20, 68, 14, on ? 0xFF1A0F05 : C_TEXT, n);
    x += 48;
  }
  /* list */
  sw_rrect(32, 98, 380, VMU_ROWS * 40 + 12, 8, C_PANEL);
  if (vmu_free_blk < 0) {
    sw_text(SWF_BODY, 48, 116, 15, C_DIM, "This memory card could not be read.");
  } else if (vmu_nfiles == 0) {
    sw_text(SWF_BODY, 48, 116, 15, C_DIM, "No saves on this memory card.");
  }
  if (vmu_file_sel < vmu_top) vmu_top = vmu_file_sel;
  if (vmu_file_sel >= vmu_top + VMU_ROWS) vmu_top = vmu_file_sel - VMU_ROWS + 1;
  for (int r = 0; r < VMU_ROWS && vmu_top + r < vmu_nfiles; r++) {
    int i = vmu_top + r;
    float y = 104 + r * 40;
    int on = i == vmu_file_sel;
    if (on) {
      sw_rrect(38, y, 368, 36, 6, SW_ALPHA(C_ORANGE, 0x2E));
      sw_rect(38, y + 8, 3, 20, C_ORANGE);
    }
    char fname[13];
    vmu_name(&vmu_dir[i], fname);
    const char *desc = (vmu_desc && vmu_desc[i][0] && vmu_desc[i][1]) ? vmu_desc[i] + 1 : NULL;
    if (!strcmp(fname, "SWIRL.DAT")) desc = "SWIRL settings and history";
    sw_text_clip(SWF_UI, 52, y + 3, 14, on ? C_WHITE : SW_ALPHA(C_TEXT, 0xD8), desc ? desc : fname, 280);
    sw_text(SWF_SMALL, 52, y + 20, 11, C_FAINT, fname);
    char b[16];
    snprintf(b, sizeof(b), "%d block%s", vmu_dir[i].filesize, vmu_dir[i].filesize == 1 ? "" : "s");
    sw_text_right(SWF_SMALL, 396, y + 12, 12, on ? C_WHITE : C_DIM, b);
  }
  /* info card */
  float ix = 428;
  sw_rrect(ix, 98, 180, 150, 8, C_PANEL);
  sw_text(SWF_UI, ix + 14, 110, 14, C_WHITE, "Space");
  if (vmu_free_blk >= 0) {
    int used = 0;
    for (int i = 0; i < vmu_nfiles; i++) used += vmu_dir[i].filesize;
    int total = used + vmu_free_blk;
    char b[32];
    snprintf(b, sizeof(b), "%d", vmu_free_blk);
    sw_text(SWF_TITLE, ix + 14, 132, 28, C_WHITE, b);
    sw_text(SWF_SMALL, ix + 14, 168, 12, C_DIM, "blocks free");
    sw_rrect(ix + 14, 192, 152, 8, 4, 0x30EEF1F7);
    if (total > 0) sw_rrect(ix + 14, 192, 152.f * used / total, 8, 4, C_ORANGE);
    snprintf(b, sizeof(b), "%d saves, %d blocks used", vmu_nfiles, used);
    sw_text_clip(SWF_SMALL, ix + 14, 208, 11, C_FAINT, b, 152);
  }
  if (vmu_nfiles > 0 && vmu_file_sel < vmu_nfiles) {
    const vmu_dir_t *e = &vmu_dir[vmu_file_sel];
    sw_rrect(ix, 258, 180, 110, 8, C_PANEL);
    sw_text(SWF_UI, ix + 14, 270, 14, C_WHITE, "Saved");
    char b[32];
#define BCD(v) (((v) >> 4) * 10 + ((v) & 15))
    snprintf(b, sizeof(b), "%02d/%02d/%02d%02d", BCD(e->timestamp.month), BCD(e->timestamp.day), BCD(e->timestamp.cent), BCD(e->timestamp.year));
    sw_text(SWF_BODY, ix + 14, 292, 15, C_TEXT, b);
    snprintf(b, sizeof(b), "%02d:%02d", BCD(e->timestamp.hour), BCD(e->timestamp.min));
    sw_text(SWF_SMALL, ix + 14, 314, 12, C_DIM, b);
    sw_text(SWF_SMALL, ix + 14, 338, 12, C_FAINT, e->filetype == 0xCC ? "VMU game" : "Save file");
#undef BCD
  }
  const char *foot[] = {"L / R  Memory card", "X  Delete", "B  Back"};
  draw_footer(foot, 3, NULL);

  if (vmu_confirm && vmu_file_sel < vmu_nfiles) {
    char fname[13];
    vmu_name(&vmu_dir[vmu_file_sel], fname);
    sw_rect(0, 0, 640, 480, 0x9005070D);
    sw_rrect(150, 170, 340, 140, 12, 0xF80C1222);
    sw_text(SWF_HEAD, 170, 186, 18, C_WHITE, "Delete this save?");
    const char *desc = (vmu_desc && vmu_desc[vmu_file_sel][1]) ? vmu_desc[vmu_file_sel] + 1 : fname;
    sw_text_clip(SWF_BODY, 170, 216, 15, C_DIM, desc, 300);
    sw_text(SWF_SMALL, 170, 238, 12, C_FAINT, "This cannot be undone.");
    float bx = 170;
    bx += draw_button_hint(bx, 272, C_BTN_A, "A", "Delete", C_TEXT);
    draw_button_hint(bx, 272, C_BTN_B, "B", "Keep it", C_TEXT);
  }
}

static void input_vmu(unsigned int btn, int pressed) {
  if (vmu_confirm) {
    if (btn == A && pressed && vmu_file_sel < vmu_nfiles) {
      char fname[13];
      vmu_name(&vmu_dir[vmu_file_sel], fname);
      int r = vmufs_delete(vmu_devs[vmu_dev_sel], fname);
      show_toast(r == 0 ? "Save deleted" : "Could not delete that save");
      int keep = vmu_file_sel;
      vmu_load_device();
      vmu_file_sel = keep < vmu_nfiles ? keep : (vmu_nfiles ? vmu_nfiles - 1 : 0);
    }
    if ((btn == A || btn == B) && pressed) vmu_confirm = 0;
    return;
  }
  if (btn == UP && dir_pressed(btn) && vmu_file_sel > 0) vmu_file_sel--;
  if (btn == DOWN && dir_pressed(btn) && vmu_file_sel < vmu_nfiles - 1) vmu_file_sel++;
  if ((btn == TRIG_L || btn == LEFT) && pressed && vmu_dev_sel > 0) { vmu_dev_sel--; vmu_load_device(); }
  if ((btn == TRIG_R || btn == RIGHT) && pressed && vmu_dev_sel < vmu_ndev - 1) { vmu_dev_sel++; vmu_load_device(); }
  if (btn == X && pressed && vmu_nfiles > 0) vmu_confirm = 1;
  if (btn == B && pressed) {
    mode = MODE_TABS;
    if (vmu_dir) { free(vmu_dir); vmu_dir = NULL; }
  }
}

/* ---------- screenshots ---------- */
static void load_shots(const sw_game *g) {
  if (shot_game == (int)g->key) return;
  shot_game = (int)g->key;
  shot_count = 0;
  if (!have_shot_dat) return;
  if (!shot_buf) shot_buf = memalign(32, SHOT_CHUNK);
  if (!shot_buf) return;
  if (!DAT_read_file_by_ID(&shot_dat, g->item->product, shot_buf)) return;
  for (int i = 0; i < 2; i++) {
    const uint8_t *pvr = shot_buf + i * SHOT_PVR_BYTES;
    if (memcmp(pvr, "GBIX", 4) || memcmp(pvr + 16, "PVRT", 4)) break;
    if (!shot_img[i].texture) shot_img[i].texture = pvr_mem_malloc(256 * 256 * 2);
    if (!shot_img[i].texture) continue;
    pvr_txr_load(pvr + 32, shot_img[i].texture, 256 * 256 * 2);
    shot_img[i].width = shot_img[i].height = 256;
    shot_img[i].format = PVR_TXRFMT_RGB565 | PVR_TXRFMT_TWIDDLED;
    shot_count = i + 1;
  }
}

/* ---------- controller test ---------- */
static void draw_padtest(void) {
  sw_rrect(120, 90, 400, 300, 12, 0xF00C1222);
  sw_text_center(SWF_HEAD, 320, 106, 20, C_WHITE, "Controller test");
  maple_device_t *dev = maple_enum_type(0, MAPLE_FUNC_CONTROLLER);
  cont_state_t *st = dev ? (cont_state_t *)maple_dev_status(dev) : NULL;
  if (!st) {
    sw_text_center(SWF_BODY, 320, 200, 15, C_DIM, "No controller in port A");
    return;
  }
  struct { uint32_t mask; const char *name; uint32_t col; float x, y; } btn[] = {
      {CONT_A, "A", C_BTN_A, 420, 230}, {CONT_B, "B", C_BTN_B, 450, 200}, {CONT_X, "X", C_BTN_X, 390, 200},
      {CONT_Y, "Y", C_BTN_Y, 420, 170}, {CONT_START, "Start", C_ORANGE, 320, 250},
      {CONT_DPAD_UP, "", C_TEXT, 220, 170}, {CONT_DPAD_DOWN, "", C_TEXT, 220, 230},
      {CONT_DPAD_LEFT, "", C_TEXT, 190, 200}, {CONT_DPAD_RIGHT, "", C_TEXT, 250, 200}};
  for (unsigned i = 0; i < sizeof(btn) / sizeof(btn[0]); i++) {
    int on = st->buttons & btn[i].mask;
    sw_circle(btn[i].x, btn[i].y, 14, on ? btn[i].col : 0x30EEF1F7);
    if (btn[i].name[0])
      sw_text_center(SWF_SMALL, btn[i].x, btn[i].y - 7, 12, C_WHITE, btn[i].name);
  }
  float lt = st->ltrig / 255.f, rt = st->rtrig / 255.f;
  sw_rrect(160, 290, 120, 10, 5, 0x30EEF1F7);
  sw_rrect(160, 290, 120 * lt, 10, 5, C_ORANGE);
  sw_rrect(360, 290, 120, 10, 5, 0x30EEF1F7);
  sw_rrect(360, 290, 120 * rt, 10, 5, C_ORANGE);
  sw_text(SWF_SMALL, 160, 306, 12, C_DIM, "L trigger");
  sw_text(SWF_SMALL, 360, 306, 12, C_DIM, "R trigger");
  sw_circle(320, 190, 30, 0x20EEF1F7);
  sw_circle(320 + st->joyx / 128.f * 22, 190 + st->joyy / 128.f * 22, 9, C_ORANGE);
  sw_text_center(SWF_SMALL, 320, 360, 12, C_DIM, "Hold B and Start together to leave");
}

/* ---------- input ---------- */
static int dir_pressed(unsigned int btn) {
  /* initial press, then auto repeat after 16 frames every 4 frames */
  if (btn != prev_btn)
    return 1;
  return hold_frames >= 16 && ((hold_frames - 16) % 4 == 0);
}

static void change_tab(int d) {
  tab = (tab + d + TAB_COUNT) % TAB_COUNT;
  tab_anim = 0.f;
  if (tab == TAB_HOME) build_home();
  if (tab == TAB_COLLECTIONS) build_collection();
  if (tab == TAB_SYSTEM) {
    openmenu_settings *s = settings_get();
    sys_style = 0;
    for (int i = 0; i < 4; i++)
      if (style_values[i] == (int)s->ui) sys_style = i;
  }
}

static void toggle_fav(const sw_game *g) {
  sw_lib_toggle_fav(g);
  sw_audio_sfx(SW_SFX_FAV);
  show_toast(sw_lib_is_fav(g) ? "Added to Favorites" : "Removed from Favorites");
  save_countdown = 180;
}

static void play_game(int game_idx, int ret_mode) {
  sw_game *g = G(game_idx);
  if (!g) return;
  if (g->discs > 1) {
    open_detail(game_idx, ret_mode);
    return;
  }
  begin_launch(g, g->item);
}

static void keyboard_jump(void) {
  static char typed[8];
  static int typed_len, typed_timer;
  maple_device_t *kbd = maple_enum_type(0, MAPLE_FUNC_KEYBOARD);
  if (typed_timer > 0 && --typed_timer == 0)
    typed_len = 0;
  if (!kbd)
    return;
  int k;
  while ((k = kbd_queue_pop(kbd, true)) != KBD_QUEUE_END) {
    if (k < 32 || k > 126 || typed_len >= 7) continue;
    typed[typed_len++] = toupper(k);
    typed[typed_len] = 0;
    typed_timer = 60;
    int *list = tab == TAB_LIBRARY ? lib_list : home_list;
    int len = tab == TAB_LIBRARY ? lib_len : home_len;
    for (int i = 0; i < len; i++) {
      if (!strncasecmp(G(list[i])->item->name, typed, typed_len)) {
        if (tab == TAB_LIBRARY) lib_sel = i;
        else home_sel = i;
        idle_frames = 0;
        break;
      }
    }
  }
}

static void input_tabs(unsigned int btn, int pressed) {
  if (btn == TRIG_L && pressed) { change_tab(-1); return; }
  if (btn == TRIG_R && pressed) { change_tab(1); return; }
  if (btn == START && pressed && tab != TAB_SYSTEM) { tab = TAB_SYSTEM; change_tab(0); return; }

  switch (tab) {
    case TAB_HOME:
      if (home_len <= 0) break;
      if (btn == LEFT && dir_pressed(btn) && home_sel > 0) home_sel--;
      if (btn == RIGHT && dir_pressed(btn) && home_sel < home_len - 1) home_sel++;
      if (btn == UP && pressed) { tab = TAB_LIBRARY; change_tab(0); lib_sel = 0; }
      if (btn == DOWN && pressed && !surprise_frames) surprise_me();
      if (btn == A && pressed) play_game(home_list[home_sel], MODE_TABS);
      if (btn == X && pressed) open_detail(home_list[home_sel], MODE_TABS);
      if (btn == Y && pressed) toggle_fav(G(home_list[home_sel]));
      break;
    case TAB_LIBRARY:
      if (lib_len <= 0) break;
      if (btn == LEFT && dir_pressed(btn) && lib_sel > 0) lib_sel--;
      if (btn == RIGHT && dir_pressed(btn) && lib_sel < lib_len - 1) lib_sel++;
      if (btn == UP && dir_pressed(btn) && lib_sel - LIB_COLS >= 0) lib_sel -= LIB_COLS;
      if (btn == DOWN && dir_pressed(btn)) {
        if (lib_sel + LIB_COLS < lib_len) lib_sel += LIB_COLS;
        else if ((lib_sel / LIB_COLS) < (lib_len - 1) / LIB_COLS) lib_sel = lib_len - 1;
      }
      if (btn == A && pressed) open_detail(lib_list[lib_sel], MODE_TABS);
      if (btn == Y && pressed) toggle_fav(G(lib_list[lib_sel]));
      if (btn == X && pressed) {
        int keep = lib_list[lib_sel];
        lib_sort = (lib_sort + 1) % SW_SORT_COUNT;
        sw_lib_prefs()->sort = lib_sort;
        build_library();
        for (int i = 0; i < lib_len; i++)
          if (lib_list[i] == keep) lib_sel = i;
        show_toast(sort_names[lib_sort]);
      }
      if (btn == B && pressed) { tab = TAB_HOME; change_tab(0); }
      break;
    case TAB_COLLECTIONS:
      if (!col_focus_grid) {
        if (btn == UP && dir_pressed(btn) && col_sel > 0) { col_sel--; col_game_sel = 0; col_top_row = 0; build_collection(); }
        if (btn == DOWN && dir_pressed(btn) && col_sel < num_cols - 1) { col_sel++; col_game_sel = 0; col_top_row = 0; build_collection(); }
        if ((btn == RIGHT || btn == A) && pressed && col_len > 0) col_focus_grid = 1;
        if (btn == B && pressed) { tab = TAB_HOME; change_tab(0); }
      } else {
        if (btn == LEFT && dir_pressed(btn)) {
          if (col_game_sel % COL_GRID_COLS == 0) col_focus_grid = 0;
          else col_game_sel--;
        }
        if (btn == RIGHT && dir_pressed(btn) && col_game_sel < col_len - 1) col_game_sel++;
        if (btn == UP && dir_pressed(btn) && col_game_sel - COL_GRID_COLS >= 0) col_game_sel -= COL_GRID_COLS;
        if (btn == DOWN && dir_pressed(btn) && col_game_sel + COL_GRID_COLS < col_len) col_game_sel += COL_GRID_COLS;
        if (btn == A && pressed) open_detail(col_list[col_game_sel], MODE_TABS);
        if (btn == Y && pressed) {
          toggle_fav(G(col_list[col_game_sel]));
          int keep = col_list[col_game_sel];
          build_collection();
          if (col_len == 0) col_focus_grid = 0;
          for (int i = 0; i < col_len; i++)
            if (col_list[i] == keep) col_game_sel = i;
        }
        if (btn == B && pressed) col_focus_grid = 0;
      }
      break;
    case TAB_SYSTEM: {
      sw_prefs *p = sw_lib_prefs();
      openmenu_settings *s = settings_get();
      if (btn == UP && dir_pressed(btn) && sys_sel > 0) sys_sel--;
      if (btn == DOWN && dir_pressed(btn) && sys_sel < SYS_COUNT - 1) sys_sel++;
      int d = 0;
      if (btn == LEFT && dir_pressed(btn)) d = -1;
      if ((btn == RIGHT && dir_pressed(btn)) || (btn == A && pressed)) d = 1;
      if (d) {
        int changed_pref = 1;
        switch (sys_sel) {
          case SYS_STYLE:
            changed_pref = 0;
            if (btn == A) {
              if (style_values[sys_style] != UI_SWIRL) {
                s->ui = style_values[sys_style];
                sw_audio_shutdown(); /* before the blocking VMU writes */
                if (sw_lib_dirty()) sw_lib_save();
                settings_save();
                reload_ui();
                return;
              }
            } else
              sys_style = (sys_style + d + 4) % 4;
            break;
          case SYS_ACCENT:
            if (p->backdrop == BACKDROP_SEASONAL) { show_toast("Pick a backdrop other than Seasonal first"); changed_pref = 0; break; }
            p->accent = (p->accent + d + NUM_ACCENTS) % NUM_ACCENTS;
            break;
          case SYS_BACKDROP: p->backdrop = (p->backdrop + d + BACKDROP_COUNT) % BACKDROP_COUNT; break;
          case SYS_QUALITY:
            p->quality = !p->quality;
            show_toast("Applies the next time SWIRL starts");
            break;
          case SYS_MUSIC:
            if (!sw_audio_has_music()) { show_toast("Add music with SWIRL Card Manager"); changed_pref = 0; break; }
            p->music = !p->music;
            break;
          case SYS_MUSIC_VOL: p->music_vol = (uint8_t)((p->music_vol + d + 11) % 11); break;
          case SYS_SFX: p->sfx = !p->sfx; break;
          case SYS_SFX_VOL: p->sfx_vol = (uint8_t)((p->sfx_vol + d + 11) % 11); break;
          case SYS_RESUME: p->resume = !p->resume; break;
          case SYS_CLOCK: p->clock24 = !p->clock24; break;
          case SYS_RUMBLE: p->rumble = !p->rumble; if (p->rumble) rumble(); break;
          case SYS_BEEP: s->beep = s->beep == BEEP_ON ? BEEP_OFF : BEEP_ON; changed_pref = 0; break;
          case SYS_ATTRACT: p->attract = !p->attract; break;
          case SYS_SAVER_STYLE: p->saver_style = (uint8_t)((p->saver_style + d + SAVER_COUNT) % SAVER_COUNT); break;
          case SYS_SAVER_TIME: p->saver_min = (uint8_t)((p->saver_min - 1 + d + 30) % 30 + 1); break;
          case SYS_SAVER_TEST:
            changed_pref = 0;
            if (btn == A) saver_start();
            break;
          case SYS_VMU:
            changed_pref = 0;
            if (btn == A) open_vmu_manager();
            break;
          case SYS_SAVE:
            changed_pref = 0;
            if (btn == A) {
              if (sw_lib_save_async() == 0) show_toast("Saving to VMU");
              settings_save();
            }
            break;
          case SYS_PADTEST:
            changed_pref = 0;
            if (btn == A) mode = MODE_PADTEST;
            break;
          case SYS_BIOS:
            changed_pref = 0;
            if (btn == A) {
              sw_audio_shutdown();
              if (sw_lib_dirty()) sw_lib_save();
              arch_menu();
            }
            break;
        }
        if (changed_pref) {
          sw_lib_mark_dirty();
          save_countdown = 180;
          apply_theme();
          if (sys_sel == SYS_SFX_VOL || sys_sel == SYS_SFX) sw_audio_sfx(SW_SFX_SELECT);
        }
      }
      if (btn == B && pressed) { tab = TAB_HOME; change_tab(0); }
    } break;
  }
}

static void input_detail(unsigned int btn, int pressed) {
  sw_game *g = G(detail_game);
  if (!g) { mode = MODE_TABS; return; }
  if (btn == LEFT && dir_pressed(btn) && detail_disc > 0) detail_disc--;
  if (btn == RIGHT && dir_pressed(btn) && detail_disc < detail_num_discs - 1) detail_disc++;
  if (btn == A && pressed) begin_launch(g, detail_discs[detail_disc]);
  if (btn == X && pressed) open_options();
  if (btn == Y && pressed) toggle_fav(g);
  if (btn == B && pressed) {
    mode = MODE_TABS;
    tab_anim = 0.f;
    if (tab == TAB_COLLECTIONS) build_collection();
  }
}

/* ---------- UI entry points ---------- */
FUNCTION(UI_NAME, init) {
  texman_clear();
  unsigned int t = texman_create();
  draw_load_texture_buffer("EMPTY.PVR", &img_empty_boxart, texman_get_tex_data(t));
  texman_reserve_memory(img_empty_boxart.width, img_empty_boxart.height, 2);

  t = texman_create();
  draw_load_texture_buffer("THEME/NTSC_U/BG_U_L.PVR", &img_logo_src, texman_get_tex_data(t));
  have_logo = img_logo_src.texture && img_logo_src.texture != img_empty_boxart.texture && img_logo_src.width >= 128;
  if (have_logo)
    texman_reserve_memory(img_logo_src.width, img_logo_src.height, 2);

  sw_gfx_init();
  sw_vmu_init();
  if (!have_vmu_dat) {
    DAT_init(&vmu_dat);
    have_vmu_dat = (DAT_load_parse(&vmu_dat, "VMU.DAT") == 0 && vmu_dat.chunk_size == 192);
  }
  if (!have_shot_dat) {
    DAT_init(&shot_dat);
    have_shot_dat = (DAT_load_parse(&shot_dat, "SHOT.DAT") == 0 && shot_dat.chunk_size == SHOT_CHUNK);
  }
  sw_lib_init();
  lib_sort = sw_lib_prefs()->sort % SW_SORT_COUNT;
  srand((unsigned)rtc_unix_secs());
  sw_audio_init(); /* no-op when already running */
  gdemu_before_launch = sw_audio_shutdown;
  apply_theme();
  printf("SWIRL: %d games, stats %s\n", sw_lib_count(), sw_lib_stats_loaded() ? "loaded" : "new");
}

FUNCTION(UI_NAME, setup) {
  tab = TAB_HOME;
  mode = MODE_TABS;
  home_sel = 0;
  home_scroll = 0;
  lib_sel = lib_top_row = 0;
  col_sel = col_top = col_game_sel = col_top_row = col_focus_grid = 0;
  sys_sel = 0;
  focus_game = -1;
  tab_anim = 0.f;
  prev_btn = NONE;
  hold_frames = 0;
  rebuild_all();
  sw_vmu_show_logo();
  /* quick resume, only once per power on */
  if (!resume_checked) {
    resume_checked = 1;
    resume_game = sw_lib_prefs()->resume ? sw_lib_last_played() : -1;
    if (resume_game >= 0) {
      mode = MODE_RESUME;
      resume_frames = 300;
      set_focus(resume_game);
    }
  }
}

FUNCTION_INPUT(UI_NAME, handle_input) {
  unsigned int btn = button;
  int pressed = (btn != prev_btn) && btn != NONE;
  if (btn == prev_btn && btn != NONE)
    hold_frames++;
  else
    hold_frames = 0;

  if (saver_input(btn, pressed)) {
    prev_btn = btn;
    return; /* the screen saver is showing, or the press that closed it */
  }
  if (btn != NONE)
    idle_frames = 0;

  /* remember where the cursor is so a sound can follow what changed */
  const int before[10] = {tab, mode, home_sel, lib_sel, col_sel * 1000 + col_game_sel + col_focus_grid * 100000,
                          sys_sel, detail_disc, opt_sel, vmu_file_sel + vmu_dev_sel * 1000, vmu_confirm};

  switch (mode) {
    case MODE_DETAIL:
      input_detail(btn, pressed);
      break;
    case MODE_OPTIONS:
      input_options(btn, pressed);
      break;
    case MODE_VMU:
      input_vmu(btn, pressed);
      break;
    case MODE_RESUME:
      if (btn == A && pressed) { mode = MODE_TABS; play_game(resume_game, MODE_TABS); }
      else if (btn == B && pressed) { mode = MODE_TABS; show_toast("Welcome back"); }
      break;
    case MODE_LAUNCH:
      break;
    case MODE_PADTEST: {
      maple_device_t *dev = maple_enum_type(0, MAPLE_FUNC_CONTROLLER);
      cont_state_t *st = dev ? (cont_state_t *)maple_dev_status(dev) : NULL;
      if (!st || ((st->buttons & CONT_B) && (st->buttons & CONT_START))) mode = MODE_TABS;
    } break;
    default:
      if (surprise_frames) break; /* let the spin finish */
      input_tabs(btn, pressed);
      if (tab == TAB_LIBRARY || tab == TAB_HOME) keyboard_jump();
      break;
  }
  prev_btn = btn;

  if (mode != MODE_LAUNCH && btn != NONE) {
    const int after[10] = {tab, mode, home_sel, lib_sel, col_sel * 1000 + col_game_sel + col_focus_grid * 100000,
                           sys_sel, detail_disc, opt_sel, vmu_file_sel + vmu_dev_sel * 1000, vmu_confirm};
    if (after[0] != before[0])
      sw_audio_sfx(SW_SFX_TAB);
    else if (after[1] != before[1] || after[9] != before[9])
      sw_audio_sfx((after[1] == MODE_TABS || after[9] < before[9]) ? SW_SFX_BACK : SW_SFX_SELECT);
    else
      for (int i = 2; i < 9; i++)
        if (after[i] != before[i]) {
          sw_audio_sfx(SW_SFX_MOVE);
          break;
        }
  }
}

/* While the selection rests, read ahead the sharp covers it is most likely to need next, one per step so
 * a single SD read never lands on a frame where the player is moving. */
static int intro_done;

static void prefetch_tick(void) {
  if (mode != MODE_TABS || saver_on || !intro_done) /* reading ahead would stall the start-up fade */
    return;
  const int *list = NULL;
  int len = 0, sel = 0;
  if (tab == TAB_HOME) {
    list = home_list; len = home_len; sel = home_sel;
  } else if (tab == TAB_LIBRARY) {
    list = lib_list; len = lib_len; sel = lib_sel;
  }
  if (!list || len <= 0 || sel < 0 || sel >= len)
    return;
  switch (focus_frames) {
    case 20: prefetch_large(G(list[sel])); break;
    case 28: if (sel + 1 < len) prefetch_large(G(list[sel + 1])); break;
    case 36: if (sel > 0) prefetch_large(G(list[sel - 1])); break;
    default: break;
  }
}

/* Start up: the screen comes up from black and the dashboard settles into place, instead of appearing
 * all at once when the BIOS hands over. The fade waits until the first covers have loaded (frames come
 * steadily again), so it never stutters, and it is timed by the clock rather than by frames. */
#define INTRO_FADE_MS 700
#define INTRO_MAX_HOLD_MS 2500
static int intro_steady;
static uint64_t intro_start, intro_fade_at, intro_last;

static int intro_holding(void) {
  return !intro_done && !intro_fade_at;
}

static void draw_intro(void) {
  if (intro_done)
    return;
  uint64_t now = timer_ms_gettime64();
  if (!intro_start)
    intro_start = intro_last = now;
  if (!intro_fade_at) {
    intro_steady = (now - intro_last) < 30 ? intro_steady + 1 : 0;
    /* ready: the sharp cover of the first game and the music have loaded, and frames come steadily */
    int loaded = focus_frames > 16 && sw_audio_settled();
    if ((loaded && intro_steady >= 8) || now - intro_start > INTRO_MAX_HOLD_MS)
      intro_fade_at = now;
  }
  intro_last = now;
  float t = 0.f;
  if (intro_fade_at) {
    t = (float)(now - intro_fade_at) / INTRO_FADE_MS;
    if (t >= 1.f) {
      intro_done = 1;
      return;
    }
  }
  float e = t * t * (3.f - 2.f * t); /* smoothstep */
  sw_rect(0, 0, 640, 480, (uint32_t)((1.f - e) * 255.f + 0.5f) << 24);
}

/* ---------- screen saver ----------
 * Starts after the chosen number of minutes without a button press anywhere in SWIRL (not while a game
 * is starting). Any button brings the menu back; that press does nothing else. */

static int saver_on;         /* 1 while shown (or fading out) */
static int saver_leaving;    /* fading back to the menu */
static int saver_style_now;  /* the style being shown */
static int saver_swallow;    /* the button that woke SWIRL is ignored until released */
static uint64_t saver_t0, saver_leave_t0, last_input_ms;

/* per style state */
#define DRIFT_TILES 16
static struct { float x, y, size, speed, drift; int game; } drift[DRIFT_TILES];
static int show_game, show_prev;
static uint64_t show_t0;
static float bounce_x, bounce_y, bounce_dx, bounce_dy;
static int bounce_col;

static uint64_t now_ms(void) { return timer_ms_gettime64(); }

static int saver_eligible(void) {
  return mode == MODE_TABS || mode == MODE_DETAIL || mode == MODE_OPTIONS || mode == MODE_RESUME;
}

static int random_game(void) {
  int n = sw_lib_count();
  return n > 0 ? rand() % n : -1;
}

static void drift_spawn(int i, int anywhere) {
  static const float sizes[3] = {64.f, 92.f, 128.f};
  int layer = rand() % 3;
  drift[i].size = sizes[layer];
  drift[i].speed = 0.18f + layer * 0.16f + (rand() % 100) * 0.001f; /* pixels per frame, nearer is faster */
  drift[i].drift = ((rand() % 200) - 100) * 0.0008f;
  /* spread across the screen: each new tile takes the next of 8 columns, in a scattered order */
  static int col_next;
  static const int order[8] = {0, 5, 2, 7, 3, 6, 1, 4};
  int col = order[col_next++ & 7];
  drift[i].x = col * 80.f + 40.f - drift[i].size / 2 + (float)(rand() % 41 - 20);
  drift[i].y = anywhere ? (float)(rand() % 560) - 40.f : 480.f + (rand() % 120);
  drift[i].game = random_game();
}

static void saver_start(void) {
  if (saver_on && !saver_leaving)
    return;
  sw_prefs *p = sw_lib_prefs();
  saver_style_now = p->saver_style % SAVER_COUNT;
  saver_on = 1;
  saver_leaving = 0;
  saver_t0 = now_ms();
  srand((unsigned)saver_t0);
  for (int i = 0; i < DRIFT_TILES; i++) drift_spawn(i, 1);
  /* nearer (bigger) tiles are drawn last */
  for (int i = 0; i < DRIFT_TILES; i++)
    for (int j = i + 1; j < DRIFT_TILES; j++)
      if (drift[j].size < drift[i].size) { __typeof__(drift[0]) t = drift[i]; drift[i] = drift[j]; drift[j] = t; }
  show_game = random_game();
  show_prev = -1;
  show_t0 = saver_t0;
  bounce_x = 120 + rand() % 300;
  bounce_y = 100 + rand() % 200;
  bounce_dx = (rand() & 1) ? 1.1f : -1.1f;
  bounce_dy = (rand() & 1) ? 0.8f : -0.8f;
  bounce_col = rand() % NUM_ACCENTS;
}

static void saver_stop(void) {
  if (saver_on && !saver_leaving) {
    saver_leaving = 1;
    saver_leave_t0 = now_ms();
  }
}

static int clock_text(char *buf, int len) {
  time_t t = rtc_unix_secs();
  struct tm tmv;
  gmtime_r(&t, &tmv);
  if (tmv.tm_year + 1900 < 2000)
    return 0;
  if (sw_lib_prefs()->clock24)
    snprintf(buf, len, "%02d:%02d", tmv.tm_hour, tmv.tm_min);
  else {
    int h = tmv.tm_hour % 12;
    snprintf(buf, len, "%d:%02d %s", h ? h : 12, tmv.tm_min, tmv.tm_hour < 12 ? "AM" : "PM");
  }
  return 1;
}

static void saver_cover(int game, float x, float y, float size, float alpha, int large) {
  sw_game *g = game >= 0 ? G(game) : NULL;
  if (!g) return;
  image img;
  int ok = large ? get_large(g, &img) : 0;
  if (!ok) ok = get_small(g, &img);
  const float old = sw_get_fade();
  sw_set_fade(old * alpha);
  if (ok) sw_image_rounded(&img, x, y, size, size, size >= 150 ? 8 : 5, C_WHITE);
  else sw_rrect(x, y, size, size, 6, SW_ALPHA(g->color, 0xFF));
  sw_set_fade(old);
}

static void draw_saver_drift(float t) {
  sw_grad_v(0, 0, 640, 480, 0xFF070B16, 0xFF020308);
  for (int i = 0; i < DRIFT_TILES; i++) {
    drift[i].y -= drift[i].speed;
    drift[i].x += drift[i].drift;
    if (drift[i].y < -drift[i].size - 10) {
      float sz = drift[i].size;
      drift_spawn(i, 0);
      drift[i].size = sz; /* keep the layer so the drawing order stays right */
    }
    /* fade in at the bottom, out at the top, and farther tiles dimmer */
    float edge = drift[i].y < 60 ? (drift[i].y + drift[i].size) / (60 + drift[i].size) : (drift[i].y > 380 ? (480 - drift[i].y) / 100.f : 1.f);
    if (edge < 0) edge = 0;
    if (edge > 1) edge = 1;
    float depth = 0.35f + 0.65f * (drift[i].size - 64.f) / 64.f;
    saver_cover(drift[i].game, drift[i].x, drift[i].y, drift[i].size, edge * depth, 0);
  }
  (void)t;
}

static void draw_saver_showcase(float t) {
  const uint64_t now = now_ms();
  const float period = 9000.f;
  float k = (now - show_t0) / period;
  if (k >= 1.f) {
    show_prev = show_game;
    show_game = random_game();
    show_t0 = now;
    k = 0.f;
  }
  sw_game *g = show_game >= 0 ? G(show_game) : NULL;
  uint32_t base = g ? g->color : C_DEEP;
  sw_rect(0, 0, 640, 480, 0xFF030409);
  sw_glow(320, 190, 250, SW_ALPHA(base, 0x55));
  /* each game fades in, drifts and grows a little, then fades out */
  float a = k < 0.12f ? k / 0.12f : (k > 0.88f ? (1.f - k) / 0.12f : 1.f);
  float size = 208.f + 18.f * k;
  float x = 320 - size / 2 + (k - 0.5f) * 24.f, y = 60 + (1.f - k) * 6.f;
  if (g) {
    const float old = sw_get_fade();
    sw_set_fade(old * a);
    sw_shadow(x, y + 6, size, size, 22, 0xA0000000);
    sw_set_fade(old);
    saver_cover(show_game, x, y, size, a, 1);
    sw_set_fade(old * a);
    sw_text_center(SWF_HEAD, 320, 300, 20, C_WHITE, g->item->name);
    sw_set_fade(old);
  }
  char buf[16];
  if (clock_text(buf, sizeof(buf)))
    sw_text_center(SWF_UI, 320, 420, 14, C_DIM, buf);
  (void)t;
}

static void draw_saver_swirl(float t) {
  sw_rect(0, 0, 640, 480, 0xFF020308);
  /* the whole figure wanders slowly around the screen */
  float cx = 320 + 150 * sinf(t * 0.07f), cy = 230 + 90 * sinf(t * 0.053f + 1.3f);
  const int arms = 3, dots = 24;
  for (int a = 0; a < arms; a++) {
    for (int i = 0; i < dots; i++) {
      float f = (float)i / dots;
      float ang = t * 0.6f + a * (6.2832f / arms) + f * 5.5f;
      float r = 14 + f * 150 + 6 * sinf(t * 1.3f + i * 0.4f);
      float x = cx + cosf(ang) * r, y = cy + sinf(ang) * r * 0.82f;
      uint32_t col = mix(C_ORANGE, C_WHITE, 0.5f + 0.5f * sinf(t * 0.8f + f * 4.f + a));
      uint8_t al = (uint8_t)(40 + 190 * (1.f - f));
      if (i % 3 == 0)
        sw_glow(x, y, 8 + 14 * (1.f - f), SW_ALPHA(col, al / 2));
      sw_circle(x, y, 1.8f + 2.8f * (1.f - f), SW_ALPHA(col, al));
    }
  }
  float pulse = 0.75f + 0.25f * sinf(t * 1.1f);
  sw_glow(cx, cy, 70, SW_ALPHA(C_ORANGE, (uint8_t)(0x50 * pulse)));
  sw_text_center(SWF_TITLE, cx, cy - 18, 30, SW_ALPHA(C_WHITE, (uint8_t)(255 * pulse)), "SWIRL");
  char buf[16];
  if (clock_text(buf, sizeof(buf)))
    sw_text_center(SWF_UI, cx, cy + 22, 14, C_DIM, buf);
}

static void draw_saver_bounce(float t) {
  sw_rect(0, 0, 640, 480, 0xFF010204);
  char buf[16];
  int has_clock = clock_text(buf, sizeof(buf));
  const float w = 190, h = has_clock ? 92 : 64;
  bounce_x += bounce_dx;
  bounce_y += bounce_dy;
  int hit = 0;
  if (bounce_x < 12) { bounce_x = 12; bounce_dx = -bounce_dx; hit = 1; }
  if (bounce_x + w > 628) { bounce_x = 628 - w; bounce_dx = -bounce_dx; hit = 1; }
  if (bounce_y < 12) { bounce_y = 12; bounce_dy = -bounce_dy; hit = 1; }
  if (bounce_y + h > 468) { bounce_y = 468 - h; bounce_dy = -bounce_dy; hit = 1; }
  if (hit) bounce_col = (bounce_col + 1) % NUM_ACCENTS;
  uint32_t col = accents[bounce_col].col;
  sw_glow(bounce_x + w / 2, bounce_y + h / 2, 150, SW_ALPHA(col, 0x38));
  sw_rrect(bounce_x, bounce_y, w, h, 16, SW_ALPHA(col, 0x30));
  sw_rrect_outline(bounce_x, bounce_y, w, h, 16, 2, col);
  sw_text_center(SWF_TITLE, bounce_x + w / 2, bounce_y + 14, 30, C_WHITE, "SWIRL");
  if (has_clock)
    sw_text_center(SWF_UI, bounce_x + w / 2, bounce_y + 58, 14, SW_ALPHA(col, 0xFF), buf);
  (void)t;
}

static void draw_saver_dim(float t) {
  /* the menu stays visible underneath, dimmed, breathing very slowly */
  float a = 0.80f + 0.05f * sinf(t * 0.5f);
  sw_rect(0, 0, 640, 480, (uint32_t)(a * 255.f) << 24);
}

static void draw_saver(void) {
  if (!saver_on)
    return;
  const uint64_t now = now_ms();
  float t = (now - saver_t0) / 1000.f;
  /* fade in over 1.5 s (the dim style over 3 s), out over 0.3 s */
  float in = t / (saver_style_now == SAVER_DIM ? 3.f : 1.5f);
  float vis = in > 1.f ? 1.f : in;
  if (saver_leaving) {
    float out = 1.f - (now - saver_leave_t0) / 300.f;
    if (out <= 0.f) {
      saver_on = saver_leaving = 0;
      last_input_ms = now;
      return;
    }
    if (out < vis) vis = out;
  }
  vis = vis * vis * (3.f - 2.f * vis);
  const float old = sw_get_fade();
  sw_set_fade(vis);
  switch (saver_style_now) {
    case SAVER_DRIFT: draw_saver_drift(t); break;
    case SAVER_SHOWCASE: draw_saver_showcase(t); break;
    case SAVER_SWIRL: draw_saver_swirl(t); break;
    case SAVER_BOUNCE: draw_saver_bounce(t); break;
    default: draw_saver_dim(t); break;
  }
  sw_set_fade(old);
}

/* called for every input poll: returns 1 when the input should go no further */
static int saver_input(unsigned int btn, int pressed) {
  const uint64_t now = now_ms();
  if (!last_input_ms)
    last_input_ms = now;
  if (saver_swallow) {
    if (btn == NONE) saver_swallow = 0;
    return 1;
  }
  if (btn != NONE) {
    last_input_ms = now;
    if (saver_on) {
      if (pressed) saver_stop();
      saver_swallow = 1;
      return 1;
    }
    return 0;
  }
  sw_prefs *p = sw_lib_prefs();
  if (!saver_on && p->attract && saver_eligible() && now - last_input_ms > (uint64_t)p->saver_min * 60000u)
    saver_start();
  return saver_on;
}

FUNCTION(UI_NAME, drawOP) {
  sw_gfx_frame();
  sw_rect(0, 0, 640, 480, mix(C_DEEP, ambient_color(), 0.35f));
}

FUNCTION(UI_NAME, drawTR) {
  /* per frame timers */
  if (amb_t < 1.f) amb_t = amb_t + 0.08f > 1.f ? 1.f : amb_t + 0.08f;
  if (intro_holding()) tab_anim = 0.f; /* the dashboard slides in with the fade */
  else if (tab_anim < 1.f) {
    const float step = intro_done ? 0.1f : 0.035f;
    tab_anim = tab_anim + step > 1.f ? 1.f : tab_anim + step;
  }
  focus_frames++;
  frame_no++;
  prefetch_tick();
  if (toast_frames > 0) toast_frames--;
  /* VMU writes run on a worker thread so the music keeps streaming while they happen */
  if (save_countdown > 0 && --save_countdown == 0 && sw_lib_dirty())
    sw_lib_save_async();
  {
    int r;
    if (sw_lib_save_result(&r))
      show_toast(r == 0 ? "Saved to VMU" : "Could not save: no VMU with space");
  }

  sw_vmu_tick();
  sw_audio_poll();
  tick_surprise();
  tick_resume();
  vmu_tick();
  if (mode == MODE_LAUNCH) tick_launch();

  draw_ambient(C_DEEP);

  const float e = 1.f - (1.f - tab_anim) * (1.f - tab_anim) * (1.f - tab_anim);
  const float slide = (1.f - e) * 24.f;


  draw_header();
  sw_set_fade(e);
  if (mode == MODE_DETAIL || mode == MODE_OPTIONS || (mode == MODE_LAUNCH && launch_from_detail)) {
    draw_detail(slide);
  } else if (mode == MODE_VMU) {
    draw_vmu_manager();
  } else {
    switch (tab) {
      case TAB_HOME: draw_home(slide); break;
      case TAB_LIBRARY: draw_library(slide); break;
      case TAB_COLLECTIONS: draw_collections(slide); break;
      case TAB_SYSTEM: draw_system(slide); break;
    }
  }
  sw_set_fade(1.f);

  if (mode == MODE_PADTEST) draw_padtest();
  if (mode == MODE_OPTIONS) draw_options();
  if (mode == MODE_RESUME) draw_resume();
  if (mode == MODE_LAUNCH) draw_launch();

  if (toast_frames > 0) {
    float a = toast_frames > 20 ? 1.f : toast_frames / 20.f;
    sw_set_fade(a);
    float w = sw_text_width(SWF_SMALL, 12, toast) + 28;
    sw_rrect(320 - w / 2, 404, w, 28, 14, 0xF0161E33);
    sw_text_center(SWF_SMALL, 320, 411, 12, C_WHITE, toast);
    sw_set_fade(1.f);
  }
#ifdef SW_SAVER_DEMO
  /* test build: shows every screen saver in turn, 7 s each */
  if (intro_done) {
    static uint64_t demo_t;
    static int demo_i = -1;
    if (!demo_t || now_ms() - demo_t > 7000) {
      demo_t = now_ms();
      demo_i = (demo_i + 1) % SAVER_COUNT;
      sw_lib_prefs()->saver_style = (uint8_t)demo_i;
      saver_on = 0;
      saver_start();
    }
  }
#endif
  draw_saver();
  draw_intro();
}

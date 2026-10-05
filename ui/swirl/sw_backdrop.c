/* SWIRL backdrops: see sw_backdrop.h and BACKDROPS_DESIGN.md for the rules every backdrop keeps to
   (cover art first, nothing faster than a pixel a frame, nothing above 38 percent opacity and half that
   behind the text column and the hero cover, colours only from the navy, the accent and the cover, every
   backdrop finished with motion stopped, nothing thinner than 2 px for a CRT). */
#include "sw_backdrop.h"

#include <arch/rtc.h>
#include <dc/pvr.h>
#include <kos/fs.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>

#include "../dc/pvr_texture.h"
#include "sw_audio.h"
#include "sw_codes.h"
#include "sw_gfx.h"
#include "sw_lang.h"
#include "sw_trace.h"

#define NAVY 0xFF05070D
#define NIGHT_BLUE 0xFF16295A
_Static_assert(BACKDROP_COUNT == SW_BACKDROP_COUNT, "sw_lib.h and sw_backdrop.h disagree on the backdrop count");

/* ---------- helpers ---------- */
static uint32_t cmix(uint32_t a, uint32_t b, float t) {
  if (t <= 0.f) return a;
  if (t >= 1.f) return b;
  uint32_t r = 0;
  for (int s = 0; s < 32; s += 8) {
    float ca = (a >> s) & 0xFF, cb = (b >> s) & 0xFF;
    r |= ((uint32_t)(ca + (cb - ca) * t) & 0xFF) << s;
  }
  return r;
}
static float frand(void) {
  return (float)(rand() & 0xFFFF) / 65535.f;
}
/* the text column and the hero cover: a backdrop is at its quietest there */
static int in_quiet(float x, float y) {
  return (x >= 400 && x <= 608 && y >= 60 && y <= 300) || (x >= 32 && x <= 380 && y >= 60 && y <= 300);
}
/* a band with soft ends: two gradients meeting in the middle */
static void soft_band(float x, float y, float w, float h, uint32_t c, int a) {
  sw_grad_h(x, y, w / 2, h, SW_ALPHA(c, 0), SW_ALPHA(c, a));
  sw_grad_h(x + w / 2, y, w / 2, h, SW_ALPHA(c, a), SW_ALPHA(c, 0));
}

/* ---------- state ---------- */
static float now;              /* seconds of motion, frozen when Backdrop motion is Off */
static int motion = 1;
static int backdrop, picture, pic_dim, pic_motion;
static uint32_t accent = 0xFFF28C28;
static float hour = 21.f;      /* clock, read every second */
static int hour_frames;

/* ---------- seasons ---------- */
enum { PART_NONE = 0, PART_SNOW, PART_LEAVES, PART_PETALS, PART_SPARKLE };
enum { WX_NONE = 0, WX_RAIN, WX_MIST };
typedef struct season {
  int name;
  uint32_t accent, glow;
  int particles, weather;
} season;
static const season seasons[12] = {
    {S_SEASON_WINTER, 0xFF7FC8F8, 0xFF1C3F7A, PART_SNOW, WX_NONE},        {S_SEASON_VALENTINE, 0xFFEA3C8F, 0xFF5A1638, PART_PETALS, WX_NONE},
    {S_SEASON_SPRING, 0xFF6CCB5F, 0xFF1F5A3A, PART_PETALS, WX_MIST},      {S_SEASON_SPRING, 0xFF6CCB5F, 0xFF1F5A3A, PART_PETALS, WX_RAIN},
    {S_SEASON_EARLY_SUMMER, 0xFF2EC4B6, 0xFF0F4A5A, PART_SPARKLE, WX_NONE}, {S_SEASON_SUMMER, 0xFF2EC4B6, 0xFF0F4A5A, PART_SPARKLE, WX_NONE},
    {S_SEASON_SUMMER, 0xFFF2C230, 0xFF12305A, PART_SPARKLE, WX_NONE},     {S_SEASON_SUMMER, 0xFFF2C230, 0xFF12305A, PART_SPARKLE, WX_NONE},
    {S_SEASON_HARVEST, 0xFFE8B822, 0xFF5A3A12, PART_LEAVES, WX_NONE},     {S_SEASON_HALLOWEEN, 0xFFF28C28, 0xFF3B0F4F, PART_LEAVES, WX_MIST},
    {S_SEASON_AUTUMN, 0xFFD9772B, 0xFF4A2410, PART_LEAVES, WX_RAIN},      {S_SEASON_HOLIDAYS, 0xFFE0584A, 0xFF123D2A, PART_SNOW, WX_NONE}};
static const season *cur_season;
static int cur_month;

static void read_clock(void) {
  time_t t = rtc_unix_secs();
  struct tm tmv;
  gmtime_r(&t, &tmv);
  cur_month = tmv.tm_mon % 12;
  hour = (float)tmv.tm_hour + (float)tmv.tm_min / 60.f;
}

/* dawn, day, dusk, night: a tint and how much "day" there is, blended across the hour between them */
static uint32_t sky_tint(float h, float *day) {
  const uint32_t dawn = 0xFFFF9678, dayc = 0xFFFFFFFF, dusk = 0xFFFF783C, night = 0xFF5A6EC8;
  if (h < 5.f) { *day = 0.f; return night; }
  if (h < 8.f) { *day = (h - 5.f) / 3.f; return cmix(dawn, dayc, *day); }
  if (h < 17.f) { *day = 1.f; return dayc; }
  if (h < 20.f) { *day = 1.f - (h - 17.f) / 3.f; return cmix(dusk, night, (h - 17.f) / 3.f); }
  *day = 0.f;
  return night;
}

/* ---------- particles (seasonal) ---------- */
#define NUM_PARTS 36
static struct { float x, y, vx, vy, r, ph; } parts[NUM_PARTS];
static int parts_ready;
static void reset_part(int i, int top) {
  parts[i].x = frand() * 660.f - 10.f;
  parts[i].y = top ? -10.f - frand() * 40.f : frand() * 480.f;
  parts[i].vx = (frand() - 0.5f) * 0.4f;
  parts[i].vy = 0.25f + frand() * 0.55f;
  parts[i].r = 1.2f + frand() * 2.2f;
  parts[i].ph = frand() * 6.28f;
}
static void draw_particles(int kind, uint32_t col, int half) {
  if (kind == PART_NONE) return;
  if (!parts_ready) {
    for (int i = 0; i < NUM_PARTS; i++) reset_part(i, 0);
    parts_ready = 1;
  }
  for (int i = 0; i < NUM_PARTS; i++) {
    if (motion) {
      parts[i].ph += 0.03f;
      parts[i].x += parts[i].vx + sinf(parts[i].ph) * (kind == PART_SNOW ? 0.35f : 0.6f);
      parts[i].y += kind == PART_SPARKLE ? -parts[i].vy * 0.4f : parts[i].vy;
      if (parts[i].y > 490.f || parts[i].y < -20.f || parts[i].x < -20.f || parts[i].x > 660.f) {
        reset_part(i, 1);
        if (kind == PART_SPARKLE) parts[i].y = 490.f;
      }
    }
    float a = 0.5f + 0.5f * sinf(parts[i].ph * 1.7f);
    uint32_t c;
    int al;
    switch (kind) {
      case PART_SNOW: c = 0xFFFFFFFF; al = 0x70 + (int)(a * 0x50); break;
      case PART_LEAVES: c = i & 1 ? 0xFFD9772B : 0xFFB8452A; al = 0x90; break;
      case PART_PETALS: c = i & 1 ? 0xFFFFB3D1 : 0xFFFFFFFF; al = 0x80; break;
      default: c = col; al = (int)(a * 0x90); break;
    }
    if (half) al /= 2;
    if (kind == PART_LEAVES || kind == PART_PETALS)
      sw_rrect(parts[i].x, parts[i].y, parts[i].r * 2.2f, parts[i].r * 1.4f, parts[i].r * 0.7f, SW_ALPHA(c, al));
    else
      sw_circle(parts[i].x, parts[i].y, parts[i].r, SW_ALPHA(c, al));
  }
}

/* ---------- weather (Seasonal) ---------- */
static struct { float x, y; } rain[30];
static int rain_ready;
static float mist_x[2] = {0.f, 300.f};
static void draw_weather(const season *s, float day, int half) {
  if (s->weather == WX_RAIN) {
    if (!rain_ready) {
      for (int i = 0; i < 30; i++) { rain[i].x = frand() * 640.f; rain[i].y = frand() * 480.f; }
      rain_ready = 1;
    }
    for (int i = 0; i < 30; i++) {
      if (motion) {
        rain[i].y += 50.f / 60.f;
        rain[i].x -= 0.3f;
        if (rain[i].y > 490.f) { rain[i].y = -10.f; rain[i].x = frand() * 700.f; }
      }
      float f = (rain[i].y - 280.f) / 60.f;
      if (f <= 0.f) continue;
      if (f > 1.f) f = 1.f;
      int a = (int)(0x40 * f);
      if (half) a /= 2;
      sw_rrect(rain[i].x, rain[i].y, 2, 6, 1, SW_ALPHA(0xFFC8DCFF, a));
    }
  } else if (s->weather == WX_MIST) {
    for (int i = 0; i < 2; i++) {
      if (motion) {
        mist_x[i] -= 4.f / 60.f;
        if (mist_x[i] < -400.f) mist_x[i] = 640.f;
      }
      sw_rrect(mist_x[i], 400 + i * 30, 400, 60, 30, SW_ALPHA(0xFFC8D2E6, half ? 0x0C : 0x18));
    }
  }
  /* a summer day shimmers low on the screen; the fireflies come out at night */
  if (s->particles == PART_SPARKLE && day > 0.5f)
    for (int i = 0; i < 4; i++)
      sw_rrect(200 + sinf(now / 8.f * 6.28f + i) * 3.f, 370 + i * 26, 300, 24, 12, SW_ALPHA(s->accent, half ? 0x08 : 0x10));
}

/* ---------- stars ---------- */
static struct { float x, y, ph, sp; int layer; } stars[40];
static int stars_ready;
static float shoot_p = -1.f, shoot_next = 8.f;
static void draw_stars(int half) {
  static const float speed[3] = {2.f, 5.f, 9.f}, rad[3] = {2.f, 2.5f, 3.f};
  if (!stars_ready) {
    for (int i = 0; i < 40; i++) {
      stars[i].x = frand() * 640.f;
      stars[i].y = 20.f + frand() * 300.f;
      stars[i].layer = i < 16 ? 0 : i < 30 ? 1 : 2;
      stars[i].ph = frand() * 6.28f;
      stars[i].sp = 4.f + frand() * 3.f;
    }
    stars_ready = 1;
  }
  const uint32_t c = cmix(0xFFFFFFFF, accent, 0.3f);
  for (int i = 0; i < 40; i++) {
    if (motion) {
      stars[i].x -= speed[stars[i].layer] / 60.f;
      if (stars[i].x < -5.f) stars[i].x = 645.f;
      stars[i].ph += 6.28f / (60.f * stars[i].sp);
    }
    int a = 0x30 + (int)(0x40 * (0.5f + 0.5f * sinf(stars[i].ph)));
    if (in_quiet(stars[i].x, stars[i].y)) a /= 2;
    if (half) a /= 2;
    sw_circle(stars[i].x, stars[i].y, rad[stars[i].layer], SW_ALPHA(c, a));
  }
  if (motion) {
    shoot_next -= 1.f / 60.f;
    if (shoot_next <= 0.f && shoot_p < 0.f) {
      shoot_p = 0.f;
      shoot_next = 25.f + frand() * 20.f;
    }
    if (shoot_p >= 0.f) {
      shoot_p += 1.f / 45.f;
      if (shoot_p > 1.f) shoot_p = -1.f;
    }
  }
  if (shoot_p >= 0.f) {
    const float x = 640.f - 300.f * shoot_p, y = 56.f + 84.f * shoot_p;
    const float a = 0x60 * sinf(shoot_p * 3.1416f) * (half ? 0.5f : 1.f);
    for (int k = 0; k < 4; k++)
      sw_rrect(x + k * 12, y - k * 3.4f, 40, 3, 1.5f, SW_ALPHA(0xFFFFFFFF, (int)(a * (1.f - k * 0.25f))));
  }
}

/* ---------- embers ---------- */
static struct { float x, y, v, ph, s, r; int hot; } emb[30];
static int emb_ready;
static void draw_embers(int home, int half) {
  if (!emb_ready) {
    for (int i = 0; i < 30; i++) {
      emb[i].x = 220.f + frand() * 440.f;
      emb[i].y = frand() * 480.f;
      emb[i].v = 12.f + frand() * 18.f;
      emb[i].ph = frand() * 6.28f;
      emb[i].s = 3.f + frand() * 2.f;
      emb[i].r = 2.f + frand() * 2.f;
      emb[i].hot = i & 1;
    }
    emb_ready = 1;
  }
  const uint32_t cool = cmix(accent, 0xFFFFFFFF, 0.5f);
  for (int i = 0; i < 30; i++) {
    if (motion) {
      emb[i].y -= emb[i].v / 60.f;
      emb[i].ph += 6.28f / (60.f * emb[i].s);
      if (emb[i].y < 100.f) { emb[i].y = 490.f; emb[i].x = 220.f + frand() * 440.f; }
    }
    float a = 0x20 + 0x50 * (0.5f + 0.5f * sinf(emb[i].ph));
    if (emb[i].y < 200.f) a *= (emb[i].y - 120.f) / 80.f;
    if (emb[i].y > 430.f) a *= (490.f - emb[i].y) / 60.f;
    if (emb[i].x >= 400.f && emb[i].x <= 608.f && emb[i].y < 300.f) a *= (emb[i].y - 200.f) / 100.f;
    if (home && emb[i].y >= 340.f && emb[i].y <= 440.f) a *= 0.5f;
    if (half) a *= 0.5f;
    if (a <= 1.f) continue;
    const float x = emb[i].x + sinf(emb[i].ph) * 10.f;
    const uint32_t c = emb[i].hot ? accent : cool;
    sw_circle(x, emb[i].y, emb[i].r, SW_ALPHA(c, (int)a));
    if (emb[i].r > 3.5f) sw_circle(x, emb[i].y, emb[i].r * 2.f, SW_ALPHA(c, (int)(a * 0.25f)));
  }
}

/* ---------- spiral ---------- */
static void draw_spiral(int half) {
  const float cx = 560.f, cy = 420.f, rot = now * 3.f * 3.1416f / 180.f;
  for (int arm = 0; arm < 3; arm++)
    for (int i = 0; i < 12; i++) {
      const float k = i / 11.f, r = 40.f + 380.f * k, ang = arm * 2.094f + rot + k * 4.2f;
      const float x = cx + cosf(ang) * r, y = cy + sinf(ang) * r;
      if (x < -20.f || x > 660.f || y < -20.f || y > 500.f) continue;
      float a = 0x14 + 0x34 * k;
      if (x > 560.f || y > 440.f) {
        float e = x > 560.f ? (x - 560.f) / 120.f : 0.f;
        if (y > 440.f && (y - 440.f) / 60.f > e) e = (y - 440.f) / 60.f;
        a *= 1.f - e > 0.2f ? 1.f - e : 0.2f;
      }
      if (in_quiet(x, y)) a *= 0.5f;
      if (half) a *= 0.5f;
      sw_circle(x, y, 3.f + 4.f * k, SW_ALPHA(accent, (int)a));
    }
  if (!half) sw_glow(cx, cy, 160, SW_ALPHA(accent, 0x30));
}

/* ---------- tide ---------- */
static void draw_tide(uint32_t cover, int home) {
  static const float widths[4] = {420, 560, 480, 520}, heights[4] = {28, 40, 32, 36}, speeds[4] = {8, -6, 10, -7};
  static const float y_home[4] = {404, 428, 446, 462}, y_else[4] = {330, 366, 404, 446};
  const uint32_t alt = cmix(cover, accent, 0.5f);
  for (int i = 0; i < 4; i++) {
    const float w = widths[i], h = heights[i] + 4.f * sinf(now / 9.f * 6.28f + i);
    const float span = 640.f + w;
    float x = fmodf(now * speeds[i] + i * 180.f, span);
    if (x < 0.f) x += span;
    x -= w;
    soft_band(x, home ? y_home[i] : y_else[i], w, h, i & 1 ? alt : cover, home ? 0x24 : 0x40);
  }
  sw_grad_v(0, 320, 640, 160, SW_ALPHA(NAVY, 0), SW_ALPHA(NAVY, 0x60));
}

/* ---------- horizon ---------- */
static void draw_horizon(void) {
  sw_grad_v(0, 320, 640, 160, SW_ALPHA(0xFF1C3F7A, 0x50), SW_ALPHA(0xFF9FC8F8, 0x30));
  for (int i = 0; i < 6; i++) {
    float p = fmodf(now / 12.f + i / 6.f, 1.f);
    const float y = 330.f + 150.f * p * p;
    float f = p * 8.f < (1.f - p) * 8.f ? p * 8.f : (1.f - p) * 8.f;
    if (f > 1.f) f = 1.f;
    sw_rrect(0, y, 640, 4, 2, SW_ALPHA(0xFFC8E1FF, (int)(0x30 * f)));
  }
  sw_glow(560, 300, 50, SW_ALPHA(accent, 0x30));
}

/* ---------- pulse ---------- */
static float level;
static float rings[12];
static int nrings;
static float last_beat = -1.f;
static void draw_pulse_glow(uint32_t c) {
  const float raw = sw_audio_level();
  level += raw > level ? (raw - level) * 0.35f : (raw - level) * 0.06f;
  sw_glow(520, 120, 330.f * (1.f + 0.08f * level), SW_ALPHA(c, 0xB0 + (int)(0x18 * level)));
  if (motion && raw > 0.6f && now - last_beat > 0.33f && nrings < 12) {
    rings[nrings++] = 0.f;
    last_beat = now;
  }
  for (int i = 0; i < nrings; i++) {
    if (motion) rings[i] += 1.f / 150.f;
    if (rings[i] >= 1.f) {
      rings[i] = rings[--nrings];
      i--;
      continue;
    }
    const float rad = 120.f + 240.f * rings[i];
    const int a = (int)(0x28 * (1.f - rings[i]));
    sw_glow(520, 120, rad, SW_ALPHA(accent, a));
    sw_glow(520, 120, rad - 40.f, SW_ALPHA(NAVY, a));
  }
}

/* ---------- pictures (BG/BG.DAT and BG/BGnn.PVR, written by Card Manager) ---------- */
typedef struct __attribute__((packed)) bg_entry {
  char name[20];
  uint8_t dim;
  uint8_t pad[3];
  uint32_t size;
  uint32_t crc;
} bg_entry;
_Static_assert(sizeof(bg_entry) == 32, "bg_entry is not the 32 bytes Card Manager writes");
static bg_entry bg[SW_BG_MAX];
static int bg_count;
static image pic_img;
static int pic_loaded, pic_failed;

static uint32_t crc32_of(const uint8_t *b, size_t n) {
  uint32_t c = 0xFFFFFFFFu;
  for (size_t i = 0; i < n; i++) {
    c ^= b[i];
    for (int k = 0; k < 8; k++) c = (c >> 1) ^ (0xEDB88320u & (0u - (c & 1u)));
  }
  return ~c;
}

void sw_bd_init(void) {
  bg_count = 0;
  file_t f = fs_open("/cd/BG/BG.DAT", O_RDONLY);
  if (f == FILEHND_INVALID) return;
  uint8_t hdr[12];
  if (fs_read(f, hdr, 12) == 12 && !memcmp(hdr, "SWBG", 4)) {
    uint32_t ver, cnt;
    memcpy(&ver, hdr + 4, 4);
    memcpy(&cnt, hdr + 8, 4);
    if (ver == 1 && cnt >= 1 && cnt <= SW_BG_MAX && fs_read(f, bg, cnt * sizeof(bg_entry)) == (ssize_t)(cnt * sizeof(bg_entry))) {
      bg_count = (int)cnt;
      for (int i = 0; i < bg_count; i++) {
        bg[i].name[19] = 0;
        if (!bg[i].name[0]) snprintf(bg[i].name, sizeof(bg[i].name), "Picture %d", i + 1);
        if (bg[i].dim > 10) bg[i].dim = 4;
      }
      sw_trace("backdrop pictures: %d on the disc", bg_count);
    } else {
      sw_warn(SW_WARN_PICTURE_BG, "BG.DAT: not a usable picture index");
    }
  } else {
    sw_warn(SW_WARN_PICTURE_BG, "BG.DAT: not SWIRL's picture index");
  }
  fs_close(f);
}

static void unload_picture(void) {
  if (pic_loaded && pic_img.texture) pvr_mem_free(pic_img.texture);
  memset(&pic_img, 0, sizeof(pic_img));
  pic_loaded = 0;
}

static void load_picture(int n) {
  unload_picture();
  pic_failed = 0;
  if (n < 1 || n > bg_count) {
    if (n) { pic_failed = 1; sw_warn(SW_WARN_PICTURE_BG, "picture %d: not on this disc", n); }
    return;
  }
  const bg_entry *e = &bg[n - 1];
  char path[32];
  snprintf(path, sizeof(path), "/cd/BG/BG%02d.PVR", n);
  file_t f = fs_open(path, O_RDONLY);
  if (f == FILEHND_INVALID) { pic_failed = 1; sw_warn(SW_WARN_PICTURE_BG, "%s: missing", path + 4); return; }
  const ssize_t total = fs_total(f);
  if (total <= 0 || (uint32_t)total != e->size || total > (ssize_t)pvr_internal_buffer_size()) {
    fs_close(f);
    pic_failed = 1;
    sw_warn(SW_WARN_PICTURE_BG, "%s: %ld bytes, the index says %u", path + 4, (long)total, (unsigned)e->size);
    return;
  }
  uint8_t *buf = pvr_get_internal_buffer();
  const ssize_t got = fs_read(f, buf, (size_t)total);
  fs_close(f);
  if (got != total || crc32_of(buf, (size_t)total) != e->crc) {
    pic_failed = 1;
    sw_warn(SW_WARN_PICTURE_BG, "%s: damaged (checksum)", path + 4);
    return;
  }
  uint32_t w, h, fmt;
  pvr_ptr_t t = load_pvr_from_buffer(buf, (size_t)total, &w, &h, &fmt);
  if (!t || w != 512 || h != 512) {
    if (t) pvr_mem_free(t);
    pic_failed = 1;
    sw_warn(SW_WARN_PICTURE_BG, "%s: not a 512 x 512 picture", path + 4);
    return;
  }
  pic_img.texture = t;
  pic_img.width = w;
  pic_img.height = h;
  pic_img.format = fmt;
  pic_loaded = 1;
  sw_trace("backdrop picture %d (%s) loaded, %u bytes of video memory", n, e->name, (unsigned)total);
}

/* ---------- API ---------- */
void sw_bd_apply(const sw_prefs *p) {
  motion = !p->motion_off;
  backdrop = p->backdrop < BACKDROP_COUNT ? p->backdrop : 0;
  pic_dim = p->pic_dim <= 10 ? p->pic_dim : 4;
  pic_motion = p->pic_motion < SW_PIC_MOTION_COUNT ? p->pic_motion : 0;
  read_clock();
  cur_season = &seasons[cur_month];
  const int want = p->picture <= SW_BG_MAX ? p->picture : 0;
  if (want != picture || (want && !pic_loaded && !pic_failed)) {
    picture = want;
    load_picture(want);
  }
}

void sw_bd_set_accent(uint32_t c) {
  accent = c;
}

int sw_bd_season_accent(uint32_t *c) {
  if (picture ? pic_motion != SW_PIC_MOTION_SEASONAL : backdrop != BACKDROP_SEASONAL) return 0;
  if (!cur_season) return 0;
  *c = cur_season->accent;
  return 1;
}

int sw_bd_season_name(void) {
  return cur_season ? cur_season->name : seasons[0].name;
}

int sw_bd_backdrop_name(int b) {
  static const int names[BACKDROP_COUNT] = {S_BACKDROP_COVER, S_BACKDROP_NIGHT, S_BACKDROP_SEASONAL, S_BACKDROP_TIDE, S_BACKDROP_SPIRAL,
                                             S_BACKDROP_STARFIELD, S_BACKDROP_EMBERS, S_BACKDROP_HORIZON, S_BACKDROP_PULSE};
  return names[b >= 0 && b < BACKDROP_COUNT ? b : 0];
}

int sw_bd_picture_count(void) {
  return bg_count;
}

const char *sw_bd_picture_name(int n) {
  return n >= 1 && n <= bg_count ? bg[n - 1].name : "";
}

int sw_bd_picture_failed(void) {
  return picture && pic_failed;
}

int sw_bd_picture_dim(int n) {
  return n >= 1 && n <= bg_count ? bg[n - 1].dim : 4;
}

int sw_bd_pic_motion_name(int m) {
  static const int names[SW_PIC_MOTION_COUNT] = {S_PIC_MOTION_NONE, S_BACKDROP_SEASONAL, S_BACKDROP_SPIRAL, S_BACKDROP_STARFIELD,
                                                  S_BACKDROP_EMBERS, S_BACKDROP_PULSE};
  return names[m >= 0 && m < SW_PIC_MOTION_COUNT ? m : 0];
}

uint32_t sw_bd_ambient(uint32_t cover) {
  if (picture && pic_loaded) return cover; /* the cover always wins over a picture */
  switch (backdrop) {
    case BACKDROP_NIGHT:
    case BACKDROP_STARFIELD: return NIGHT_BLUE;
    case BACKDROP_SEASONAL: {
      if (!cur_season) return cover;
      float day;
      const uint32_t tint = sky_tint(hour, &day);
      return cmix(cmix(cover, cur_season->glow, 0.6f), tint, 0.25f * (1.f - day) + 0.1f);
    }
    default: return cover;
  }
}

void sw_bd_draw(uint32_t base, uint32_t cover, int home) {
  if (motion) now += 1.f / 60.f;
  if (++hour_frames >= 60) {
    hour_frames = 0;
    read_clock();
    cur_season = &seasons[cur_month];
  }
  const int on_pic = picture && pic_loaded;
  const int what = on_pic ? -1 : backdrop;
  const int overlay = on_pic ? pic_motion : SW_PIC_MOTION_NONE;
  const uint32_t glow = sw_bd_ambient(cover);
  float day = 1.f;

  /* the picture, then its darkening */
  if (on_pic) {
    sw_image(&pic_img, 0, 0, 640, 480, 0xFFFFFFFF);
    if (pic_dim) sw_rect(0, 0, 640, 480, SW_ALPHA(0xFF000000, pic_dim * 0x13));
  }

  /* the glows: the cover's colour top right, the base bottom left */
  if (what == BACKDROP_SEASONAL && cur_season) {
    sky_tint(hour, &day);
    if (hour >= 6.f && hour <= 20.f)
      sw_glow(-40.f + 720.f * (hour - 6.f) / 14.f, 10, 90, SW_ALPHA(cmix(cur_season->accent, 0xFFFFFFFF, 0.3f), 0x40));
    else
      sw_glow(-40.f + 720.f * (hour >= 20.f ? hour - 20.f : hour + 4.f) / 9.f, 10, 60, SW_ALPHA(0xFFB4C8FF, 0x30));
    sw_glow(520, 120, 330, SW_ALPHA(glow, (int)(0xB0 * (0.6f + 0.4f * day))));
    sw_glow(120, 520, 260, SW_ALPHA(cur_season->glow, 0x60));
    draw_weather(cur_season, day, 0);
  } else if (what == BACKDROP_PULSE || overlay == SW_PIC_MOTION_PULSE) {
    draw_pulse_glow(glow);
    sw_glow(120, 520, 260, SW_ALPHA(base, 0x60));
  } else {
    sw_glow(520, 120, 330, SW_ALPHA(glow, 0xB0));
    sw_glow(120, 520, 260, SW_ALPHA(base, 0x60));
  }
  if (what == BACKDROP_SPIRAL || overlay == SW_PIC_MOTION_SPIRAL) draw_spiral(on_pic);
  if (what == BACKDROP_STARFIELD || overlay == SW_PIC_MOTION_STARFIELD) draw_stars(on_pic);
  if (what == BACKDROP_EMBERS || overlay == SW_PIC_MOTION_EMBERS) draw_embers(home, on_pic);

  /* the shading every backdrop gets: the text column and the footer stay dark whatever is behind them */
  sw_grad_h(0, 0, 640, 480, 0xF005070D, 0x5905070D);
  sw_grad_v(0, 280, 640, 200, 0x0005070D, what == BACKDROP_HORIZON ? 0xB005070D : 0xF505070D);
  if (what == BACKDROP_HORIZON) draw_horizon();
  if (what == BACKDROP_TIDE) draw_tide(cover, home);

  /* particles sit on top of the shading, as Seasonal always did */
  if (what == BACKDROP_SEASONAL && cur_season) {
    const int night = hour >= 20.f || hour < 5.f;
    if (cur_season->particles != PART_SPARKLE || night) draw_particles(cur_season->particles, cur_season->accent, 0);
  } else if (overlay == SW_PIC_MOTION_SEASONAL && cur_season) {
    sky_tint(hour, &day);
    draw_weather(cur_season, day, 1);
    draw_particles(cur_season->particles, cur_season->accent, 1);
  }
}

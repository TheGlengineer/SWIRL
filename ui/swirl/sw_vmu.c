/*
 * SWIRL: VMU screen output (48x32, 1 bit) plus an on-screen preview texture.
 *
 * What the VMU shows, in order of priority:
 *   1. a save status animation (countdown, writing, saving, saved, not saved), set every frame by the menu;
 *   2. the SWIRL logo, animated: an intro, then a loop (the owner's LOGO.VMU gets its own effects);
 *   3. the menu's picture for the game in focus, or a few lines of text.
 *
 * Frames go out 12 times a second, and only when they change, from a small thread of their own, with the
 * same KallistiOS call every VMU game uses (it waits for the VMU's reply, so it runs off the menu's thread and
 * the TV picture never waits for it). The menu only says what to show. Nothing is sent while a save is
 * writing: the "writing" picture goes out just before the write starts and stays until it ends.
 *
 * At power on the logo intro starts after the settings have been read and plays while SWIRL loads its
 * pictures. It pauses before SWIRL.DAT is used and carries on once the menu is running.
 */
#include "sw_vmu.h"
#include "sw_lib.h"
#include "sw_utf8.h"

#include <arch/timer.h>
#include <dc/maple.h>
#include <dc/maple/vmu.h>
#include <dc/pvr.h>
#include <kos/fs.h>
#include <kos/mutex.h>
#include <kos/thread.h>
#include <math.h>
#include <string.h>

#include "sw_vmu_anim.h"
#include "vmu_font.h"

#define W 48
#define H 32
#define FRAME_MS (1000 / VMU_ANIM_FPS)
#define SW_PI 3.14159265f
#define COUNT(a) ((int)(sizeof(a) / sizeof((a)[0])))

static uint8_t pix[H][W];     /* what the VMU shows (or is about to): 1 = dark pixel */
static uint8_t base[192];     /* the menu's own picture: game picture or text (row major, MSB = left) */
static char last_key[4][24];
static int base_settle;       /* 1/12 s steps a new menu picture waits, so fast scrolling doesn't flood the bus */
static image preview;
static uint16_t preview_buf[64 * 32] __attribute__((aligned(32)));
static volatile int preview_dirty;

/* ---------- what plays ---------- */
enum { SHOW_BASE = 0, SHOW_LOGO };
static int show = SHOW_BASE;
static int logo_intro;        /* 1 while the intro plays, 0 for the loop */
static int logo_boot_intro;   /* the power on intro (the logo shatters first); it always plays to the end */
static int logo_frame;
static int base_waiting;      /* the menu asked for its picture while the power on intro was still playing */

static int overlay = SW_VMU_NONE, overlay_arg, overlay_frame;
static uint64_t next_ms;
static uint8_t sent[192];     /* the last picture sent */
static int sent_valid;

/* the sending thread */
static kthread_t *thd;
static mutex_t lock = MUTEX_INITIALIZER;   /* the state above, between the menu and the thread */
static volatile int booting;               /* power on: sending before the menu runs */
static volatile uint64_t last_tick_ms;     /* the menu is running (SWIRL's style is on) */
static volatile int frozen;                /* launching a game: the VMU is left alone */
static volatile int sending;               /* the thread is talking to a VMU right now */
static volatile int held;                  /* a save is about to write */
static volatile uint64_t held_since;
static volatile int held_saw_busy;

/* ---------- the owner's own logo ---------- */
static const uint8_t default_logo[192] = {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x03, 0xfe, 0x00, 0x00, 0x00, 0x00, 0x07, 0xff, 0x80, 0x00, 0x00, 0x00, 0x0f, 0x81, 0x80, 0x00, 0x00, 0x01, 0x0f, 0x00, 0x40, 0x00, 0x00, 0x01, 0x1e, 0x00, 0x00, 0x00, 0x00, 0x03, 0x1c, 0x78, 0x00, 0x00, 0x00, 0x03, 0x1c, 0x7e, 0x00, 0x00, 0x00, 0x03, 0x8d, 0xcf, 0x00, 0x00, 0x00, 0x03, 0x85, 0xc7, 0x00, 0x00, 0x00, 0x01, 0xc3, 0xc7, 0x00, 0x00, 0x00, 0x01, 0xe0, 0x67, 0x80, 0x00, 0x00, 0x01, 0xf0, 0x67, 0x80, 0x00, 0x00, 0x00, 0xff, 0xc7, 0x80, 0x00, 0x00, 0x00, 0x7f, 0xc7, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x38, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0xa2, 0xb2, 0x00, 0x00, 0x00, 0x02, 0x22, 0xaa, 0x00, 0x00, 0x00, 0x03, 0xaa, 0xb2, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0x00, 0x00, 0x00, 0x03, 0x94, 0xab, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00};
static uint8_t logo[192];
static int logo_loaded, logo_custom;
static uint8_t logo_key[H][W]; /* the owner's logo: when each pixel appears in its intro (0..255) */

#define CUSTOM_INTRO 17
#define CUSTOM_IDLE 48

static inline int get_bit(const uint8_t *b, int x, int y) { return (b[y * 6 + x / 8] >> (7 - x % 8)) & 1; }
static inline void set_bit(uint8_t *b, int x, int y) { b[y * 6 + x / 8] |= 0x80 >> (x % 8); }

static void load_logo(void) {
  if (logo_loaded)
    return;
  logo_loaded = 1;
  memcpy(logo, default_logo, sizeof(logo));
  file_t f = fs_open("/cd/LOGO.VMU", O_RDONLY);
  if (f != FILEHND_INVALID) {
    uint8_t buf[192];
    if (fs_read(f, buf, sizeof(buf)) == (ssize_t)sizeof(buf) && memcmp(buf, default_logo, sizeof(buf))) {
      memcpy(logo, buf, sizeof(logo));
      logo_custom = 1;
    }
    fs_close(f);
  }
  if (logo_custom) {
    /* the owner's picture appears in a spiral sweep from its centre */
    float cx = 0, cy = 0;
    int n = 0;
    for (int y = 0; y < H; y++)
      for (int x = 0; x < W; x++)
        if (get_bit(logo, x, y)) { cx += x; cy += y; n++; }
    if (n) { cx /= n; cy /= n; } else { cx = 24; cy = 16; }
    for (int y = 0; y < H; y++)
      for (int x = 0; x < W; x++) {
        const float a = (atan2f(y - cy, x - cx) + SW_PI) / (2.f * SW_PI);
        const float k = sqrtf((x - cx) * (x - cx) + (y - cy) * (y - cy)) / 30.f + a * 0.35f;
        const int v = (int)(k / 1.35f * 14.f); /* the frame it appears in */
        logo_key[y][x] = (uint8_t)(v > 255 ? 255 : v);
      }
  }
}

/* one frame of the owner's logo animation */
static void custom_frame(int intro, int f, uint8_t *out) {
  memset(out, 0, 192);
  if (intro && f >= 14) {
    /* a double flash at the end of the intro */
    for (int i = 0; i < 192; i++)
      out[i] = f == 14 ? (uint8_t)~logo[i] : logo[i];
    return;
  }
  for (int y = 0; y < H; y++)
    for (int x = 0; x < W; x++) {
      if (!get_bit(logo, x, y)) continue;
      if (intro) {
        if (logo_key[y][x] <= f) set_bit(out, x, y);
      } else {
        /* a diagonal shine sweeps across every 4 seconds */
        const int p = -10 + f * 5;
        const float d = x + y * 0.6f;
        if (!(f < 14 && d >= p && d < p + 3)) set_bit(out, x, y);
      }
    }
}

/* ---------- the picture for now ---------- */
static const uint8_t *overlay_frame_bits(void) {
  switch (overlay) {
    case SW_VMU_COUNTDOWN: {
      /* in step with the banner: arg = video frames left of the 3 second countdown */
      int f = (180 - overlay_arg) / 5;
      if (f < 0) f = 0;
      if (f >= COUNT(vmu_anim_countdown)) f = COUNT(vmu_anim_countdown) - 1;
      return vmu_anim_countdown[f];
    }
    case SW_VMU_WRITING: return vmu_anim_saving_still[0];
    case SW_VMU_SAVING: return vmu_anim_saving[overlay_frame % COUNT(vmu_anim_saving)];
#define HOLD(a) a[overlay_frame < COUNT(a) ? overlay_frame : COUNT(a) - 1]
    case SW_VMU_SAVED: return HOLD(vmu_anim_saved);
    case SW_VMU_NO_SPACE: return HOLD(vmu_anim_not_saved);
    case SW_VMU_CHECK: return HOLD(vmu_anim_not_saved_check);
    case SW_VMU_BUSY: return HOLD(vmu_anim_not_saved_busy);
#undef HOLD
    default: return NULL;
  }
}

static void current(uint8_t *out) {
  const uint8_t *o = overlay_frame_bits();
  if (o) {
    memcpy(out, o, 192);
  } else if (show == SHOW_LOGO) {
    if (logo_custom)
      custom_frame(logo_intro, logo_frame, out);
    else if (logo_intro)
      memcpy(out, logo_boot_intro ? vmu_anim_logo_boot[logo_frame] : vmu_anim_logo_intro[logo_frame], 192);
    else
      memcpy(out, vmu_anim_logo_idle[logo_frame], 192);
  } else {
    memcpy(out, base, 192);
  }
}

static int intro_length(void) {
  if (logo_custom) return CUSTOM_INTRO;
  return logo_boot_intro ? COUNT(vmu_anim_logo_boot) : COUNT(vmu_anim_logo_intro);
}

/* moves every animation on by one frame (12 times a second) */
static void advance(void) {
  overlay_frame++;
  if (show != SHOW_LOGO)
    return;
  logo_frame++;
  if (logo_intro && logo_frame >= intro_length()) {
    logo_intro = 0;
    logo_boot_intro = 0;
    logo_frame = 0;
    if (base_waiting) {
      base_waiting = 0;
      show = SHOW_BASE;
    }
  } else if (!logo_intro && logo_frame >= (logo_custom ? CUSTOM_IDLE : COUNT(vmu_anim_logo_idle))) {
    logo_frame = 0;
  }
}

/* ---------- sending (on the thread) ---------- */
/* the devices KallistiOS draws on: official VMU screens */
static int is_vmu_screen(const maple_device_t *dev) {
  return dev->valid && dev->info.function_data[0] == 0x403f7e7e && (dev->info.functions & MAPLE_FUNC_LCD);
}

/* row major picture (MSB = left, 1 = dark) to the VMU's own layout, which is upside down and mirrored */
static void to_lcd(const uint8_t *bits, uint8_t *lcd) {
  memset(lcd, 0, 192);
  for (int Y = 0; Y < H; Y++)
    for (int X = 0; X < W; X++)
      if (get_bit(bits, X, Y)) {
        const int x = (W - 1) - X, y = (H - 1) - Y;
        lcd[y * (W / 8) + x / 8] |= 0x80 >> (x % 8);
      }
}

/* draws on every VMU screen and waits for each reply (KallistiOS gives up on one after 500 ms) */
static void draw_now(const uint8_t *bits) {
  uint8_t lcd[192];
  to_lcd(bits, lcd);
  maple_device_t *dev;
  for (int i = 0; i < 8 && (dev = maple_enum_type(i, MAPLE_FUNC_LCD)); i++)
    if (is_vmu_screen(dev))
      vmu_draw_lcd(dev, lcd);
  for (int y = 0; y < H; y++)
    for (int x = 0; x < W; x++)
      pix[y][x] = (uint8_t)get_bit(bits, x, y);
  preview_dirty = 1;
}

/* may the thread send now? */
static int may_send(void) {
  if (frozen)
    return 0;
  if (held) {
    /* a save said it is about to write: wait until it has written, or 2 s if it never started */
    const int busy = sw_lib_busy();
    if (busy) held_saw_busy = 1;
    if ((held_saw_busy && !busy) || timer_ms_gettime64() - held_since > 2000) {
      held = 0;
      held_saw_busy = 0;
    } else {
      return 0;
    }
  }
  if (sw_lib_busy())
    return 0;
  /* at power on, or while SWIRL's menu is running (not a Classic style, not after leaving) */
  return booting || timer_ms_gettime64() - last_tick_ms < 250;
}

static void step(void) {
  const uint64_t now = timer_ms_gettime64();
  if (now < next_ms)
    return;
  next_ms = now + FRAME_MS;
  uint8_t bits[192];
  int changed = 0;
  mutex_lock(&lock);
  if (base_settle > 0 && show == SHOW_BASE && overlay == SW_VMU_NONE) {
    base_settle--;
  } else {
    current(bits);
    changed = !sent_valid || memcmp(bits, sent, sizeof(bits));
    advance();
  }
  mutex_unlock(&lock);
  if (!changed)
    return;
  sending = 1;
  if (may_send()) {
    draw_now(bits);
    mutex_lock(&lock);
    memcpy(sent, bits, sizeof(sent));
    sent_valid = 1;
    mutex_unlock(&lock);
  }
  sending = 0;
}

static volatile int quit;

static void *vmu_thread(void *p) {
  (void)p;
  while (!quit) {
    if (may_send())
      step();
    thd_sleep(10);
  }
  return NULL;
}

static void start_thread(void) {
#ifdef SW_NO_VMU_ANIM
  return; /* test build: the VMU screen thread never runs */
#endif
  if (thd)
    return;
  quit = 0;
  thd = thd_create(0, vmu_thread, NULL);
}

/* the thread ends: before a game starts, before the BIOS and before another style. Nothing of SWIRL's may still
   be running when the console is handed over. */
void sw_vmu_shutdown(void) {
  frozen = 1;
  if (!thd)
    return;
  quit = 1;
  thd_join(thd, NULL);
  thd = NULL;
}

/* waits (at most 600 ms) until the thread is not talking to a VMU */
static void wait_quiet(void) {
  const uint64_t until = timer_ms_gettime64() + 600;
  while (sending && timer_ms_gettime64() < until)
    thd_sleep(2);
}

/* ---------- the menu's calls ---------- */
void sw_vmu_init(void) {
  /* the animation state is kept: the power on intro may still be playing */
  preview.width = 64;
  preview.height = 32;
  preview.format = PVR_TXRFMT_ARGB4444 | PVR_TXRFMT_NONTWIDDLED;
  if (!preview.texture)
    preview.texture = pvr_mem_malloc(64 * 32 * 2);
  preview_dirty = 1;
  mutex_lock(&lock);
  memset(last_key, 0, sizeof(last_key));
  sent_valid = 0;
  mutex_unlock(&lock);
  frozen = 0;
  start_thread();
}

/* the menu's own picture: shown unless a save status or the logo is showing (called with the lock held) */
static void set_base(void) {
  base_settle = 1; /* a short wait, so fast scrolling does not flood the maple bus */
  if (show == SHOW_LOGO && logo_intro && logo_boot_intro) {
    base_waiting = 1; /* the power on intro plays to the end first */
    return;
  }
  show = SHOW_BASE;
  base_waiting = 0;
}

static int text_width(const char *s) {
  int w = 0;
  for (int i = 0; s[i];) {
    unsigned char c = (unsigned char)sw_utf8_ascii(sw_utf8_next(s, &i, -1));
    if (c < 32 || c > 126) c = '?';
    w += vmu_font_adv[c - 32];
  }
  return w;
}

static void draw_text_line(uint8_t *b, int y, const char *s) {
  if (!s || !*s)
    return;
  /* centre; clip to the screen */
  int w = text_width(s);
  int x = (W - w) / 2;
  if (x < 0) x = 0;
  for (int i = 0; s[i] && x < W;) {
    unsigned char c = (unsigned char)sw_utf8_ascii(sw_utf8_next(s, &i, -1));
    if (c >= 'a' && c <= 'z') c = c - 'a' + 'A'; /* the pixel font reads best in capitals */
    if (c < 32 || c > 126) c = '?';
    const uint8_t *rows = vmu_font_rows[c - 32];
    for (int r = 0; r < 5; r++)
      for (int bx = 0; bx < 8; bx++)
        if ((rows[r] & (0x80 >> bx)) && x + bx < W && y + r < H)
          set_bit(b, x + bx, y + r);
    x += vmu_font_adv[c - 32];
  }
}

void sw_vmu_text(const char *l1, const char *l2, const char *l3, const char *l4) {
  const char *lines[4] = {l1, l2, l3, l4};
  mutex_lock(&lock);
  int same = 1;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    if (strncmp(last_key[i], s, sizeof(last_key[i]) - 1)) same = 0;
  }
  if (same && show == SHOW_BASE) {
    mutex_unlock(&lock);
    return;
  }
  int n = 0;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    strncpy(last_key[i], s, sizeof(last_key[i]) - 1);
    last_key[i][sizeof(last_key[i]) - 1] = 0;
    if (*s) n++;
  }
  memset(base, 0, sizeof(base));
  /* frame */
  for (int x = 0; x < W; x++) { set_bit(base, x, 0); set_bit(base, x, H - 1); }
  for (int y = 0; y < H; y++) { set_bit(base, 0, y); set_bit(base, W - 1, y); }
  int total = n * 5 + (n - 1) * 2;
  int y = (H - total) / 2;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    if (!*s) continue;
    draw_text_line(base, y, s);
    y += 7;
  }
  set_base();
  mutex_unlock(&lock);
}

void sw_vmu_bitmap(const uint8_t *bits, const char *key) {
  mutex_lock(&lock);
  if (!(show == SHOW_BASE && !strncmp(last_key[0], key, sizeof(last_key[0]) - 1) && !strcmp(last_key[1], "\x01img"))) {
    strncpy(last_key[0], key, sizeof(last_key[0]) - 1);
    last_key[0][sizeof(last_key[0]) - 1] = 0;
    strcpy(last_key[1], "\x01img");
    last_key[2][0] = last_key[3][0] = 0;
    memcpy(base, bits, sizeof(base));
    set_base();
  }
  mutex_unlock(&lock);
}

void sw_vmu_show_logo(void) {
  load_logo();
  mutex_lock(&lock);
  if (show == SHOW_LOGO) {
    base_waiting = 0; /* the logo is wanted after all */
  } else {
    show = SHOW_LOGO;
    logo_intro = 1;
    logo_boot_intro = 0;
    logo_frame = 0;
    next_ms = 0;
    memset(last_key, 0, sizeof(last_key));
  }
  mutex_unlock(&lock);
}

void sw_vmu_overlay(int kind, int arg) {
  mutex_lock(&lock);
  if (kind != overlay) {
    overlay = kind;
    overlay_frame = 0;
    next_ms = 0; /* show the change straight away */
  }
  overlay_arg = arg;
  mutex_unlock(&lock);
}

/* a save is about to write: the "writing" picture goes out now, and nothing else until the save is done */
void sw_vmu_before_write(void) {
  if (frozen || timer_ms_gettime64() - last_tick_ms > 250)
    return; /* launching, or SWIRL's menu isn't running */
  held_saw_busy = 0;
  held_since = timer_ms_gettime64();
  held = 1;
  wait_quiet();
  draw_now(vmu_anim_saving_still[0]);
  mutex_lock(&lock);
  memcpy(sent, vmu_anim_saving_still[0], sizeof(sent));
  sent_valid = 1;
  mutex_unlock(&lock);
}

void sw_vmu_freeze(int on) {
  frozen = on;
  if (on)
    wait_quiet();
  else
    next_ms = 0;
}

void sw_vmu_tick(void) {
  last_tick_ms = timer_ms_gettime64();
  if (preview_dirty && preview.texture) {
    preview_dirty = 0;
    for (int y = 0; y < 32; y++)
      for (int x = 0; x < 64; x++)
        preview_buf[y * 64 + x] = (x < W && pix[y][x]) ? 0xF000 : 0x0000; /* alpha = ink */
    pvr_txr_load(preview_buf, preview.texture, sizeof(preview_buf));
  }
}

const image *sw_vmu_preview(void) {
  return &preview;
}

/* ---------- power on ---------- */
/* nothing else is running yet, so the still logo is drawn straight away */
void sw_vmu_boot_logo(void) {
  load_logo();
  draw_now(logo);
}

void sw_vmu_boot_start(void) {
  load_logo();
  mutex_lock(&lock);
  show = SHOW_LOGO;
  logo_intro = 1;
  logo_boot_intro = !logo_custom; /* SWIRL's own logo shatters and forms again */
  logo_frame = 0;
  next_ms = 0;
  sent_valid = 0;
  mutex_unlock(&lock);
  booting = 1;
  start_thread();
}

/* SWIRL.DAT is next: the intro pauses until the menu runs (it carries on from there) */
void sw_vmu_boot_stop(void) {
  booting = 0;
  wait_quiet();
}

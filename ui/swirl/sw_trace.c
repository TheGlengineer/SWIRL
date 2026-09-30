/* SWIRL trace, warnings, hang watchdog and the report screen (see sw_trace.h and docs/DIAGNOSTICS.md) */
#include "sw_trace.h"
#include "sw_version.h"
#define SWIRL_REPORT_VERSION SWIRL_VERSION
#ifndef SWIRL_BUILD
#define SWIRL_BUILD "dev" /* swirl/build.sh passes the short git hash */
#endif
#ifndef SWIRL_VERSION_STR
#define SWIRL_VERSION_STR SWIRL_VERSION /* the full version (2.14.0-preview.3) comes from swirl/build.sh */
#endif

#include <arch/arch.h>
#include <arch/irq.h>
#include <arch/timer.h>
#include <assert.h>
#include <dc/biosfont.h>
#include <dc/flashrom.h>
#include <dc/maple.h>
#include <dc/maple/controller.h>
#include <dc/pvr.h>
#include <dc/video.h>
#include <kos/thread.h>
#include <malloc.h>
#include <stdarg.h>
#include <stdio.h>
#include <string.h>

#define MAX_LINES 64
#define LINE_LEN 96
#define SCREEN_CHARS 51 /* 51 characters of 12 pixels, plus the 8 pixel margin, fit in 640 */
static char lines[MAX_LINES][LINE_LEN];
static int num_lines;
static char text[MAX_LINES * LINE_LEN];

#ifdef SWIRL_TRACE_SCREEN
static int screen_on = 1; /* until the menu draws its first picture */
#else
static int screen_on = 0;
#endif

/* one line of text on the frame buffer, in whatever pixel mode is set */
static void draw_line(int row, const char *s, uint32_t fg) {
  static uint32_t tmp[640 * (BFONT_HEIGHT + 1)]; /* a spare row: bfont may touch one past the end */
  if (!vid_mode)
    return;
  /* the picture being shown (graphics chip start up moves it away from vram_s) */
  uint8_t *const shown = (uint8_t *)PVR_RAM_BASE + (PVR_GET(PVR_FB_ADDR) & (PVR_RAM_SIZE - 1));
  const int w = vid_mode->width, y = 8 + row * BFONT_HEIGHT;
  if (w != 640 || y + BFONT_HEIGHT > vid_mode->height)
    return;
  memset(tmp, 0, sizeof(tmp));
  char cut[SCREEN_CHARS + 1];
  snprintf(cut, sizeof(cut), "%s", s);
  bfont_draw_str_ex(tmp + 8, 640, fg, 0, 32, true, cut);
  for (int yy = 0; yy < BFONT_HEIGHT; yy++) {
    const uint32_t *src = tmp + yy * 640;
    switch (vid_mode->pm) {
      case PM_RGB565: {
        uint16_t *d = (uint16_t *)shown + (y + yy) * 640;
        for (int x = 0; x < 640; x++) {
          const uint32_t c = src[x];
          d[x] = ((c >> 8) & 0xF800) | ((c >> 5) & 0x07E0) | ((c >> 3) & 0x001F);
        }
        break;
      }
      case PM_RGB888P: {
        /* whole 32 bit words: video memory takes no byte writes */
        uint32_t *d = (uint32_t *)(shown + (y + yy) * 640 * 3);
        for (int x = 0; x < 640; x += 4, d += 3) {
          const uint32_t a = src[x], b = src[x + 1], c = src[x + 2], e = src[x + 3];
          d[0] = (a & 0xFFFFFF) | (b << 24);
          d[1] = ((b >> 8) & 0xFFFF) | (c << 16);
          d[2] = ((c >> 16) & 0xFF) | (e << 8);
        }
        break;
      }
      case PM_RGB0888:
        memcpy((uint32_t *)shown + (y + yy) * 640, src, 640 * 4);
        break;
      default:
        break;
    }
  }
}

/* 18 rows fit: the latest steps */
static void draw_all(uint32_t fg) {
  const int rows = 18;
  const int first = num_lines > rows ? num_lines - rows : 0;
  for (int i = first; i < num_lines; i++) draw_line(i - first, lines[i], fg);
}

void sw_trace_redraw(void) {
  if (!screen_on)
    return;
  vid_clear(0, 0, 0);
  draw_all(0xFFFFFFFF);
}

static volatile uint64_t last_alive; /* when the menu last showed signs of life (a step or a picture) */
static uint32_t x_pc, x_pr, x_stack[16]; /* where a crash or hang was: the x= line of the report */
static int x_n;
static volatile uint64_t expect_until; /* a known long wait: the watchdog stands down until then */
static kthread_t *main_thd;            /* the menu's thread, whose place is reported when it hangs */

void sw_trace_alive(void) { last_alive = timer_ms_gettime64(); }

void sw_watchdog_expect(unsigned ms) {
  const uint64_t until = timer_ms_gettime64() + ms;
  if (until > expect_until)
    expect_until = until;
  last_alive = timer_ms_gettime64();
}

/* the code addresses found on a stack, as trace lines (with the build's symbol file they name the functions) */
static void trace_stack(uint32_t sp) {
  extern char start[]; /* the linker's _start; _etext (KallistiOS's arch.h) is the end of the code */
  const uintptr_t lo = (uintptr_t)start, hi = (uintptr_t)&_etext;
  if (sp < 0x8c000000u || sp > 0x8d000000u - 4 * 128 || (sp & 3))
    return;
  char line[96];
  int n = 0, len = 0;
  line[0] = 0;
  x_n = 0;
  for (int i = 0; i < 128 && n < 16; i++) {
    const uint32_t v = ((const uint32_t *)sp)[i];
    if (v >= lo && v < hi && !(v & 1)) {
      x_stack[x_n++] = v;
      len += snprintf(line + len, sizeof(line) - len, "%s%08lx", len ? " " : "", (unsigned long)v);
      if (++n % 8 == 0) {
        sw_trace("stack %s", line);
        len = 0;
        line[0] = 0;
      }
    }
  }
  if (len)
    sw_trace("stack %s", line);
}

/* Every build: a menu that stops making progress for 12 s is stopped on the report screen, so a hang shows
   where it happened instead of just freezing. Progress is a trace line or a picture drawn (sw_trace_alive,
   called from every frame and from every wait loop that draws frames); a known long wait (a launch, the BIOS)
   says so with sw_watchdog_expect. The longest legitimate wait measured in Flycast is a 3.5 s double save
   on a slow card and a 10 s wait for a disc that never becomes ready, which is inside a launch's expect
   window. A hang with interrupts off cannot be caught here; that is only the hand over to a game. */
#define WATCHDOG_MS 12000
static void *watchdog(void *arg) {
  (void)arg;
  for (;;) {
    thd_sleep(500);
    const uint64_t now = timer_ms_gettime64();
    if (!last_alive || now < expect_until || now - last_alive <= WATCHDOG_MS)
      continue;
    char why[96];
    snprintf(why, sizeof(why), "no progress for %d s after: %s", WATCHDOG_MS / 1000,
             num_lines ? lines[num_lines - 1] + 6 : "?");
    if (main_thd) {
      /* the menu thread's saved place: exact when it waits, its last time slice end when it spins */
      sw_trace("hang: menu thread state %d%s%s pc=%08lx pr=%08lx", (int)main_thd->state,
               main_thd->wait_msg ? " waiting on " : "", main_thd->wait_msg ? main_thd->wait_msg : "",
               (unsigned long)main_thd->context.pc, (unsigned long)main_thd->context.pr);
      x_pc = main_thd->context.pc;
      x_pr = main_thd->context.pr;
      trace_stack(main_thd->context.r[15]);
    }
    sw_trace_fatal_reason(SW_REPORT_HANG, why);
  }
  return NULL;
}

void sw_trace(const char *fmt, ...) {
  char buf[128];
  va_list ap;
  va_start(ap, fmt);
  vsnprintf(buf, sizeof(buf), fmt, ap);
  va_end(ap);
#ifdef SW_MEM_TRACE
  {
    extern struct mallinfo mallinfo(void);
    struct mallinfo mi = mallinfo();
    printf("SWIRL trace %5u ms: %s [free %u of %u]\n", (unsigned)timer_ms_gettime64(), buf, (unsigned)mi.fordblks,
           (unsigned)mi.arena);
  }
#else
  printf("SWIRL trace %5u ms: %s\n", (unsigned)timer_ms_gettime64(), buf);
#endif
  if (num_lines == MAX_LINES) {
    memmove(lines[1], lines[2], sizeof(lines[0]) * (MAX_LINES - 2)); /* keep the first line */
    num_lines--;
  }
  snprintf(lines[num_lines++], LINE_LEN, "%5u %s", (unsigned)timer_ms_gettime64(), buf);
  last_alive = timer_ms_gettime64();
  if (screen_on)
    draw_all(0xFFFFFFFF);
}

const char *sw_trace_text(void) {
  text[0] = 0;
  for (int i = 0; i < num_lines; i++) {
    strcat(text, lines[i]);
    strcat(text, "\n");
  }
  return text;
}

/* ---------- warnings ----------
   A problem that does not stop the menu (a save that failed, a picture skipped, an INI line ignored) used to
   be a trace line at best. Each is now a code from sw_codes.h with a count and the first detail, shown in
   System > Diagnostics and in every report's w= line, so "my settings do not save" comes with a code. */
#define MAX_WARN 16
typedef struct sw_warning {
  uint8_t code;
  uint16_t count;
  char detail[64];
} sw_warning;
static sw_warning warns[MAX_WARN];
static int num_warns;

static const struct {
  int code;
  const char *key, *words;
} code_table[] = {
#define X(name, num, key, words) {num, key, words},
    SW_CODES(X)
#undef X
};

const char *sw_code_words(int code) {
  for (unsigned i = 0; i < sizeof(code_table) / sizeof(code_table[0]); i++)
    if (code_table[i].code == code) return code_table[i].words;
  return "Unknown warning";
}

void sw_warn(int code, const char *fmt, ...) {
  char buf[96];
  va_list ap;
  va_start(ap, fmt);
  vsnprintf(buf, sizeof(buf), fmt, ap);
  va_end(ap);
  sw_trace("W%02d %s", code, buf);
  for (int i = 0; i < num_warns; i++)
    if (warns[i].code == code) {
      if (warns[i].count < 65535) warns[i].count++;
      return;
    }
  if (num_warns < MAX_WARN) {
    warns[num_warns].code = (uint8_t)code;
    warns[num_warns].count = 1;
    snprintf(warns[num_warns].detail, sizeof(warns[num_warns].detail), "%s", buf);
    num_warns++;
  }
}

int sw_warn_count(void) { return num_warns; }

int sw_warn_get(int i, int *code, int *count, const char **detail) {
  if (i < 0 || i >= num_warns)
    return 0;
  *code = warns[i].code;
  *count = warns[i].count;
  *detail = warns[i].detail;
  return 1;
}

static int num_games;
void sw_trace_games(int n) { num_games = n; }
const char *sw_build_id(void) { return SWIRL_BUILD; }

static int x_pressed(void) {
  maple_device_t *dev;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_CONTROLLER)); i++) {
    cont_state_t *st = (cont_state_t *)maple_dev_status(dev);
    if (st && (st->buttons & CONT_X))
      return 1;
  }
  return 0;
}

static int a_pressed(void) {
  maple_device_t *dev;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_CONTROLLER)); i++) {
    cont_state_t *st = (cont_state_t *)maple_dev_status(dev);
    if (st && (st->buttons & CONT_A))
      return 1;
  }
  return 0;
}

int sw_trace_x_held(void) { return x_pressed(); }

void sw_trace_done(void) {
  sw_trace("start up done");
  screen_on = 0;
}

#include "../../external/qrcodegen/qrcodegen.h"

#ifdef SWIRL_TRACE_SCREEN

/* shows one prepared 640 x 480 page (in buf) until A is let go and pressed */
static int x_pressed(void);
/* returns 1 if the page was left with X instead of A */
static int show_page(pvr_ptr_t tex, uint16_t *buf, int tw, int th) {
  pvr_txr_load(buf, tex, tw * th * 2);
  pvr_poly_cxt_t cxt;
  pvr_poly_hdr_t hdr;
  pvr_poly_cxt_txr(&cxt, PVR_LIST_OP_POLY, PVR_TXRFMT_RGB565 | PVR_TXRFMT_NONTWIDDLED, tw, th, tex, PVR_FILTER_NONE);
  pvr_poly_compile(&hdr, &cxt);
  int state = 0;
  for (int t = 0; t < 60 * 600 && state < 2; t++) {
    pvr_wait_ready();
    pvr_scene_begin();
    pvr_list_begin(PVR_LIST_OP_POLY);
    pvr_prim(&hdr, sizeof(hdr));
    pvr_vertex_t v;
    const float u1 = 640.f / tw, v1 = 480.f / th;
    const float xs[4] = {0, 0, 640, 640}, ys[4] = {480, 0, 480, 0};
    const float us[4] = {0, 0, u1, u1}, vs[4] = {v1, 0, v1, 0};
    for (int k = 0; k < 4; k++) {
      v.flags = k == 3 ? PVR_CMD_VERTEX_EOL : PVR_CMD_VERTEX;
      v.x = xs[k];
      v.y = ys[k];
      v.z = 1.f;
      v.u = us[k];
      v.v = vs[k];
      v.argb = 0xFFFFFFFF;
      v.oargb = 0;
      pvr_prim(&v, sizeof(v));
    }
    pvr_list_finish();
    pvr_scene_finish();
    sw_trace_alive();
    const int a = a_pressed() || x_pressed();
    if (state == 0 && !a) state = 1;
    else if (state == 1 && a) state = 2;
  }
  const int was_x = x_pressed();
  for (int t = 0; t < 300 && (a_pressed() || x_pressed()); t++) thd_sleep(10);
  return was_x;
}

/* The start up report as ordinary pictures from the graphics chip. First as QR codes (a phone camera reads
   them, or send Claude a photo), each holding part of the text, then as text, 18 steps a page. */
void sw_trace_report(void) {
  const int tw = 1024, th = 512;
  pvr_ptr_t tex = pvr_mem_malloc(tw * th * 2);
  static uint16_t buf[1024 * 512];
  if (!tex)
    return;

  /* the whole log as one text, cut into parts of up to 600 bytes at line ends */
  static char all[MAX_LINES * LINE_LEN + 64];
  all[0] = 0;
  for (int i = 0; i < num_lines; i++) {
    const char *l = lines[i];
    while (*l == ' ') l++;
    strcat(all, l);
    strcat(all, "\n");
  }
  enum { PART = 400, MAX_PARTS = 12 };
  int starts[MAX_PARTS + 1], parts = 0, len = (int)strlen(all), at = 0;
  while (at < len && parts < MAX_PARTS) {
    int end = at + PART < len ? at + PART : len;
    if (end < len)
      while (end > at + 1 && all[end - 1] != '\n') end--;
    starts[parts++] = at;
    at = end;
  }
  starts[parts] = at;

  static uint8_t qr[qrcodegen_BUFFER_LEN_FOR_VERSION(25)], tmp[qrcodegen_BUFFER_LEN_FOR_VERSION(25)];
  for (int p = 0; p < parts; p++) {
    char text[PART + 32];
    const int n = starts[p + 1] - starts[p];
    snprintf(text, sizeof(text), "SWIRL log %d/%d\n", p + 1, parts);
    strncat(text, all + starts[p], n);
    for (int i = 0; i < tw * th; i++) buf[i] = 0xFFFF; /* white, the QR code needs a light border */
    if (qrcodegen_encodeText(text, tmp, qr, qrcodegen_Ecc_MEDIUM, 1, 25, qrcodegen_Mask_AUTO, true)) {
      const int size = qrcodegen_getSize(qr);
      int scale = 420 / (size + 8);
      if (scale < 1) scale = 1;
      const int x0 = (640 - size * scale) / 2, y0 = (436 - size * scale) / 2;
      for (int y = 0; y < size; y++)
        for (int x = 0; x < size; x++)
          if (qrcodegen_getModule(qr, x, y))
            for (int yy = 0; yy < scale; yy++)
              for (int xx = 0; xx < scale; xx++) buf[(y0 + y * scale + yy) * tw + x0 + x * scale + xx] = 0;
    }
    char foot[80];
    snprintf(foot, sizeof(foot), "QR %d of %d. Photo or scan it, then press A.", p + 1, parts);
    bfont_draw_str_ex(buf + 444 * tw + 40, tw, 0x0000, 0xFFFF, 16, true, foot);
    show_page(tex, buf, tw, th);
  }

  /* then as text */
  const int per_page = 18, pages = (num_lines + per_page - 1) / per_page;
  for (int page = 0; page < pages; page++) {
    memset(buf, 0, tw * th * 2);
    for (int r = 0; r < per_page && page * per_page + r < num_lines; r++) {
      char cut[SCREEN_CHARS + 1];
      snprintf(cut, sizeof(cut), "%s", lines[page * per_page + r]);
      bfont_draw_str_ex(buf + (8 + r * BFONT_HEIGHT) * tw + 8, tw, 0xFFFF, 0, 16, true, cut);
    }
    char foot[64];
    snprintf(foot, sizeof(foot), "Page %d of %d. Press A.", page + 1, pages);
    bfont_draw_str_ex(buf + (8 + 19 * BFONT_HEIGHT) * tw + 8, tw, 0xFFE0, 0, 16, true, foot);
    show_page(tex, buf, tw, th);
  }

  /* A/B test of the automatic save */
  extern int sw_autosave_like_manual;
  for (;;) {
    memset(buf, 0, tw * th * 2);
    const char *l[] = {"Automatic save test", "",
                       sw_autosave_like_manual ? "Now: NEW  (writes both files, like Save)" : "Now: OLD  (writes SWIRL.DAT only)",
                       "", "X: switch   A: start SWIRL", NULL};
    for (int r = 0; l[r]; r++)
      bfont_draw_str_ex(buf + (60 + r * BFONT_HEIGHT) * tw + 40, tw, r == 2 ? 0xFFE0 : 0xFFFF, 0, 16, true, l[r]);
    if (!show_page(tex, buf, tw, th))
      break;
    sw_autosave_like_manual = !sw_autosave_like_manual;
  }
  sw_trace("automatic save: %s", sw_autosave_like_manual ? "new" : "old");
  pvr_mem_free(tex);
}
#endif

/* ---------- the report screen, in every build ----------
   It must work however SWIRL stopped: in the middle of handing over to a game the graphics chip, the controllers
   and the timer may already be shut down. So it writes straight into the picture being shown (no graphics chip),
   and when nothing can be pressed it times its pages by counting. The report is cut into parts, each a QR code
   that a phone camera reads (or send a photo of each); the codes take turns, a few seconds each. Asked for from
   the menu (Diagnostics, or X held at power on) the same screen is left with B, and A turns the page. */
static uint8_t *fb_base(void) {
  return (uint8_t *)PVR_RAM_BASE + (PVR_GET(PVR_FB_ADDR) & (PVR_RAM_SIZE - 1));
}

/* a run of one colour on one line of the picture, in whatever pixel mode is set (0xRRGGBB) */
static void fb_run(int x, int y, int w, uint32_t rgb) {
  if (!vid_mode || x < 0 || y < 0 || x + w > 640 || y >= 480)
    return;
  uint8_t *base = fb_base();
  switch (vid_mode->pm) {
    case PM_RGB565: {
      uint16_t *d = (uint16_t *)base + y * 640 + x;
      const uint16_t c = (uint16_t)(((rgb >> 8) & 0xF800) | ((rgb >> 5) & 0x07E0) | ((rgb >> 3) & 0x001F));
      for (int i = 0; i < w; i++) d[i] = c;
      break;
    }
    case PM_RGB888P: {
      /* three bytes a pixel, but video memory takes no byte writes (they are lost, on the console and in
         Flycast): the run is written as whole 32 bit words, the two at the ends read first */
      const uint32_t first = (uint32_t)(y * 640 + x) * 3, last = first + (uint32_t)w * 3; /* [first, last) */
      volatile uint32_t *word = (volatile uint32_t *)(base + (first & ~3u));
      const uint8_t b[3] = {(uint8_t)rgb, (uint8_t)(rgb >> 8), (uint8_t)(rgb >> 16)};
      for (uint32_t at = first & ~3u; at < last; at += 4, word++) {
        uint32_t v = (at < first || at + 4 > last) ? *word : 0;
        for (int k = 0; k < 4; k++) {
          const uint32_t pos = at + k;
          if (pos < first || pos >= last) continue;
          v = (v & ~(0xFFu << (8 * k))) | ((uint32_t)b[(pos - first) % 3] << (8 * k));
        }
        *word = v;
      }
      break;
    }
    case PM_RGB0888: {
      uint32_t *d = (uint32_t *)base + y * 640 + x;
      for (int i = 0; i < w; i++) d[i] = rgb;
      break;
    }
    default:
      break;
  }
}

static void fb_fill(uint32_t rgb) {
  for (int y = 0; y < 480; y++) fb_run(0, y, 640, rgb);
}

static void fb_text(int y, const char *s, uint32_t fg, uint32_t bg) {
  static uint32_t row[640 * (BFONT_HEIGHT + 1)]; /* a spare row: bfont may touch one past the end */
  for (int i = 0; i < 640 * BFONT_HEIGHT; i++) row[i] = bg;
  char cut[SCREEN_CHARS + 1];
  snprintf(cut, sizeof(cut), "%s", s);
  bfont_draw_str_ex(row + 8, 640, fg, bg, 32, true, cut);
  for (int yy = 0; yy < BFONT_HEIGHT; yy++) {
    const uint32_t *src = row + yy * 640;
    int x = 0;
    while (x < 640) {
      int e = x + 1;
      while (e < 640 && src[e] == src[x]) e++;
      fb_run(x, y + yy, e - x, src[x] & 0xFFFFFF);
      x = e;
    }
  }
}

/* waits by counting TV pictures: the video chip's own signal, read from its register, works even when the timer
   and interrupts are off (50 or 60 a second) */
static void wait_frames(int n) {
  for (int i = 0; i < n; i++)
    vid_waitvbl();
}

/* the report text (docs/DIAGNOSTICS.md): a header, then the trace */
static int report_reason;
static char all[MAX_LINES * LINE_LEN + 512];

static const char *reason_word(int reason) {
  switch (reason) {
    case SW_REPORT_CRASH: return "crash";
    case SW_REPORT_HANG: return "hang";
    case SW_REPORT_USER: return "Diagnostics";
    case SW_REPORT_BOOT: return "boot log";
    case SW_REPORT_ASSERT: return "assert";
    case SW_REPORT_ABORT: return "abort";
    case SW_REPORT_LAUNCH: return "launch";
    default: return "?";
  }
}

static void build_report(int reason, int live) {
  int len = 0;
  const int stopped = reason != SW_REPORT_USER && reason != SW_REPORT_BOOT;
  len += snprintf(all + len, sizeof(all) - len, "SWIRL %s %s R%d\n", SWIRL_VERSION_STR, SWIRL_BUILD, reason);
  {
    char bios[6];
    memcpy(bios, (const char *)0x8c0007CC, 5);
    bios[5] = 0;
    for (int i = 0; i < 5; i++)
      if (bios[i] < ' ' || bios[i] > '~') bios[i] = '?';
    unsigned free_kb = 0, arena_kb = 0;
    if (live && !stopped) {
      /* the allocator's lists are walked here: only while the menu is known to be sound */
      struct mallinfo mi = mallinfo();
      free_kb = (unsigned)mi.fordblks / 1024;
      arena_kb = (unsigned)mi.arena / 1024;
    } else {
      extern void *sbrk(intptr_t);
      extern char end[]; /* the linker's end of the program's data */
      arena_kb = (unsigned)((uintptr_t)sbrk(0) - (uintptr_t)end) / 1024;
    }
    len += snprintf(all + len, sizeof(all) - len, "b=%s r=%d c=%d v=%dx%d/%d m=%u/%u u=%u n=%d\n", bios,
                    flashrom_get_region(), vid_check_cable(), vid_mode ? vid_mode->width : 0,
                    vid_mode ? vid_mode->height : 0, vid_mode ? vid_mode->pm : 0, free_kb, arena_kb,
                    (unsigned)(timer_ms_gettime64() / 1000), num_games);
  }
  len += snprintf(all + len, sizeof(all) - len, "d=");
  {
    int n = 0;
    for (int p = 0; p < MAPLE_PORT_COUNT; p++)
      for (int u = 0; u < MAPLE_UNIT_COUNT; u++) {
        maple_device_t *d = maple_enum_dev(p, u);
        if (!d || !d->valid) continue;
        char name[21];
        snprintf(name, sizeof(name), "%.20s", d->info.product_name);
        for (int i = (int)strlen(name) - 1; i >= 0 && name[i] == ' '; i--) name[i] = 0;
        len += snprintf(all + len, sizeof(all) - len, "%s%c%d:%s", n++ ? "," : "", 'A' + p, u, name);
      }
  }
  len += snprintf(all + len, sizeof(all) - len, "\nw=");
  for (int i = 0; i < num_warns; i++)
    len += snprintf(all + len, sizeof(all) - len, "%sW%02dx%u", i ? "," : "", warns[i].code, warns[i].count);
  if (x_n || x_pc) {
    len += snprintf(all + len, sizeof(all) - len, "\nx=%08lx %08lx", (unsigned long)x_pc, (unsigned long)x_pr);
    for (int i = 0; i < x_n; i++) len += snprintf(all + len, sizeof(all) - len, " %08lx", (unsigned long)x_stack[i]);
  }
  len += snprintf(all + len, sizeof(all) - len, "\nt=\n");
  /* the first line (the version) and the latest 39 */
  const int keep = 40;
  for (int i = 0; i < num_lines; i++) {
    if (num_lines > keep && i > 0 && i < num_lines - (keep - 1))
      continue;
    const char *l = lines[i];
    while (*l == ' ') l++;
    const int room = (int)sizeof(all) - len - 2;
    if (room <= 0) break;
    len += snprintf(all + len, sizeof(all) - len, "%.*s\n", room, l);
  }
}

static int a_pressed(void);
static int b_pressed(void) {
  maple_device_t *dev;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_CONTROLLER)); i++) {
    cont_state_t *st = (cont_state_t *)maple_dev_status(dev);
    if (st && (st->buttons & CONT_B))
      return 1;
  }
  return 0;
}

/* shows the report; with can_leave the pages turn on A (or by themselves) and B returns */
static void show_report(int reason, int can_leave) {
  enum { PART = 500, MAX_PARTS = 12 };
  static int starts[MAX_PARTS + 1];
  int parts = 0, len = (int)strlen(all), at = 0;
  /* the end of the log matters most: if it is too long, keep the header and the latest steps */
  while (len - at > PART * MAX_PARTS) {
    char *nl = strchr(all + at, '\n');
    if (!nl) break;
    at = (int)(nl - all) + 1;
  }
  while (at < len && parts < MAX_PARTS) {
    int end = at + PART < len ? at + PART : len;
    if (end < len)
      while (end > at + 1 && all[end - 1] != '\n') end--;
    starts[parts++] = at;
    at = end;
  }
  starts[parts] = at;
  static uint8_t qr[qrcodegen_BUFFER_LEN_FOR_VERSION(25)], tmp[qrcodegen_BUFFER_LEN_FOR_VERSION(25)];
  static char text[PART + 48];
  const char *first_warn = num_warns ? warns[0].detail : NULL;
  for (int p = 0;; p = (p + 1) % parts) {
    const int n = starts[p + 1] - starts[p];
    snprintf(text, sizeof(text), "SWIRL %s %d/%d\n", SWIRL_REPORT_VERSION, p + 1, parts);
    strncat(text, all + starts[p], n);
    /* From the menu the graphics chip may still finish the last frame and switch pictures after this page was
       drawn; the page is drawn again into whichever picture is shown until it stays. */
    for (int tries = 0; tries < 4; tries++) {
    const uint32_t drawn = PVR_GET(PVR_FB_ADDR);
    fb_fill(0xFFFFFF); /* white: a QR code needs a light border */
    if (qrcodegen_encodeText(text, tmp, qr, qrcodegen_Ecc_MEDIUM, 1, 25, qrcodegen_Mask_AUTO, true)) {
      const int size = qrcodegen_getSize(qr);
      int scale = 380 / (size + 8);
      if (scale < 1) scale = 1;
      const int x0 = (640 - size * scale) / 2, y0 = 44 + (380 - size * scale) / 2;
      for (int y = 0; y < size; y++)
        for (int yy = 0; yy < scale; yy++) {
          int x = 0;
          while (x < size) {
            if (!qrcodegen_getModule(qr, x, y)) {
              x++;
              continue;
            }
            int e = x + 1;
            while (e < size && qrcodegen_getModule(qr, e, y)) e++;
            fb_run(x0 + x * scale, y0 + y * scale + yy, (e - x) * scale, 0x000000);
            x = e;
          }
        }
    }
    char line[80];
    /* the first line as plain text too: a photo without a scan still says something */
    snprintf(line, sizeof(line), "SWIRL %s %s: %s%s%s", SWIRL_REPORT_VERSION, SWIRL_BUILD, reason_word(reason),
             first_warn ? ". " : "", first_warn ? first_warn : "");
    fb_text(4, line, can_leave ? 0x000000 : 0xFF0000, 0xFFFFFF);
    snprintf(line, sizeof(line), "%s. Photo or scan each code: %d of %d", can_leave ? "Report" : "SWIRL stopped", p + 1, parts);
    fb_text(4 + BFONT_HEIGHT, line, 0x000000, 0xFFFFFF);
    fb_text(480 - BFONT_HEIGHT - 4,
            can_leave ? "A: next code   B: back to the menu" : "Then switch off. Hold Y at power on for SWIRL.", 0x000000,
            0xFFFFFF);
    if (!can_leave)
      break;
    vid_waitvbl();
    vid_waitvbl();
    if (PVR_GET(PVR_FB_ADDR) == drawn)
      break;
    }
    if (!can_leave) {
      wait_frames(parts > 1 ? 6 * 60 : 60 * 60);
      continue;
    }
    /* wait for the buttons to be let go, then for A (next), B (leave) or 8 s */
    for (int t = 0; t < 300 && (a_pressed() || b_pressed()); t++) thd_sleep(10);
    for (int t = 0; t < 800; t++) {
      sw_trace_alive();
      if (b_pressed()) {
        for (int k = 0; k < 300 && b_pressed(); k++) thd_sleep(10);
        return;
      }
      if (a_pressed()) break;
      thd_sleep(10);
    }
  }
}

void sw_report_show(int reason) {
  sw_trace("report shown: %s", reason_word(reason));
  build_report(reason, 1);
  /* the frame the menu submitted last is still being drawn and shown: let that finish first */
  pvr_wait_ready();
  wait_frames(10);
  show_report(reason, 1);
}

void sw_trace_fatal(const char *why) {
  printf("SWIRL: %s\n", why);
  /* Every build stops on a report screen: returning to the BIOS would start SWIRL again and fail the same way,
     over and over, with nothing to show what happened. */
  static int inside;
  if (!inside) {
    inside = 1;
    irq_disable(); /* nothing else runs from here on */
    sw_trace("STOPPED: %s", why);
    if (!report_reason) report_reason = SW_REPORT_CRASH;
    vid_set_mode(DM_640x480, PM_RGB565);
    build_report(report_reason, 0);
    show_report(report_reason, 0);
  }
  for (;;) {
  }
}

void sw_trace_fatal_reason(int reason, const char *why) {
  report_reason = reason;
  sw_trace_fatal(why);
}

/* KallistiOS's ways out: an assert, a panic (crash) or abort. The linker's --wrap sends them here. */
void __wrap_arch_abort(void) __attribute__((noreturn));
void __wrap_arch_abort(void) { sw_trace_fatal_reason(SW_REPORT_ABORT, "abort"); }

void __wrap_arch_panic(const char *msg) __attribute__((noreturn));
void __wrap_arch_panic(const char *msg) {
  irq_context_t *c = irq_get_context();
  char buf[96];
  if (c)
    snprintf(buf, sizeof(buf), "crash: %s pc=%08lx pr=%08lx", msg, (unsigned long)c->pc, (unsigned long)c->pr);
  else
    snprintf(buf, sizeof(buf), "crash: %s", msg);
  sw_trace_fatal(buf);
}

static void on_assert(const char *file, int line, const char *expr, const char *msg, const char *func) {
  (void)func;
  const char *f = strrchr(file, '/');
  sw_trace("assert %s:%d %s", f ? f + 1 : file, line, msg ? msg : expr);
  sw_trace_fatal_reason(SW_REPORT_ASSERT, "assert");
}

/* a crash (CPU exception): note where it happened; KallistiOS then panics, which ends in sw_trace_fatal */
static void on_exception(irq_t code, irq_context_t *ctx, void *data) {
  (void)data;
  sw_trace("crash %03lx pc=%08lx pr=%08lx", (unsigned long)code, ctx ? (unsigned long)ctx->pc : 0UL,
           ctx ? (unsigned long)ctx->pr : 0UL);
  if (!ctx)
    return;
  x_pc = ctx->pc;
  x_pr = ctx->pr;
  trace_stack(ctx->r[15]);
}

/* walks the memory allocator's lists: damage shows up here (a crash in mallinfo) instead of later, somewhere
   unrelated. The trace says when the check ran. */
#include <malloc.h>
void sw_mem_check(const char *when) {
  struct mallinfo mi = mallinfo();
  if (when)
    sw_trace("memory ok %s (%u KB free of %u)", when, (unsigned)mi.fordblks / 1024, (unsigned)mi.arena / 1024);
}

void sw_trace_init(void) {
  assert_set_handler(on_assert);
  irq_set_handler(EXC_UNHANDLED_EXC, on_exception, NULL);
  main_thd = thd_get_current();
  thd_create(1, watchdog, NULL);
  if (screen_on)
    vid_clear(0, 0, 0);
}

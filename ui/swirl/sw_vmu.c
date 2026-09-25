/*
 * SWIRL: VMU screen output (48x32, 1 bit) plus an on-screen preview texture.
 */
#include "sw_vmu.h"

#include <dc/maple.h>
#include <dc/maple/vmu.h>
#include <dc/pvr.h>
#include <kos/fs.h>
#include <string.h>

#include "vmu_font.h"

#define W 48
#define H 32

static uint8_t pix[H][W];           /* 1 = dark pixel, as seen on the VMU */
static char last_key[4][24];
static int pending;                  /* frames until push */
static image preview;
static uint16_t preview_buf[64 * 32] __attribute__((aligned(32)));
static int preview_dirty;

void sw_vmu_init(void) {
  memset(pix, 0, sizeof(pix));
  preview.width = 64;
  preview.height = 32;
  preview.format = PVR_TXRFMT_ARGB4444 | PVR_TXRFMT_NONTWIDDLED;
  if (!preview.texture)
    preview.texture = pvr_mem_malloc(64 * 32 * 2);
  preview_dirty = 1;
  memset(last_key, 0, sizeof(last_key));
}

static int text_width(const char *s) {
  int w = 0;
  for (; *s; s++) {
    unsigned char c = (unsigned char)*s;
    if (c < 32 || c > 126) c = '?';
    w += vmu_font_adv[c - 32];
  }
  return w;
}

static void draw_text_line(int y, const char *s) {
  if (!s || !*s)
    return;
  /* centre; clip to the screen */
  int w = text_width(s);
  int x = (W - w) / 2;
  if (x < 0) x = 0;
  for (; *s && x < W; s++) {
    unsigned char c = (unsigned char)*s;
    if (c >= 'a' && c <= 'z') c = c - 'a' + 'A'; /* the pixel font reads best in capitals */
    if (c < 32 || c > 126) c = '?';
    const uint8_t *rows = vmu_font_rows[c - 32];
    for (int r = 0; r < 5; r++)
      for (int b = 0; b < 8; b++)
        if ((rows[r] & (0x80 >> b)) && x + b < W && y + r < H)
          pix[y + r][x + b] = 1;
    x += vmu_font_adv[c - 32];
  }
}

void sw_vmu_text(const char *l1, const char *l2, const char *l3, const char *l4) {
  const char *lines[4] = {l1, l2, l3, l4};
  int same = 1;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    if (strncmp(last_key[i], s, sizeof(last_key[i]) - 1)) same = 0;
  }
  if (same)
    return;
  int n = 0;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    strncpy(last_key[i], s, sizeof(last_key[i]) - 1);
    last_key[i][sizeof(last_key[i]) - 1] = 0;
    if (*s) n++;
  }
  memset(pix, 0, sizeof(pix));
  /* frame */
  for (int x = 0; x < W; x++) { pix[0][x] = 1; pix[H - 1][x] = 1; }
  for (int y = 0; y < H; y++) { pix[y][0] = 1; pix[y][W - 1] = 1; }
  int total = n * 5 + (n - 1) * 2;
  int y = (H - total) / 2;
  for (int i = 0; i < 4; i++) {
    const char *s = lines[i] ? lines[i] : "";
    if (!*s) continue;
    draw_text_line(y, s);
    y += 7;
  }
  pending = 8; /* small debounce so fast scrolling does not flood the maple bus */
  preview_dirty = 1;
}

void sw_vmu_bitmap(const uint8_t *bits, const char *key) {
  if (!strncmp(last_key[0], key, sizeof(last_key[0]) - 1) && !strcmp(last_key[1], "\x01img"))
    return;
  strncpy(last_key[0], key, sizeof(last_key[0]) - 1);
  last_key[0][sizeof(last_key[0]) - 1] = 0;
  strcpy(last_key[1], "\x01img");
  last_key[2][0] = last_key[3][0] = 0;
  for (int y = 0; y < H; y++)
    for (int x = 0; x < W; x++)
      pix[y][x] = (bits[y * (W / 8) + x / 8] & (0x80 >> (x % 8))) ? 1 : 0;
  pending = 8;
  preview_dirty = 1;
}

static void push(void) {
  uint8_t bitmap[W * H / 8];
  memset(bitmap, 0, sizeof(bitmap));
  /* same orientation mapping as KOS vmu_draw_lcd_xbm */
  for (int Y = 0; Y < H; Y++)
    for (int X = 0; X < W; X++)
      if (pix[Y][X]) {
        int x = (W - 1) - X, y = (H - 1) - Y;
        bitmap[y * (W / 8) + x / 8] |= 0x80 >> (x % 8);
      }
  maple_device_t *dev;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_LCD)); i++)
    vmu_draw_lcd(dev, bitmap);
}

void sw_vmu_tick(void) {
  if (pending > 0 && --pending == 0)
    push();
  if (preview_dirty && preview.texture) {
    for (int y = 0; y < 32; y++)
      for (int x = 0; x < 64; x++)
        preview_buf[y * 64 + x] = (x < W && pix[y][x]) ? 0xF000 : 0x0000; /* alpha = ink */
    pvr_txr_load(preview_buf, preview.texture, sizeof(preview_buf));
    preview_dirty = 0;
  }
}

const image *sw_vmu_preview(void) {
  return &preview;
}

/* ---------- SWIRL logo ----------
   Shown at power on and on the System screens. SWIRL Card Manager can replace it with LOGO.VMU
   (192 bytes, same layout as VMU.DAT pictures) on the menu disc. */
static const uint8_t default_logo[192] = {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0xe0, 0x00, 0x00, 0x00, 0x00, 0x01, 0xc3, 0xe0, 0x00, 0x00, 0x00, 0x01, 0x8f, 0xf8, 0x00, 0x00, 0x00, 0x03, 0x1c, 0x1e, 0x00, 0x00, 0x00, 0x03, 0x38, 0x07, 0x00, 0x00, 0x00, 0x06, 0x73, 0xe3, 0x00, 0x00, 0x00, 0x06, 0x67, 0xf1, 0x80, 0x00, 0x00, 0x06, 0x66, 0x39, 0x80, 0x00, 0x00, 0x06, 0x6c, 0x19, 0x80, 0x00, 0x00, 0x06, 0x6e, 0xdd, 0x80, 0x00, 0x00, 0x06, 0x67, 0xdd, 0x80, 0x00, 0x00, 0x06, 0x63, 0x99, 0x80, 0x00, 0x00, 0x06, 0x70, 0x19, 0x80, 0x00, 0x00, 0x03, 0x38, 0x71, 0x80, 0x00, 0x00, 0x03, 0x9f, 0xe3, 0x00, 0x00, 0x00, 0x01, 0x87, 0xc7, 0x00, 0x00, 0x00, 0x00, 0xe0, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x78, 0x3c, 0x00, 0x00, 0x00, 0x00, 0x3f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x07, 0xc0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0xa2, 0xb2, 0x00, 0x00, 0x00, 0x02, 0x22, 0xaa, 0x00, 0x00, 0x00, 0x03, 0xaa, 0xb2, 0x00, 0x00, 0x00, 0x00, 0xaa, 0xaa, 0x00, 0x00, 0x00, 0x03, 0x94, 0xab, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00};
static uint8_t logo[192];
static int logo_loaded;

static void load_logo(void) {
  if (logo_loaded)
    return;
  logo_loaded = 1;
  memcpy(logo, default_logo, sizeof(logo));
  file_t f = fs_open("/cd/LOGO.VMU", O_RDONLY);
  if (f != FILEHND_INVALID) {
    uint8_t buf[192];
    if (fs_read(f, buf, sizeof(buf)) == (ssize_t)sizeof(buf))
      memcpy(logo, buf, sizeof(logo));
    fs_close(f);
  }
}

void sw_vmu_show_logo(void) {
  load_logo();
  sw_vmu_bitmap(logo, "\x02logo");
}

/* power on: nothing else is running yet, so draw straight away */
void sw_vmu_boot_logo(void) {
  load_logo();
  for (int y = 0; y < H; y++)
    for (int x = 0; x < W; x++)
      pix[y][x] = (logo[y * (W / 8) + x / 8] & (0x80 >> (x % 8))) ? 1 : 0;
  push();
}

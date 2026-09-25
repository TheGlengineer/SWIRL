/*
 * SWIRL picture quality: with horizontal anti-aliasing on, the PowerVR renders a 1280 x 480 image and
 * scales it down to 640 x 480. Every UI in the menu draws in 640 x 480 coordinates, so all vertices
 * pass through here (the Makefile maps pvr_prim to om_prim) and have their X coordinates doubled.
 */
#include <dc/pvr.h>
#include <stdint.h>
#include <string.h>

#undef pvr_prim
int pvr_prim(const void *data, size_t size);

float om_xscale = 1.f;

/* what the last header started: 0 polygons, 1 sprites; and where we are inside a 64 byte sprite */
static int in_sprite, sprite_half;

int om_prim(const void *data, size_t size) {
  if (om_xscale == 1.f)
    return pvr_prim(data, size);
  const uint8_t *p = data;
  uint32_t buf[8] __attribute__((aligned(32)));
  for (size_t off = 0; off + 32 <= size; off += 32) {
    memcpy(buf, p + off, 32);
    uint32_t cmd = buf[0] & 0xe0000000;
    if (!in_sprite || sprite_half == 0) {
      if (cmd == 0x80000000 || cmd == 0xa0000000 || cmd == 0x20000000) {
        /* polygon or sprite header (0x20000000 = user clip): remember the type */
        if (cmd == 0x80000000 || cmd == 0xa0000000)
          in_sprite = (cmd == 0xa0000000);
        sprite_half = 0;
        pvr_prim(buf, 32);
        continue;
      }
    }
    float *f = (float *)buf;
    if (!in_sprite) {
      f[1] *= om_xscale; /* x */
    } else if (sprite_half == 0) {
      f[1] *= om_xscale; /* ax */
      f[4] *= om_xscale; /* bx */
      f[7] *= om_xscale; /* cx */
      sprite_half = 1;
    } else {
      f[2] *= om_xscale; /* dx */
      sprite_half = 0;
    }
    pvr_prim(buf, 32);
  }
  return 0;
}

/* KOS sets the pixel clip to the video width; with anti-aliasing the render is twice as wide */
#include <../hardware/pvr/pvr_internal.h>

void sw_fix_fsaa_clip(void) {
  pvr_state.pclip_right = pvr_state.w * 2 - 1;
  pvr_state.pclip_x = (pvr_state.pclip_right << 16) | pvr_state.pclip_left;
}

/* SWIRL launch options and alternative launchers (see gdemu_control.c) */
#pragma once

struct gd_item;

enum { LAUNCH_REGION_AUTO = 0, LAUNCH_REGION_JAPAN, LAUNCH_REGION_USA, LAUNCH_REGION_EUROPE };
enum { LAUNCH_BOOT_NONE = 0, LAUNCH_BOOT_ANIMATION, LAUNCH_BOOT_LICENSE, LAUNCH_BOOT_BOTH };

typedef struct launch_opts {
  int region; /* LAUNCH_REGION_* */
  int vga;    /* 1 forces VGA output */
  int boot;   /* LAUNCH_BOOT_* */
} launch_opts;

extern void (*gdemu_before_launch)(void);

void dreamcast_launch_disc_ex(struct gd_item *disc, const launch_opts *o);
void dreamcast_launch_cb(struct gd_item *disc);
void bleem_launch(struct gd_item *disc);
int codebreaker_available(void);
int bleem_available(void);
int is_psx_disc(const struct gd_item *disc);

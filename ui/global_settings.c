/*
 * File: global_settings.c
 * Project: ui
 * File Created: Monday, 12th July 2021 10:33:21 am
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#include "global_settings.h"
#include "swirl/sw_trace.h"

#include <arch/timer.h>

#include <dc/maple.h>
#include <stdio.h>
#include <string.h>
#include <strings.h>
#include <kos/thread.h>
#include <dc/maple/controller.h>
#include <external/libcrayonvmu/savefile.h>
#include <external/libcrayonvmu/setup.h>

#include "theme_manager.h"
#include "dc/pvr_texture.h"

/* Images and such */
#include "swirl/sw_lib.h"
#include "swirl/sw_vmu.h"
/* SWIRL draws its own VMU screen, so openmenu_lcd.h (the LCD picture) is not included */
#if __has_include("openmenu_pal.h") && __has_include("openmenu_vmu.h")
#include "openmenu_pal.h"
#include "openmenu_vmu.h"

#define OPENMENU_ICON (openmenu_icon)
#define OPENMENU_PAL (openmenu_pal)
#define OPENMENU_ICONS (1)
#else
#define OPENMENU_ICON (NULL)
#define OPENMENU_PAL (NULL)
#define OPENMENU_ICONS (0)
#endif

static crayon_savefile_details_t savefile_details;
static openmenu_settings savedata;

static void settings_defaults(void) {
  savedata.identifier[0] = 'O';
  savedata.identifier[1] = 'M';
  savedata.version = 1; /* SWIRL: always openMenu's version, so openMenu and the Virtual Folder Bundle read it */
  savedata.padding = 0;
  savedata.ui = UI_SWIRL;
  savedata.region = REGION_NTSC_U;
  savedata.aspect = ASPECT_NORMAL;
  savedata.sort = SORT_DEFAULT;
  savedata.filter = FILTER_ALL;
  savedata.beep = BEEP_ON;
  savedata.multidisc = MULTIDISC_SHOW;
  savedata.custom_theme = THEME_OFF;
  savedata.custom_theme_num = THEME_0;
}

/* SWIRL: picks the card a new OPENMENU.CFG goes to (the left most with room) */
static void settings_pick_card(void) {
  if (savefile_details.valid_memcards) {
    for (int iter = 0; iter <= 3; iter++) {
      for (int jiter = 1; jiter <= 2; jiter++) {
        if (crayon_savefile_get_vmu_bit(savefile_details.valid_memcards, iter, jiter)) {  // Use the left most VMU
          savefile_details.savefile_port = iter;
          savefile_details.savefile_slot = jiter;
          goto Exit_loop_2;
        }
      }
    }
  }
Exit_loop_2:;
}

/* SWIRL: no OPENMENU.CFG on any card at start up. openMenu wrote a fresh one at once; SWIRL waits for the
   first save instead, so a card that turns up late with the real file (see settings_late_card) is not
   shadowed by a new one on another card. */
static int cfg_missing;

static void settings_create(void) {
  settings_defaults();
  cfg_missing = 1;
}

void settings_init(void) {
  /* Mostly from CrayonVMU example */
  crayon_savefile_init_savefile_details(&savefile_details,
                                        (uint8_t *)&savedata, sizeof(openmenu_settings), OPENMENU_ICONS, 1,
                                        "openMenu Preferences\0", "openMenu Config\0", "openMenuPref\0", "OPENMENU.CFG\0");

  savefile_details.icon = OPENMENU_ICON;
  savefile_details.icon_palette = (unsigned short *)OPENMENU_PAL;

  /* SWIRL: its own logo (or the owner's LOGO.VMU) instead of the openMenu one */
  sw_trace("VMU logo");
  sw_vmu_boot_logo();
  sw_trace("OPENMENU.CFG: finding it");

  // Find the first savefile (if it exists)
  for (int iter = 0; iter <= 3; iter++) {
    for (int jiter = 1; jiter <= 2; jiter++) {
      if (crayon_savefile_get_vmu_bit(savefile_details.valid_saves, iter, jiter)) {  // Use the left most VMU
        savefile_details.savefile_port = iter;
        savefile_details.savefile_slot = jiter;
        goto Exit_loop_1;
      }
    }
  }
Exit_loop_1:

  settings_load();
  sw_trace("OPENMENU.CFG: VMU %d/%d, cards %02x saves %02x, style %d", savefile_details.savefile_port,
           savefile_details.savefile_slot, savefile_details.valid_memcards, savefile_details.valid_saves, savedata.ui);
  settings_validate();
  settings_swirl_once();
}

/* SWIRL: the first time SWIRL runs with a card (no SWIRL.DAT yet, or one from before this flag) the style becomes
   SWIRL once and SWIRL.DAT remembers that. From then on the style in OPENMENU.CFG stands, whichever menu set it,
   so a Classic style the owner chose, or openMenu run in between, is kept. */
static int cfg_dirty; /* the style changed here; the menu saves the file once it is up */

int settings_take_dirty(void) {
  const int r = cfg_dirty;
  cfg_dirty = 0;
  return r;
}

void settings_swirl_once(void) {
  if (sw_lib_prefs()->swirl_style_set)
    return;
  sw_lib_note_style_set();
  if (savedata.ui != UI_SWIRL) {
    sw_trace("style %d in OPENMENU.CFG: SWIRL takes over once", savedata.ui);
    savedata.ui = UI_SWIRL;
    cfg_dirty = 1;
  }
}

/* SWIRL: OPENMENU.CFG keeps openMenu's version 1, whatever menu wrote it last. Upstream resets any other version
   to defaults, so a version 2 file (SWIRL 2.14 previews wrote one) made the two menus reset each other on every
   switch. A preview's version 2 file has the same layout and is read as version 1. The one thing SWIRL wants
   from the file, its own style the first time it runs on a card, is remembered in SWIRL.DAT instead
   (settings_swirl_once). */
void settings_validate(void) {
  if (savedata.version == 2)
    savedata.version = 1;
  if (savedata.version != 1) {
    settings_defaults();
    settings_save();
    return;
  }

  if ((savedata.ui < UI_START) || (savedata.ui > UI_END)) {
    savedata.ui = UI_SWIRL;
  }

  if ((savedata.region < REGION_START) || (savedata.region > REGION_END)) {
    savedata.region = REGION_NTSC_U;
  }

  if ((savedata.aspect < ASPECT_START) || (savedata.aspect > ASPECT_END)) {
    savedata.aspect = ASPECT_NORMAL;
  }

  if ((savedata.sort < SORT_START) || (savedata.sort > SORT_END)) {
    savedata.sort = SORT_DEFAULT;
  }

  if ((savedata.filter < FILTER_START) || (savedata.filter > FILTER_END)) {
    savedata.filter = FILTER_ALL;
  }

  if ((savedata.beep < BEEP_START) || (savedata.beep > BEEP_END)) {
    savedata.beep = BEEP_ON;
  }
  if ((savedata.multidisc < MULTIDISC_START) || (savedata.multidisc > MULTIDISC_END)) {
    savedata.multidisc = MULTIDISC_SHOW;
  }

  if ((savedata.custom_theme < THEME_START) || (savedata.custom_theme > THEME_END)) {
    savedata.custom_theme = THEME_OFF; /* SWIRL: used to reset custom_theme_num instead */
  }

  if ((savedata.custom_theme_num < THEME_NUM_START) || (savedata.custom_theme_num > THEME_NUM_END)) {
    savedata.custom_theme_num = THEME_NUM_START;
  }

  if (savedata.custom_theme) {
    savedata.region = REGION_END + 1 + savedata.custom_theme_num;
  }
}

void settings_load(void) {
  // Try and load savefile
  crayon_savefile_load(&savefile_details);

  // No savefile yet
  if (savefile_details.valid_memcards && savefile_details.savefile_port == -1 && savefile_details.savefile_slot == -1) {
    settings_create();
  }
}

/* Beeps while saving if enabled */
void settings_save(void) {
  maple_device_t *vmu = NULL;
  int on = 0;
  if (savefile_details.savefile_port < 0) {
    /* the first save since start up found no file: choose the card now */
    crayon_savefile_update_valid_saves(&savefile_details, CRAY_SAVEFILE_UPDATE_MODE_BOTH);
    settings_pick_card();
    cfg_missing = 0;
  }
  if ((savedata.beep == BEEP_ON) && (vmu = maple_enum_dev(savefile_details.savefile_port, savefile_details.savefile_slot))) {
    on = vmu_beep_raw(vmu, 0x000065f0); /* Turn on Beep */
  }
  int rv = -1;
  if (savefile_details.valid_memcards) {
    rv = crayon_savefile_save(&savefile_details);
    crayon_savefile_update_valid_saves(&savefile_details, CRAY_SAVEFILE_UPDATE_MODE_BOTH);
  }
  /* SWIRL: always turn the beep off again, and keep trying if the memory card is busy: a beep left on can leave
     the VMU in a state the console's own start up may not like */
  int off = 0;
  if (vmu) {
    for (int i = 0; i < 10 && (off = vmu_beep_raw(vmu, 0x00000000)) != 0; i++) thd_sleep(20);
  }
  sw_trace("OPENMENU.CFG saved: %d (beep on %d, off %d)", rv, on, off);
}

openmenu_settings *settings_get(void) {
  return &savedata;
}

/* ---------- SWIRL: a memory card that turned up after the start up scan ---------- */
/* KallistiOS's scan of the controller ports gives up after 1.5 s (main.c), so a VM2 or VMU Pro still switching
   cards, or a slow VMU, is not there when the settings are read: the menu starts with defaults and never looks
   again. For a while after the menu is up the ports are watched, and a card that attaches then is read as if it
   had been there at start up. */
static int cards_seen = -1;

static int count_cards(void) {
  int n = 0;
  for (int i = 0; maple_enum_type(i, MAPLE_FUNC_MEMCARD); i++) n++;
#ifdef SW_TEST_LATE_VMU
  if (timer_ms_gettime64() < 12000) n = 0; /* test only: the cards attach 12 s after power on */
#endif
  return n;
}

int settings_cfg_missing(void) { return cfg_missing; }

/* 1 when a card that attached late carried the file, and the settings (maybe the style) changed */
int settings_late_card(void) {
  const int n = count_cards();
  if (cards_seen < 0) {
    cards_seen = n;
    return 0;
  }
  if (n <= cards_seen)
    return 0;
  cards_seen = n;
  if (savefile_details.savefile_port >= 0)
    return 0; /* the file was read at start up: that card's settings stand */
  crayon_savefile_update_valid_saves(&savefile_details, CRAY_SAVEFILE_UPDATE_MODE_BOTH);
  int port = -1, slot = -1;
  for (int iter = 0; iter <= 3 && port < 0; iter++)
    for (int jiter = 1; jiter <= 2; jiter++)
      if (crayon_savefile_get_vmu_bit(savefile_details.valid_saves, iter, jiter)) {
        port = iter;
        slot = jiter;
        break;
      }
  if (port < 0) {
    sw_trace("OPENMENU.CFG: memory card attached late, no file on it");
    return 0;
  }
  const openmenu_settings before = savedata;
  savefile_details.savefile_port = port;
  savefile_details.savefile_slot = slot;
  crayon_savefile_load(&savefile_details);
  cfg_missing = 0;
  sw_trace("OPENMENU.CFG: read from %c%d, attached late (style %d)", 'A' + port, slot, savedata.ui);
  settings_validate();
  settings_swirl_once();
  if (savedata.ui != before.ui && !settings_style_ready(savedata.ui))
    savedata.ui = before.ui; /* the saved style needs theme files this disc lacks: stay */
  return memcmp(&before, &savedata, sizeof(savedata)) != 0;
}
/* ---------- SWIRL: styles that can actually run on this menu disc ---------- */

static int disc_has(const char *rel) {
  char path[96];
  snprintf(path, sizeof(path), "/cd/%s", rel);
  file_t f = fs_open(path, O_RDONLY);
  if (f == FILEHND_INVALID)
    return 0;
  /* a picture must also have a usable header and be long enough for its size, or the style would start on a
     missing picture (a damaged theme file used to end on the crash report screen) */
  size_t len = strlen(rel);
  if (len > 4 && strcasecmp(rel + len - 4, ".PVR") == 0) {
    unsigned char hdr[32];
    ssize_t total = fs_total(f);
    ssize_t got = fs_read(f, hdr, sizeof(hdr));
    fs_close(f);
    if (total <= 0 || got != (ssize_t)sizeof(hdr) || !pvr_header_usable(hdr, sizeof(hdr), (size_t)total)) {
      printf("SWIRL: %s is not a usable picture\n", rel);
      return 0;
    }
    return 1;
  }
  fs_close(f);
  return 1;
}

static int all_on_disc(const char *const *files) {
  for (; *files; files++)
    if (!disc_has(*files)) {
      printf("SWIRL: %s is not on the menu disc\n", *files);
      return 0;
    }
  return 1;
}

/* the background pictures the Classic list and grid use for the chosen region or custom theme */
static int theme_pictures_on_disc(void) {
  int n = 0;
  if (savedata.custom_theme) {
    theme_custom *ct = theme_get_custom(&n);
    int i = savedata.custom_theme_num;
    return ct && i >= 0 && i < n && disc_has(ct[i].bg_left) && disc_has(ct[i].bg_right);
  }
  theme_region *rt = theme_get_default(savedata.aspect, &n);
  int r = savedata.region;
  return rt && r >= 0 && r < n && disc_has(rt[r].bg_left) && disc_has(rt[r].bg_right);
}

static int style_ready_uncached(int ui);

/* The answer is kept: the menu disc cannot change while SWIRL runs, and this is asked every frame while the
   System tab shows the Style setting. It is worked out again only if the region or theme settings change. */
int settings_style_ready(int ui) {
  static int known[UI_END + 1], ready[UI_END + 1];
  static int key[UI_END + 1];
  if (ui < UI_START || ui > UI_END)
    return 0;
  const int k = 1 + savedata.region + 16 * savedata.aspect + 256 * savedata.custom_theme + 4096 * savedata.custom_theme_num;
  if (!known[ui] || key[ui] != k) {
    ready[ui] = style_ready_uncached(ui);
    key[ui] = k;
    known[ui] = 1;
  }
  return ready[ui];
}

static int style_ready_uncached(int ui) {
  static const char *const list_files[] = {"EMPTY.PVR", "THEME/SHARED/HIGHLIGHT.PVR", "THEME/SHARED/ICON_WHITE.PVR",
                                           "THEME/SHARED/ICON_BLACK.PVR", "FONT/BASILEA.FNT", "FONT/BASILEA_W.PVR", NULL};
  static const char *const grid_files[] = {"EMPTY.PVR", "THEME/SHARED/HIGHLIGHT.PVR", "FONT/BASILEA.FNT", "FONT/BASILEA_W.PVR",
                                           NULL};
  static const char *const gdmenu_files[] = {"THEME/GDMENU/BG_L.PVR", "THEME/GDMENU/BG_R.PVR", "FONT/GDMNUFNT.PVR", NULL};
  switch (ui) {
    case UI_SWIRL: return 1;
    case UI_GDMENU: return all_on_disc(gdmenu_files);
    case UI_GRID3: return all_on_disc(grid_files) && theme_pictures_on_disc();
    default: return all_on_disc(list_files) && theme_pictures_on_disc();
  }
}

static int y_held(void) {
  maple_device_t *dev;
  for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_CONTROLLER)); i++) {
    cont_state_t *st = (cont_state_t *)maple_dev_status(dev);
    if (st && (st->buttons & CONT_Y))
      return 1;
  }
  return 0;
}

static int boot_y;
static int boot_reset; /* Y switched a saved Classic style back to SWIRL; still to be saved */

int settings_boot_y(void) { return boot_y; }

/* Y held for a moment just after the menu appeared (the start up check can miss it): same as holding it at
   start up */
void settings_force_swirl(void) {
  if (savedata.ui == UI_SWIRL)
    return;
  sw_trace("Y held after start: switching to the SWIRL style");
  savedata.ui = UI_SWIRL;
  boot_y = 1;
  boot_reset = 1;
}

int settings_boot_reset_take(void) {
  const int r = boot_reset;
  boot_reset = 0;
  return r;
}

void settings_boot_guard(void) {
  /* read Y a few times over a tenth of a second: one reading can come before the controller has reported (and
     holding Y a moment longer once the menu shows is caught too, see main.c) */
  boot_y = 0;
  char seen[64] = "";
  for (int i = 0; i < 5 && !(boot_y = y_held()); i++) {
    maple_device_t *c = maple_enum_type(0, MAPLE_FUNC_CONTROLLER);
    cont_state_t *st = c ? (cont_state_t *)maple_dev_status(c) : NULL;
    char one[8];
    snprintf(one, sizeof(one), "%s%lx", i ? " " : "", st ? (unsigned long)st->buttons : 0xfffffUL);
    strncat(seen, one, sizeof(seen) - strlen(seen) - 1);
    thd_sleep(20);
  }
  sw_trace("Y samples: %s", seen[0] ? seen : "(first one held)");
  {
    maple_device_t *dev;
    for (int i = 0; (dev = maple_enum_type(i, MAPLE_FUNC_CONTROLLER)); i++) {
      cont_state_t *st = (cont_state_t *)maple_dev_status(dev);
      sw_trace("pad %c%d buttons %04lx", 'A' + dev->port, dev->unit, st ? (unsigned long)st->buttons : 0xFFFFUL);
    }
    sw_trace("Y held: %s, saved style %d", boot_y ? "yes" : "no", savedata.ui);
  }
  if (savedata.ui == UI_SWIRL)
    return;
  /* only the running style changes here; the memory card is written once the menu is up */
  if (boot_y) {
    printf("SWIRL: Y held at start, using the SWIRL style\n");
    savedata.ui = UI_SWIRL;
    boot_reset = 1; /* the owner asked for SWIRL: keep it (a style that only can't run now is not saved over) */
  } else if (!settings_style_ready(savedata.ui)) {
    printf("SWIRL: the saved style needs openMenu's theme files, using the SWIRL style\n");
    savedata.ui = UI_SWIRL;
  }
}

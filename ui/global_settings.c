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

#include <dc/maple.h>
#include <dc/maple/controller.h>
#include <external/libcrayonvmu/savefile.h>
#include <external/libcrayonvmu/setup.h>

#include "theme_manager.h"

/* Images and such */
#include "swirl/sw_vmu.h"
#if __has_include("openmenu_lcd.h") && __has_include("openmenu_pal.h") && __has_include("openmenu_vmu.h")
#include "openmenu_lcd.h"
#include "openmenu_pal.h"
#include "openmenu_vmu.h"

#define OPENMENU_ICON (openmenu_icon)
#define OPENMENU_LCD (openmenu_lcd)
#define OPENMENU_PAL (openmenu_pal)
#define OPENMENU_ICONS (1)
#else
#define OPENMENU_ICON (NULL)
#define OPENMENU_LCD (NULL)
#define OPENMENU_PAL (NULL)
#define OPENMENU_ICONS (0)
#endif

static crayon_savefile_details_t savefile_details;
static openmenu_settings savedata;

static void settings_defaults(void) {
  savedata.identifier[0] = 'O';
  savedata.identifier[1] = 'M';
  savedata.version = 2;
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

static void settings_create(void) {
  // If we don't already have a savefile, choose a VMU
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

  settings_defaults();

  settings_save();
}

void settings_init(void) {
  /* Mostly from CrayonVMU example */
  crayon_savefile_init_savefile_details(&savefile_details,
                                        (uint8_t *)&savedata, sizeof(openmenu_settings), OPENMENU_ICONS, 1,
                                        "openMenu Preferences\0", "openMenu Config\0", "openMenuPref\0", "OPENMENU.CFG\0");

  savefile_details.icon = OPENMENU_ICON;
  savefile_details.icon_palette = (unsigned short *)OPENMENU_PAL;

  /* SWIRL: its own logo (or the owner's LOGO.VMU) instead of the openMenu one */
  sw_vmu_boot_logo();

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
  settings_validate();
}

void settings_validate(void) {
  if (savedata.version == 1) {
    /* SWIRL: keep the user's openMenu settings, switch the style to SWIRL once */
    savedata.version = 2;
    savedata.ui = UI_SWIRL;
    settings_save();
  } else if (savedata.version != 2) {
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
    savedata.custom_theme_num = THEME_OFF;
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
  if ((savedata.beep == BEEP_ON) && (vmu = maple_enum_dev(savefile_details.savefile_port, savefile_details.savefile_slot))) {
    vmu_beep_raw(vmu, 0x000065f0); /* Turn on Beep */
  }
  if (savefile_details.valid_memcards) {
    crayon_savefile_save(&savefile_details);
    crayon_savefile_update_valid_saves(&savefile_details, CRAY_SAVEFILE_UPDATE_MODE_BOTH);
    if ((savedata.beep == BEEP_ON) && (vmu)) {
      vmu_beep_raw(vmu, 0x00000000); /* Turn off Beep */
    }
  }
}

openmenu_settings *settings_get(void) {
  return &savedata;
}
/* ---------- SWIRL: styles that can actually run on this menu disc ---------- */

static int disc_has(const char *rel) {
  char path[96];
  snprintf(path, sizeof(path), "/cd/%s", rel);
  file_t f = fs_open(path, O_RDONLY);
  if (f == FILEHND_INVALID)
    return 0;
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

int settings_boot_y(void) { return boot_y; }

void settings_boot_guard(void) {
  boot_y = y_held();
  if (savedata.ui == UI_SWIRL)
    return;
  /* only the running style changes; the memory card is written at the next save as usual */
  if (boot_y) {
    printf("SWIRL: Y held at start, using the SWIRL style\n");
    savedata.ui = UI_SWIRL;
  } else if (!settings_style_ready(savedata.ui)) {
    printf("SWIRL: the saved style needs openMenu's theme files, using the SWIRL style\n");
    savedata.ui = UI_SWIRL;
  }
}

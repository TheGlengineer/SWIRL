/*
 * File: theme_manager.c
 * Project: ui
 * File Created: Tuesday, 27th July 2021 12:09:21 pm
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#include "theme_manager.h"

#include <stdlib.h>
#include <string.h>
#include <strings.h>

#include "../external/ini.h"
#include "../gdrom/gdrom_fs.h"
#include "draw_prototypes.h"
#include "swirl/sw_trace.h"

/* SWIRL: the custom theme table is fixed at 10 (the settings store a single digit); a THEME.INI is small */
#define MAX_CUSTOM_THEMES (10)
#define THEME_INI_MAX_SIZE (64 * 1024)

/* Missing on sh-elf-gcc 9.1 ? */
char *strdup(const char *s);
char *strsep(char **stringp, const char *delim);

static theme_region region_themes[] = {
    {.bg_left = "THEME/NTSC_U/BG_U_L.PVR",
     .bg_right = "THEME/NTSC_U/BG_U_R.PVR",
     .colors = {.icon_color = COLOR_WHITE,
                .text_color = COLOR_WHITE,
                .highlight_color = COLOR_ORANGE_U,
                .menu_text_color = COLOR_WHITE,
                .menu_highlight_color = COLOR_ORANGE_U,
                .menu_bkg_color = COLOR_BLACK,
                .menu_bkg_border_color = COLOR_WHITE}},
    {.bg_left = "THEME/NTSC_J/BG_J_L.PVR",
     .bg_right = "THEME/NTSC_J/BG_J_R.PVR",
     .colors = {.icon_color = COLOR_BLACK,
                .text_color = COLOR_BLACK,
                .highlight_color = COLOR_ORANGE_J,
                .menu_text_color = COLOR_BLACK,
                .menu_highlight_color = COLOR_ORANGE_J,
                .menu_bkg_color = COLOR_WHITE,
                .menu_bkg_border_color = COLOR_BLACK}},
    {.bg_left = "THEME/PAL/BG_E_L.PVR",
     .bg_right = "THEME/PAL/BG_E_R.PVR",
     .colors = {.icon_color = COLOR_BLACK,
                .text_color = COLOR_BLACK,
                .highlight_color = COLOR_BLUE,
                .menu_text_color = COLOR_BLACK,
                .menu_highlight_color = COLOR_BLUE,
                .menu_bkg_color = COLOR_WHITE,
                .menu_bkg_border_color = COLOR_BLACK}},
};

static theme_custom custom_themes[MAX_CUSTOM_THEMES];
static int num_custom_themes = 0;

static void select_art_by_aspect(CFG_ASPECT aspect) {
  if (aspect == ASPECT_NORMAL) {
    region_themes[REGION_NTSC_U].bg_left = "THEME/NTSC_U/BG_U_L.PVR";
    region_themes[REGION_NTSC_U].bg_right = "THEME/NTSC_U/BG_U_R.PVR";

    region_themes[REGION_NTSC_J].bg_left = "THEME/NTSC_J/BG_J_L.PVR";
    region_themes[REGION_NTSC_J].bg_right = "THEME/NTSC_J/BG_J_R.PVR";

    region_themes[REGION_PAL].bg_left = "THEME/PAL/BG_E_L.PVR";
    region_themes[REGION_PAL].bg_right = "THEME/PAL/BG_E_R.PVR";
  } else {
    region_themes[REGION_NTSC_U].bg_left = "THEME/NTSC_U/BG_U_L.PVR";
    region_themes[REGION_NTSC_U].bg_right = "THEME/NTSC_U/BG_U_R.PVR";

    region_themes[REGION_NTSC_J].bg_left = "THEME/NTSC_J/BG_J_L_WIDE.PVR";
    region_themes[REGION_NTSC_J].bg_right = "THEME/NTSC_J/BG_J_R_WIDE.PVR";

    region_themes[REGION_PAL].bg_left = "THEME/PAL/BG_E_L_WIDE.PVR";
    region_themes[REGION_PAL].bg_right = "THEME/PAL/BG_E_R_WIDE.PVR";
  }
}

static inline long int filelength(FD_TYPE f) {
  long int end;
  fseek(f, 0, SEEK_END);
  end = ftell(f);
  fseek(f, 0, SEEK_SET);

  return end;
}

static uint32_t str2argb(const char *str) {
  char *token, *temp, *tofree, *endptr;
  int rgb[3] = {0, 0, 0};
  int n = 0;

  tofree = temp = strdup(str);
  if (!tofree)
    return PVR_PACK_ARGB(0xFF, 0, 0, 0);
  /* SWIRL: at most three values; a fourth used to write past rgb[] */
  while (n < 3 && (token = strsep(&temp, ","))) {
    long v = strtol(token, &endptr, 0);
    rgb[n++] = (int)(v < 0 ? 0 : v > 255 ? 255 : v);
  }
  free(tofree);
  return PVR_PACK_ARGB(0xFF, rgb[0], rgb[1], rgb[2]);
}

static int read_theme_ini(void *user, const char *section, const char *name, const char *value) {
  /* unused */
  (void)user;
  if (strcmp(section, "THEME") == 0) {
    theme_custom *new_theme = (theme_custom *)user;
    theme_color *new_color = &new_theme->colors;
    if (strcasecmp(name, "NAME") == 0) {
      snprintf(new_theme->name, sizeof(new_theme->name), "%s", value);
    } else if (strcasecmp(name, "ICON_COLOR") == 0) {
      new_color->icon_color = str2argb(value);
    } else if (strcasecmp(name, "TEXT_COLOR") == 0) {
      new_color->text_color = str2argb(value);
    } else if (strcasecmp(name, "HIGHLIGHT_COLOR") == 0) {
      new_color->highlight_color = str2argb(value);
    } else if (strcasecmp(name, "MENU_TEXT_COLOR") == 0) {
      new_color->menu_text_color = str2argb(value);
    } else if (strcasecmp(name, "MENU_HIGHLIGHT_COLOR") == 0) {
      new_color->menu_highlight_color = str2argb(value);
    } else if (strcasecmp(name, "MENU_BKG_COLOR") == 0) {
      new_color->menu_bkg_color = str2argb(value);
    } else if (strcasecmp(name, "MENU_BKG_BORDER_COLOR") == 0) {
      new_color->menu_bkg_border_color = str2argb(value);
    } else {
      printf("Unknown theme value: %s\n", name);
    }
  } else {
    /* error */
    printf("INI:Error unknown [%s] %s: %s\n", section, name, value);
  }
  return 1;
}

static int theme_read(const char *filename, theme_custom *theme) {
  FD_TYPE ini = fopen(filename, "rb");
  if (!FD_IS_OK(ini)) {
    printf("INI:Error opening %s!\n", filename);
    fflush(stdout);
    /*exit or something */
    return -1;
  }

  /* SWIRL: the size, the allocation and the read are checked, and the buffer gets the NUL inih needs */
  long ini_size = filelength(ini);
  if (ini_size < 0 || ini_size > THEME_INI_MAX_SIZE) {
    sw_warn(SW_WARN_THEME, "theme: %s is %ld bytes, not read", filename, ini_size);
    fclose(ini);
    return -1;
  }
  char *ini_buffer = malloc((size_t)ini_size + 1);
  if (!ini_buffer) {
    fclose(ini);
    return -1;
  }
  size_t got = fread(ini_buffer, 1, (size_t)ini_size, ini);
  fclose(ini);
  ini_buffer[got] = '\0';

  int ret = ini_parse_string(ini_buffer, read_theme_ini, (void *)theme);
  free(ini_buffer);
  if (ret != 0) {
    /* a bad line is skipped (inih carries on); the name and colours read so far are kept */
    sw_warn(SW_WARN_THEME, "theme: %s line %d not understood", filename, ret);
    return ret < 0 ? -1 : 0;
  }

  return 0;
}

/* SWIRL: the folder is opened by its full path (KOS's working directory is /, so the relative "THEME" never
   opened on hardware and custom themes were never listed). The picture paths stay relative to the disc, as the
   texture loader adds the prefix; the INI path is absolute. Every string is bounded by its field and the table
   stops at MAX_CUSTOM_THEMES. */
static void load_themes(const char *basePath) {
  char path[128];
  DIRENT_TYPE dp;
  snprintf(path, sizeof(path), "%s%s", DISC_PREFIX, basePath);
  DIR_TYPE dir = opendir(path);

  if (!dir) {
    sw_trace("theme: %s not found, no custom themes", path);
    return;
  }

  while ((dp = readdir(dir)) != NULL) {
    if (strcmp(dp->d_name, ".") != 0 && strcmp(dp->d_name, "..") != 0) {
      if (strncasecmp(dp->d_name, "CUST_", 5) == 0) {
        if (num_custom_themes >= MAX_CUSTOM_THEMES) {
          sw_warn(SW_WARN_THEME, "theme: more than %d custom themes, %s not listed", MAX_CUSTOM_THEMES, dp->d_name);
          continue;
        }
        theme_custom *theme = &custom_themes[num_custom_themes];
        int theme_num = dp->d_name[5] - '0';
        if (theme_num < 0 || theme_num > 9)
          theme_num = num_custom_themes;

        /* the picture paths must fit their fields (a long folder name used to overflow bg_left) */
        int n = snprintf(theme->bg_left, sizeof(theme->bg_left), "%s/%s/BG_L.PVR", basePath, dp->d_name);
        if (n < 0 || n >= (int)sizeof(theme->bg_left)) {
          sw_warn(SW_WARN_THEME, "theme: folder name %.24s too long, not listed", dp->d_name);
          theme->bg_left[0] = '\0';
          continue;
        }
        snprintf(theme->bg_right, sizeof(theme->bg_right), "%s/%s/BG_R.PVR", basePath, dp->d_name);

        /* dummy colors */
        theme->colors = (theme_color){.text_color = COLOR_WHITE,
                                      .highlight_color = COLOR_ORANGE_U,
                                      .menu_text_color = COLOR_WHITE,
                                      .menu_bkg_color = COLOR_BLACK,
                                      .menu_bkg_border_color = COLOR_WHITE};

        /* dummy name */
        snprintf(theme->name, sizeof(theme->name), "CUSTOM #%d", theme_num);

        /* load INI if available, for name & colors */
        snprintf(path, sizeof(path), "%s%s/%s/THEME.INI", DISC_PREFIX, basePath, dp->d_name);
        theme_read(path, theme);
        printf("theme #%d: %s @ %s (%s)\n", theme_num, dp->d_name, theme->bg_left, theme->name);

        num_custom_themes++;
      }
    }
  }

  closedir(dir);
}

int theme_manager_load(void) {
  /* Original themes are statically loaded */

  /* Load custom themes if they exist */
  load_themes("THEME");
  return 0;
}

theme_region *theme_get_default(CFG_ASPECT aspect, int *num_themes) {
  select_art_by_aspect(aspect);
  *num_themes = 3;
  return region_themes;
}
theme_custom *theme_get_custom(int *num_themes) {
  *num_themes = num_custom_themes;
  return custom_themes;
}

/*
 * File: common.h
 * Project: ui
 * File Created: Thursday, 20th May 2021 12:53:34 am
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#pragma once

#include <string.h>
#include <sys/stat.h>

#include "../external/easing.h"

enum control { NONE = 0,
               LEFT,
               RIGHT,
               UP,
               DOWN,
               A,
               B,
               X,
               Y,
               START,
               TRIG_L,
               TRIG_R };

static inline int file_exists(const char *path) {
  struct stat buffer;
  return (stat(path, &buffer) == 0);
}

/* Joins left and right with spaces between them so the result is at least width characters, for the Classic
   styles' info lines. The result always fits cap bytes, terminator included: a right string that is too long
   is cut, never written past the buffer. From sergiosaint's pull request #10, with the bound added. */
static inline void ui_pad_concat(char *out, size_t cap, const char *left, const char *right, size_t width) {
  if (cap == 0) return;
  size_t pos = 0;
  for (; *left && pos + 1 < cap; left++) out[pos++] = *left;
  const size_t right_len = strlen(right);
  size_t pad = pos + right_len < width ? width - pos - right_len : 0;
  for (; pad && pos + 1 < cap; pad--) out[pos++] = ' ';
  for (; *right && pos + 1 < cap; right++) out[pos++] = *right;
  out[pos] = 0;
}

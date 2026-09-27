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

static inline void ui_string_pad_concat(char *out, const char *left, const char *right, size_t width) {
  const size_t left_len = strlen(left);
  const size_t right_len = strlen(right);
  const size_t input_len = left_len + right_len;
  const size_t padding = input_len < width ? width - input_len : 0;

  memcpy(out, left, left_len);
  memset(out + left_len, ' ', padding);
  memcpy(out + left_len + padding, right, right_len + 1);
}

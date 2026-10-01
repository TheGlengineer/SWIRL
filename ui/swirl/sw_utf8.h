/* SWIRL: UTF-8 decoding for the text renderer. Strings in the menu (OPENMENU.INI names, the language
   files, the built in English) are UTF-8; the fonts hold the Latin-1 range (U+0020..U+00FF), so every
   western European language renders and anything outside that range draws as '?'. Header only so the
   host check (tests/host/utf8_check.c) builds it without the PVR. */
#pragma once
#include <stdint.h>

/* Decodes one UTF-8 sequence from s[*i] (stopping at NUL, or at byte n when n >= 0) and advances *i past
   it. A malformed or truncated sequence yields '?' and advances one byte, so bad input never stalls or
   runs past the end. */
static inline uint32_t sw_utf8_next(const char *s, int *i, int n) {
  const unsigned char c = (unsigned char)s[*i];
  (*i)++;
  if (c < 0x80)
    return c;
  int len;
  uint32_t cp;
  if ((c & 0xE0) == 0xC0) {
    len = 1;
    cp = c & 0x1F;
  } else if ((c & 0xF0) == 0xE0) {
    len = 2;
    cp = c & 0x0F;
  } else if ((c & 0xF8) == 0xF0) {
    len = 3;
    cp = c & 0x07;
  } else {
    return '?';
  }
  for (int k = 0; k < len; k++) {
    const int at = *i;
    if ((n >= 0 && at >= n) || ((unsigned char)s[at] & 0xC0) != 0x80)
      return '?';
    cp = (cp << 6) | ((unsigned char)s[at] & 0x3F);
    (*i)++;
  }
  return cp;
}

/* The largest byte count <= n at which s can be cut without splitting a UTF-8 sequence. */
static inline int sw_utf8_boundary(const char *s, int n) {
  while (n > 0 && ((unsigned char)s[n] & 0xC0) == 0x80)
    n--;
  return n;
}

/* The plain letter behind a Latin-1 accented one (É -> E, ñ -> n, ç -> c), for the VMU's 5 pixel font and
   anything else that only has ASCII. ASCII passes through; ß becomes s; anything else is '?'. */
static inline char sw_utf8_ascii(uint32_t cp) {
  static const char upper[] = "AAAAAAACEEEEIIIIDNOOOOO*OUUUUYTs"; /* U+00C0..U+00DF */
  static const char lower[] = "aaaaaaaceeeeiiiidnooooo/ouuuuyty"; /* U+00E0..U+00FF */
  if (cp < 0x80)
    return (char)cp;
  if (cp >= 0xC0 && cp <= 0xDF)
    return upper[cp - 0xC0];
  if (cp >= 0xE0 && cp <= 0xFF)
    return lower[cp - 0xE0];
  return '?';
}

/* Copies s into dst (size bytes, always terminated) with every character reduced by sw_utf8_ascii. */
static inline void sw_utf8_fold_ascii(char *dst, int size, const char *s) {
  int i = 0, o = 0;
  while (s[i] && o < size - 1)
    dst[o++] = sw_utf8_ascii(sw_utf8_next(s, &i, -1));
  dst[o] = 0;
}

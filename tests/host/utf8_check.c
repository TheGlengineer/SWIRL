/* SWIRL host check for the UTF-8 decoder behind the text renderer (ui/swirl/sw_utf8.h): code points,
   malformed input, the byte limit and the cut boundary used by sw_text_clip. Built with the sanitizers
   by run.sh. */
#include <stdio.h>
#include <string.h>

#include "ui/swirl/sw_utf8.h"

static int fail;

static void decode(const char *what, const char *s, int n, const uint32_t *want, int count) {
  int i = 0, k = 0;
  while (s[i] && (n < 0 || i < n)) {
    uint32_t cp = sw_utf8_next(s, &i, n);
    if (k >= count || cp != want[k]) {
      printf("FAIL %s: code point %d is U+%04X, want %s\n", what, k, (unsigned)cp,
             k < count ? "another value" : "the end");
      fail = 1;
      return;
    }
    k++;
  }
  if (k != count) {
    printf("FAIL %s: %d code points, want %d\n", what, k, count);
    fail = 1;
  }
  if (n >= 0 && i > n) {
    printf("FAIL %s: read past the byte limit (%d > %d)\n", what, i, n);
    fail = 1;
  }
}

static void boundary(const char *s, int n, int want) {
  int got = sw_utf8_boundary(s, n);
  if (got != want) {
    printf("FAIL boundary(%d) = %d, want %d\n", n, got, want);
    fail = 1;
  }
}

int main(void) {
  /* ASCII passes through */
  decode("ascii", "Ab ~", -1, (const uint32_t[]){'A', 'b', ' ', '~'}, 4);
  /* Latin-1: two byte sequences */
  decode("latin1", "\xc3\xa9\xc3\x9c\xc3\xb1\xc2\xa1", -1, (const uint32_t[]){0xE9, 0xDC, 0xF1, 0xA1}, 4);
  /* beyond Latin-1 decodes to its code point (the renderer draws it as '?') */
  decode("three byte", "\xe2\x82\xac" "x", -1, (const uint32_t[]){0x20AC, 'x'}, 2);
  decode("four byte", "\xf0\x9f\x8e\xae", -1, (const uint32_t[]){0x1F3AE}, 1);
  /* malformed input: a stray continuation byte, a lead byte with nothing after it, an invalid lead */
  decode("stray continuation", "a\x80" "b", -1, (const uint32_t[]){'a', '?', 'b'}, 3);
  decode("truncated at NUL", "a\xc3", -1, (const uint32_t[]){'a', '?'}, 2);
  decode("lead then ascii", "\xc3" "b", -1, (const uint32_t[]){'?', 'b'}, 2);
  decode("invalid lead", "\xff\xfe", -1, (const uint32_t[]){'?', '?'}, 2);
  /* the byte limit stops a sequence that crosses it without reading past it */
  decode("limit inside sequence", "ab\xc3\xa9", 3, (const uint32_t[]){'a', 'b', '?'}, 3);
  decode("limit at sequence end", "ab\xc3\xa9", 4, (const uint32_t[]){'a', 'b', 0xE9}, 3);
  /* clip boundary: stepping back from inside a sequence lands before its lead byte */
  const char *s = "a\xc3\xa9" "b"; /* bytes: a C3 A9 b */
  boundary(s, 4, 4);
  boundary(s, 3, 3);
  boundary(s, 2, 1);
  boundary(s, 1, 1);
  boundary(s, 0, 0);
  /* the ASCII fold for the VMU screen */
  char out[32];
  sw_utf8_fold_ascii(out, sizeof(out), "\xc3\x89" "t\xc3\xa9 \xc3\x91" "and\xc3\xba \xc3\xa7" "a \xe2\x82\xac \xc3\x9f");
  if (strcmp(out, "Ete Nandu ca ? s") != 0) {
    printf("FAIL fold: '%s'\n", out);
    fail = 1;
  }
  sw_utf8_fold_ascii(out, 4, "abcdef"); /* size limit, always terminated */
  if (strcmp(out, "abc") != 0) {
    printf("FAIL fold limit: '%s'\n", out);
    fail = 1;
  }
  if (fail)
    return 1;
  printf("utf8 check: all cases pass\n");
  return 0;
}

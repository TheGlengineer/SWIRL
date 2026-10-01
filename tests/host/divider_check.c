/* SWIRL host check for sw_lib_divider_label (ui/swirl/sw_divider.c): which GDMENU slot names are dividers,
   and what label they give. Built with the sanitizers by run.sh. */
#include <stdio.h>
#include <string.h>

#include "ui/swirl/sw_lib.h"

static int fail;
static void expect(const char *name, int want_len, const char *want_label) {
  char label[32];
  int got = sw_lib_divider_label(name, label, sizeof(label));
  if (got != want_len || (want_len >= 0 && strcmp(label, want_label) != 0)) {
    printf("FAIL '%s': got %d '%s' want %d '%s'\n", name ? name : "(null)", got, label, want_len, want_label);
    fail = 1;
  }
}

int main(void) {
  expect("*****USA*****", 3, "USA");
  expect("--- Arcade ---", 6, "Arcade");
  expect("  == Fighting ==  ", 8, "Fighting");
  expect("######", 0, "");
  expect("---", -1, ""); /* three marks alone is a name */
  expect("****  ", 0, "");
  expect("~~ Homebrew and Other ~~", 18, "Homebrew and Other");
  expect("++ A label longer than the thirty one characters a section name holds ++", 31,
         "A label longer than the thirty ");
  expect("Sonic Adventure", -1, "");
  expect("***Name", -1, ""); /* marks on one side only: a game called that */
  expect("-Dash-", -1, "");  /* one mark each side is a name */
  expect("*****USA-----", -1, ""); /* different marks */
  expect("", -1, "");
  expect(NULL, -1, "");
  char small[4];
  int n = sw_lib_divider_label("== Racing ==", small, sizeof(small));
  if (n != 3 || strcmp(small, "Rac") != 0) {
    printf("FAIL small buffer: got %d '%s'\n", n, small);
    fail = 1;
  }
  if (!fail) printf("divider check: all cases pass\n");
  return fail;
}

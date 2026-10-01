/* SWIRL host check for ui_pad_concat (ui/common.h): the padded info lines of the Classic styles never write
   past their buffer, whatever the inputs. Built with the sanitizers by run.sh. */
#include <stdio.h>
#include <string.h>

#include "ui/common.h"

static int fail;
static void expect(const char *got, const char *want, const char *what) {
  if (strcmp(got, want) != 0) {
    printf("FAIL %s: got '%s' want '%s'\n", what, got, want);
    fail = 1;
  }
}

int main(void) {
  char b[26];
  ui_pad_concat(b, sizeof(b), "Region", "USA", 16);
  expect(b, "Region       USA", "pads to the width");
  ui_pad_concat(b, sizeof(b), "Version", "V1.000", 16);
  expect(b, "Version   V1.000", "pads a 13 character pair");
  ui_pad_concat(b, sizeof(b), "Disc", "1/1", 25);
  expect(b, "Disc                  1/1", "fills the 26 byte buffer exactly");
  ui_pad_concat(b, sizeof(b), "Date", "A very long date string here", 16);
  expect(b, "DateA very long date stri", "cuts a right string that would not fit");
  ui_pad_concat(b, sizeof(b), "A left string longer than the buffer itself", "x", 16);
  expect(b, "A left string longer than", "cuts a left string that would not fit");
  ui_pad_concat(b, sizeof(b), "", "", 16);
  expect(b, "                ", "two empty strings give the padding");
  char tiny[1];
  ui_pad_concat(tiny, sizeof(tiny), "abc", "def", 10);
  expect(tiny, "", "a one byte buffer holds the terminator only");
  if (!fail) printf("pad check: all cases pass\n");
  return fail;
}

/* GDMENU card dividers: see sw_lib_divider_label in sw_lib.h. Plain C, no KallistiOS, so
   tests/host/divider_check.c can build it. */
#include <string.h>

#include "sw_lib.h"

int sw_lib_divider_label(const char *name, char *label, int cap) {
  if (cap > 0) label[0] = 0;
  if (!name) return -1;
  while (*name == ' ') name++;
  const char mark = *name;
  if (!mark || !strchr("*-=#~_+", mark)) return -1;
  const char *p = name;
  int lead = 0;
  while (*p == mark) p++, lead++;
  const char *end = name + strlen(name);
  while (end > p && end[-1] == ' ') end--;
  if (p == end) { /* a bare line of marks is a divider with no label */
    if (lead < 4) return -1;
    return 0;
  }
  if (lead < 2) return -1;
  int trail = 0;
  while (end > p && end[-1] == mark) end--, trail++;
  if (trail < 2) return -1; /* "***Name" with nothing after is a name, not a divider */
  while (p < end && *p == ' ') p++;
  while (end > p && end[-1] == ' ') end--;
  int len = (int)(end - p);
  if (cap > 0) {
    if (len > cap - 1) len = cap - 1;
    memcpy(label, p, len);
    label[len] = 0;
  }
  return len;
}

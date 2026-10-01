/* SWIRL: the menu's text in the chosen language (sw_lang.h).

   LANG.DAT (little endian), written by SWIRL Card Manager from swirl/lang/<code>.json:
     "SWL1"   magic
     u32      table hash: FNV-1a of the key names in table order joined by '\n' (sw_lang.h)
     u32      strings per language (S_COUNT when the file was made)
     u32      language count
     per language, 16 bytes: u8 code (SW_LANG_*), u8 pad[3], u32 offset from the file start, u32 size, u32 0
     per language block: u32 offset[strings] from the block start (0: not translated), then the UTF-8
              strings, each NUL terminated
   The file is read once per selection: the header, then only the chosen language's block, which stays in
   memory (10 to 20 KB). Anything that does not fit this layout leaves the menu in English with warning W20. */
#include "sw_lang.h"

#include <dc/flashrom.h>
#include <kos/fs.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "sw_trace.h"

#define SW_LANG_STR(id, text) text,
static const char *const english[S_COUNT] = {SW_STRINGS(SW_LANG_STR)};
#undef SW_LANG_STR
#define SW_LANG_KEY(id, text) #id,
static const char *const keys[S_COUNT] = {SW_STRINGS(SW_LANG_KEY)};
#undef SW_LANG_KEY

#define MAX_FILE (512 * 1024)
#define MAX_LANGS 16

static char lang_path[32];
static int file_state = -1;             /* -1 none, -2 unusable, 0 ok */
static int nlangs;
static struct { int code; uint32_t off, size; } langs[MAX_LANGS];
static uint32_t file_count;             /* strings per language in the file */
static int current = SW_LANG_EN;
static char *block;                     /* the loaded language's block */
static const char *cur[S_COUNT];        /* NULL where the file has no translation */

const char *sw_t(int id) {
  if (id < 0 || id >= S_COUNT)
    return "";
  const char *s = cur[id];
  return (s && s[0]) ? s : english[id];
}

const char *sw_lang_key(int id) {
  return (id >= 0 && id < S_COUNT) ? keys[id] : "";
}

static uint32_t table_hash(void) {
  uint32_t h = 2166136261u;
  for (int i = 0; i < S_COUNT; i++) {
    for (const char *p = keys[i]; *p; p++)
      h = (h ^ (unsigned char)*p) * 16777619u;
    h = (h ^ '\n') * 16777619u;
  }
  return h;
}

static uint32_t rd32(const unsigned char *p) {
  return p[0] | (p[1] << 8) | (p[2] << 16) | ((uint32_t)p[3] << 24);
}

static void drop_block(void) {
  free(block);
  block = NULL;
  memset(cur, 0, sizeof(cur));
}

int sw_lang_load(const char *path) {
  snprintf(lang_path, sizeof(lang_path), "%s", path);
  nlangs = 0;
  file_state = -1;
  file_t f = fs_open(path, O_RDONLY);
  if (f == FILEHND_INVALID)
    return -1;
  unsigned char hdr[16 + 16 * MAX_LANGS];
  const ssize_t total = fs_total(f);
  const ssize_t got = fs_read(f, hdr, sizeof(hdr));
  fs_close(f);
  file_state = -2;
  if (total < 16 || total > MAX_FILE || got < 16 || memcmp(hdr, "SWL1", 4) != 0) {
    sw_warn(SW_WARN_LANG, "LANG.DAT: not a SWIRL language file");
    return -2;
  }
  const uint32_t hash = rd32(hdr + 4), count = rd32(hdr + 8), n = rd32(hdr + 12);
  if (hash != table_hash()) {
    sw_warn(SW_WARN_LANG, "LANG.DAT: made for another SWIRL version (Update SWIRL in Card Manager)");
    return -2;
  }
  if (count == 0 || count > S_COUNT || n == 0 || n > MAX_LANGS || got < 16 + 16 * (ssize_t)n) {
    sw_warn(SW_WARN_LANG, "LANG.DAT: bad header (%lu strings, %lu languages)", (unsigned long)count,
            (unsigned long)n);
    return -2;
  }
  file_count = count;
  for (uint32_t i = 0; i < n; i++) {
    const unsigned char *e = hdr + 16 + 16 * i;
    const int code = e[0];
    const uint32_t off = rd32(e + 4), size = rd32(e + 8);
    if (code <= SW_LANG_AUTO || code >= SW_LANG_COUNT || off < 16 + 16 * n || size < 4 * count ||
        off + size > (uint32_t)total) {
      sw_warn(SW_WARN_LANG, "LANG.DAT: bad language entry %lu", (unsigned long)i);
      nlangs = 0;
      return -2;
    }
    langs[nlangs].code = code;
    langs[nlangs].off = off;
    langs[nlangs].size = size;
    nlangs++;
  }
  file_state = 0;
  sw_trace("LANG.DAT: %d languages, %lu strings; console language %d", nlangs, (unsigned long)count,
           sw_lang_console());
  return 0;
}

int sw_lang_available(int lang) {
  if (lang == SW_LANG_EN)
    return 1;
  for (int i = 0; i < nlangs; i++)
    if (langs[i].code == lang)
      return 1;
  return 0;
}

int sw_lang_console(void) {
  flashrom_syscfg_t cfg;
  static int traced;
  const int rv = flashrom_get_syscfg(&cfg);
  if (!traced) {
    traced = 1;
    sw_trace("flashrom syscfg: %s, language %d", rv == 0 ? "read" : "not readable", rv == 0 ? cfg.language : -1);
  }
  if (rv != 0)
    return SW_LANG_EN;
  /* the flashrom order is Japanese 0, English 1, German 2, French 3, Spanish 4, Italian 5: SW_LANG_* matches */
  if (cfg.language >= SW_LANG_EN && cfg.language <= SW_LANG_IT)
    return cfg.language;
  return SW_LANG_EN; /* Japanese and anything unknown */
}

/* reads one language block into memory and points cur[] into it; 0 on success */
static int load_block(int idx) {
  drop_block();
  file_t f = fs_open(lang_path, O_RDONLY);
  if (f == FILEHND_INVALID)
    return -1;
  const uint32_t size = langs[idx].size;
  char *b = malloc(size + 1);
  if (!b) {
    fs_close(f);
    return -1;
  }
  int ok = fs_seek(f, langs[idx].off, SEEK_SET) == (off_t)langs[idx].off && fs_read(f, b, size) == (ssize_t)size;
  fs_close(f);
  if (!ok) {
    free(b);
    sw_warn(SW_WARN_LANG, "LANG.DAT: could not read language %d", langs[idx].code);
    return -1;
  }
  b[size] = 0; /* a string that runs to the end of the block still ends */
  block = b;
  for (uint32_t i = 0; i < file_count && i < S_COUNT; i++) {
    const uint32_t off = rd32((unsigned char *)b + 4 * i);
    if (off == 0)
      continue;
    if (off < 4 * file_count || off >= size) {
      sw_warn(SW_WARN_LANG, "LANG.DAT: bad offset for %s", keys[i]);
      drop_block();
      return -1;
    }
    cur[i] = b + off;
  }
  return 0;
}

void sw_lang_select(int lang) {
  if (lang == SW_LANG_AUTO)
    lang = sw_lang_console();
  if (lang <= SW_LANG_AUTO || lang >= SW_LANG_COUNT)
    lang = SW_LANG_EN;
  current = SW_LANG_EN;
  drop_block();
  if (lang == SW_LANG_EN || file_state != 0)
    return;
  for (int i = 0; i < nlangs; i++) {
    if (langs[i].code == lang) {
      if (load_block(i) == 0)
        current = lang;
      return;
    }
  }
}

int sw_lang_current(void) {
  return current;
}

const char *sw_lang_name(int lang) {
  /* the file holds each language's own name as its first string; the built in names cover the rest */
  static const char *const names[SW_LANG_COUNT] = {"", "English", "Deutsch", "Fran\xc3\xa7" "ais",
                                                   "Espa\xc3\xb1" "ol", "Italiano", "Portugu\xc3\xaa" "s"};
  if (lang == current && cur[S_LANG_NAME] && cur[S_LANG_NAME][0])
    return cur[S_LANG_NAME];
  return (lang > SW_LANG_AUTO && lang < SW_LANG_COUNT) ? names[lang] : "";
}

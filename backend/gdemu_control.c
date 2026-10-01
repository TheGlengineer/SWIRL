/*
 * Game launching through GDEMU.
 *
 * SWIRL: the public openMenu source only switched the GDEMU image and then exited to the BIOS menu
 * (arch_menu), so the user had to press Play there. This version hands off to the GDMENU loader the
 * same way the maintained openMenu fork does (github.com/DerekPascarella/openMenu-Virtual-Folder-Bundle,
 * BSD): loader parameters at 0xACCFFF00, a few BIOS patches, then arch_exec() of the loader, which
 * boots the selected disc directly. In-game reset (IGR) is enabled so A+B+X+Y+Start returns to SWIRL.
 *
 * CodeBreaker (PELICAN.BIN + cheats/) and Bleem (BLEEM.BIN, PlayStation discs) launching follow the
 * same fork; those files are taken from the menu the card already had.
 */
#include <arch/arch.h>
#include <dc/maple.h>
#include <kos/genwait.h>
#include <dc/cdrom.h>
#include <dc/sound/sound.h>
#include <kos.h>
#include <kos/thread.h>
#include <stdio.h>
#include <string.h>
#include <strings.h>

#include "backend/gd_item.h"
#include "cb_loader.h"
#include "controls.p1.h"
#include "gdemu_control.h"
#include "gdemu_sdk.h"
#include "gdmenu_binary.h"
#include "ui/swirl/sw_trace.h"
void run_game(const char *region, const char *product);

/* called before handing the machine to another program (stops music and sound effects) */
void (*gdemu_before_launch)(void) = NULL;

void gd_reset_handles(void) {
}

/* Wait until the GD drive reports the newly selected image (up to ~10 s); 0 when it did */
static int wait_cd_ready(void) {
  for (int i = 0; i < 500; i++) {
    if (cdrom_reinit() == ERR_OK)
      return 0;
    thd_sleep(20);
  }
  return -1;
}

/* SWIRL: why the last launch came back to the menu, or NULL. A launch used to hand the console to the loader
   whatever the drive said; a game that never became ready was then a blank screen with nothing to report. */
static const char *launch_error;
const char *gdemu_launch_error(void) { return launch_error; }

/* the drive did not switch or did not become ready: go back to the menu disc so the menu can carry on */
static void launch_failed(const char *why) {
  launch_error = why;
  sw_warn(strstr(why, "GDEMU") ? SW_WARN_GDEMU : SW_WARN_LAUNCH, "launch failed: %s", why);
  if (gdemu_set_img_num(1) != 0 || wait_cd_ready() != 0) {
    /* the menu disc is not back either: nothing more can be read from the card, so stop with the report */
    sw_trace_fatal_reason(SW_REPORT_LAUNCH, "GDEMU did not answer. Switch the console off and on.");
  }
  sw_trace("launch: back on the menu disc");
}

static int region_code(const char *region, int override) {
  switch (override) {
    case LAUNCH_REGION_JAPAN: return 0;
    case LAUNCH_REGION_USA: return 1;
    case LAUNCH_REGION_EUROPE: return 2;
    default: break;
  }
  if (!strncmp(region, "JUE", 3))
    return (int)(((uint8_t *)0x8C000072)[0] & 7); /* console's own region */
  switch (region[0]) {
    case 'J': return 0;
    case 'U': return 1;
    case 'E': return 2;
    default: return (int)(((uint8_t *)0x8C000072)[0] & 7);
  }
}

static void quiet(void) {
  if (gdemu_before_launch)
    gdemu_before_launch();
}

/* ---------- Game ID for VMU replacements (VM2, VMU Pro, USB4MAPLE, Pico2Maple) ---------- */
/*
 * These devices keep a separate virtual memory card per game. When a game starts, SWIRL tells them which one
 * (maple command 33 with the disc's product ID and name), so each game gets its own card. A standard VMU is never
 * sent anything new: each memory card is first asked for its full device information (ALLINFO) and only the four
 * devices below, which name themselves there, get the Game ID.
 *
 * Protocol and device names from the openMenu Virtual Folder Bundle (Derek Pascarella, BSD 3-Clause,
 * src/openmenu/src/vm2/vm2_api.c). SWIRL's version limits every wait: 200 ms per reply, a few retries when the
 * device says "again", and at most about a second for all cards together, so a launch can never hang here.
 * It is only sent at launch, after SWIRL's last save, so SWIRL never reads or writes a card that is switching.
 */

#define MAPLE_COMMAND_GAMEID 33
#define REPLY_WAIT_MS 200
#define AGAIN_TRIES 5
#define TOTAL_MS 1000

/* the reply is copied here by the callback (interrupt time) */
static uint8_t reply[4 + 192];
static volatile int reply_len;

static void on_reply(maple_state_t *st, maple_frame_t *frm) {
  (void)st;
  maple_response_t *resp = (maple_response_t *)frm->recv_buf;
  int n = 4 + resp->data_len * 4;
  if (n > (int)sizeof(reply)) n = sizeof(reply);
  memcpy(reply, resp, n);
  reply_len = n;
  maple_frame_unlock(frm);
  genwait_wake_all(frm);
}

/* sends one frame and waits for its reply; 0 when a reply came. data must stay valid (static) until then. */
static int exchange(maple_device_t *dev, int cmd, const void *data, int words) {
  /* the frame is free unless something else is using this card; give up quickly if so */
  int locked = 0;
  for (int i = 0; i < 20 && !locked; i++) {
    if (maple_frame_lock(&dev->frame) == 0) locked = 1;
    else thd_sleep(5);
  }
  if (!locked) return -1;

  memset(reply, 0, sizeof(reply));
  reply_len = 0;
  maple_frame_init(&dev->frame);
  /* KOS sends a frame again by itself when the device answers "again", so the message must not share the buffer
     the reply is written to (as KOS's own VMU commands do); it is copied from here on every send */
  void *send_buf = words ? (void *)data : (void *)dev->frame.recv_buf;
  dev->frame.cmd = cmd;
  dev->frame.dst_port = dev->port;
  dev->frame.dst_unit = dev->unit;
  dev->frame.length = words;
  dev->frame.callback = on_reply;
  dev->frame.send_buf = send_buf;
  maple_queue_frame(&dev->frame);

  if (genwait_wait(&dev->frame, "sw_gameid", REPLY_WAIT_MS, NULL) < 0) {
    if (dev->frame.state != MAPLE_FRAME_UNSENT) {
      /* no answer: free the frame, as KOS does for its own VMU commands */
      dev->frame.state = MAPLE_FRAME_VACANT;
      return -1;
    }
  }
  return reply_len ? 0 : -1;
}

/* the 16 character name a VMU replacement gives in its full device information, or NULL for a standard VMU */
static const char *replacement_name(maple_device_t *dev) {
  static const struct { const char *id, *name; } known[] = {
    {"VM2 by Dreamware", "VM2"},
    {"8BITMODS VMUPro ", "VMU Pro"},
    {"USB RP2040 EMU  ", "USB4MAPLE"},
    {"Pico2Maple USBBT", "Pico2Maple"},
  };
  if (exchange(dev, MAPLE_COMMAND_ALLINFO, NULL, 0) != 0) return NULL;
  maple_response_t *resp = (maple_response_t *)reply;
  /* functions, 3 function data words, area, direction, name 30, license 60, power 2 + 2: 112 bytes, then extended */
  if (resp->response != MAPLE_RESPONSE_ALLINFO || resp->data_len * 4 < 112 + 16) return NULL;
  const char *ext = (const char *)reply + 4 + 112;
  for (unsigned i = 0; i < sizeof(known) / sizeof(known[0]); i++)
    if (!strncasecmp(ext, known[i].id, 16)) return known[i].name;
  return NULL;
}

static int send_id(maple_device_t *dev, const gd_item *disc, uint64_t until) {
  /* word 0 memory card function, then the product ID (12 bytes) and the name (128 bytes) */
  static uint32_t msg[36];
  memset(msg, 0, sizeof(msg));
  msg[0] = MAPLE_FUNC_MEMCARD;
  strncpy((char *)&msg[1], disc->product, 12);
  strncpy((char *)&msg[4], disc->name, 128);
  for (int i = 0; i < AGAIN_TRIES && timer_ms_gettime64() < until; i++) {
    if (exchange(dev, MAPLE_COMMAND_GAMEID, msg, 36) != 0) return -1;
    int r = ((maple_response_t *)reply)->response;
    if (r == MAPLE_RESPONSE_OK) return 0;
    if (r != MAPLE_RESPONSE_AGAIN) return -1;
    thd_sleep(20);
  }
  return -1;
}

static int gameid_send(const gd_item *disc) {
  if (!disc || !disc->product[0]) return 0;
  const uint64_t until = timer_ms_gettime64() + TOTAL_MS;
  int sent = 0;
  for (int i = 0; i < 8 && timer_ms_gettime64() < until; i++) {
    maple_device_t *dev = maple_enum_type(i, MAPLE_FUNC_MEMCARD);
    if (!dev) break;
    if (!dev->valid) continue;
    const char *kind = replacement_name(dev);
    if (!kind) continue; /* a standard VMU: nothing is sent */
    int rv = send_id(dev, disc, until);
    dbglog(DBG_INFO, "SWIRL: Game ID %.12s to %s on %c%d: %s\n", disc->product, kind, 'A' + dev->port, dev->unit,
           rv ? "no answer" : "OK");
    if (!rv) sent++;
  }
  return sent;
}

/* System > Game ID for VM2 / VMU Pro (on unless turned off; kept in SWIRL.DAT) */
extern int sw_gameid_enabled(void);

/* after the disc is chosen: a VM2 or VMU Pro switches to this game's own card. SWIRL has finished saving by now. */
static void send_game_id(const gd_item *disc) {
  if (sw_gameid_enabled())
    gameid_send(disc);
}

/* ---------- SWIRL: memory bounds (SW-4) ----------
   The loader's parameters go to 0x8CCFFF00, CodeBreaker's cheats to 0x8CD00000 and its CD loader to
   0x8CE10000, all above the menu's memory. openMenu only printed where the menu's memory ended; a menu that had
   grown past the line (a very large library, a leak) wrote its parameters over its own memory and the launch
   came back wrong or not at all. A launch that would cross the line now stops with a message. The patches to
   PELICAN.BIN and BLEEM.BIN are applied only to files long enough to hold them, and a cheat file must fit
   between the cheat area and the CD loader. */
#define LOADER_DATA 0x8CCFFF00u
#define CHEAT_AREA 0x8CD00000u
#define CB_LOADER_AREA 0x8CE10000u
#define PELICAN_MIN_SIZE (310786u * 2u) /* the last patched 16 bit word, pelican[310785] */
#define BLEEM_MIN_SIZE (0x7079Cu + 256u)  /* the controller table patch at 0x7079C (altctrl is under 256 bytes) */

static uintptr_t heap_end(void) {
  extern void *sbrk(intptr_t);
  return (uintptr_t)sbrk(0);
}

/* 1 when the menu's memory ends below the loader's data (and says where it ends in the trace) */
static int memory_fits(void) {
  const uintptr_t end = heap_end();
#ifdef SW_TEST_BIG_HEAP
  {
    /* test only: grow the menu's memory past the line, so the launch has to stop */
    static volatile uint8_t *big;
    if (!big && (big = malloc(12 * 1024 * 1024))) big[12 * 1024 * 1024 - 1] = 1;
  }
#endif
  if (heap_end() > LOADER_DATA) {
    sw_warn(SW_WARN_MEMORY, "launch: memory ends at %08lx, past the loader data at %08lx", (unsigned long)heap_end(),
            (unsigned long)LOADER_DATA);
    return 0;
  }
  (void)end;
  return 1;
}

static void launch_loader(const char *region, int game_fix, const launch_opts *o);
static void launch_loader(const char *region, int game_fix, const launch_opts *o) {
  static const launch_opts defaults = {LAUNCH_REGION_AUTO, 1, LAUNCH_BOOT_BOTH}; /* the full start, as openMenu */
  if (!o)
    o = &defaults;
  ldr_params_t param;
  memset(&param, 0, sizeof(param));
  param.region_free = 1;
  param.force_vga = o->vga ? 1 : 0;
  param.IGR = 1; /* A+B+X+Y+Start in game resets back to the menu */
  param.boot_intro = (o->boot == LAUNCH_BOOT_ANIMATION || o->boot == LAUNCH_BOOT_BOTH) ? 1 : 0;
  param.sega_license = (o->boot == LAUNCH_BOOT_LICENSE || o->boot == LAUNCH_BOOT_BOTH) ? 1 : 0;
  param.game_region = region_code(region, o->region);
  sw_trace("launch: options: animation %u, SEGA screen %u, VGA %u, region %u", (unsigned)param.boot_intro, (unsigned)param.sega_license,
           (unsigned)param.force_vga, (unsigned)param.game_region);

  sw_trace("launch: waiting for the disc");
  if (wait_cd_ready() != 0) {
    launch_failed("The game's disc did not become ready");
    return;
  }
  int status = 0, disc_type = 0;
  cdrom_get_status(&status, &disc_type);
  param.disc_type = (disc_type == CD_GDROM);
  sw_trace("launch: disc ready (type %d), memory ends at %08lx (loader data at %08lx)", disc_type,
           (unsigned long)heap_end(), (unsigned long)LOADER_DATA);
  param.need_game_fix = game_fix;

  /* BIOS version specific patches used by GDMENU / openMenu */
  if (!strncmp((char *)0x8c0007CC, "1.004", 5)) {
    ((uint32_t *)0xAC000E20)[0] = 0;
  } else if (!strncmp((char *)0x8c0007CC, "1.01d", 5) || !strncmp((char *)0x8c0007CC, "1.01c", 5)) {
    ((uint32_t *)0xAC000E1C)[0] = 0;
  }
  ((uint32_t *)0xAC0000E4)[0] = -3;

  memcpy((void *)0xACCFFF00, &param, 32);

  arch_exec(gdmenu_loader, gdmenu_loader_length);
}

void run_game(const char *region, const char *product) {
  (void)product;
  launch_loader(region, 0, NULL);
}

static int needs_fix(const gd_item *disc) {
  return !strncmp(disc->name, "PSO VER.2", 9) || !strncmp(disc->name, "SONIC ADVENTURE 2", 17);
}

/* An entry whose type is "other" (the Virtual Folder Bundle's word for an audio CD or anything that is not a
   Dreamcast game) cannot go through the loader. The image is selected and the console goes to its own menu,
   which plays or starts what is in the drive; that is how openMenu itself started every disc. */
static int is_other_disc(const gd_item *disc) {
  return !strcasecmp(disc->type, "other");
}

static void launch_bios_exit(gd_item *disc) {
  sw_trace("launch: disc %u (%.12s) to the console's menu", disc->slot_num, disc->product);
  if (gdemu_set_img_num((uint16_t)disc->slot_num) != 0) {
    launch_failed("GDEMU did not answer the image change");
    return;
  }
  thd_sleep(200);
  send_game_id(disc);
  if (wait_cd_ready() != 0) {
    launch_failed("The disc did not become ready");
    return;
  }
  arch_menu();
}

/* every wait for the drive (image change 3 s, disc ready 10 s, back to the menu disc 13 s) is inside this */
#define LAUNCH_EXPECT_MS 30000

void dreamcast_launch_disc_ex(gd_item *disc, const launch_opts *o) {
  quiet();
  launch_error = NULL;
  sw_watchdog_expect(LAUNCH_EXPECT_MS);
  if (is_other_disc(disc)) {
    launch_bios_exit(disc);
    return;
  }
  sw_trace("launch: disc %u (%.12s)", disc->slot_num, disc->product);
  if (!memory_fits()) {
    launch_error = "Out of memory: switch the console off and on";
    return;
  }
  if (gdemu_set_img_num((uint16_t)disc->slot_num) != 0) {
    launch_failed("GDEMU did not answer the image change");
    return;
  }
  thd_sleep(200);
  sw_trace("launch: Game ID");
  send_game_id(disc);
  launch_loader(disc->region, needs_fix(disc), o);
}

void dreamcast_launch_disc(gd_item *disc) {
  dreamcast_launch_disc_ex(disc, NULL);
}

/* read a whole file from the menu disc into a 32 byte aligned buffer; *raw_out is what to free */
static uint8_t *load_file(const char *path, uint32_t *size_out, uint8_t **raw_out) {
  *raw_out = NULL;
  file_t fd = fs_open(path, O_RDONLY);
  if (fd == FILEHND_INVALID)
    return NULL;
  ssize_t size = fs_total(fd);
  if (size <= 0 || size > 8 * 1024 * 1024) {
    sw_trace("launch: %s is %ld bytes, not loaded", path, (long)size);
    fs_close(fd);
    return NULL;
  }
  uint8_t *raw = malloc(size + 64);
  if (!raw) {
    sw_trace("launch: no memory for %s (%ld bytes)", path, (long)size);
    fs_close(fd);
    return NULL;
  }
  uint8_t *buf = (uint8_t *)(((uint32_t)raw + 31) & ~31u);
  const ssize_t got = fs_read(fd, buf, size);
  fs_close(fd);
  if (got != size) {
    sw_trace("launch: %s short read (%ld of %ld)", path, (long)got, (long)size);
    free(raw);
    return NULL;
  }
  *size_out = (uint32_t)size;
  *raw_out = raw;
  return buf;
}

int codebreaker_available(void) {
  file_t fd = fs_open("/cd/PELICAN.BIN", O_RDONLY);
  if (fd == FILEHND_INVALID)
    return 0;
  fs_close(fd);
  return 1;
}

int bleem_available(void) {
  file_t fd = fs_open("/cd/BLEEM.BIN", O_RDONLY);
  if (fd == FILEHND_INVALID)
    return 0;
  fs_close(fd);
  return 1;
}

int is_psx_disc(const gd_item *disc) {
  return !strcmp(disc->type, "psx");
}

void dreamcast_launch_cb(gd_item *disc) {
  uint32_t cb_size = 0, cheat_size = 0;
  uint8_t *cb_raw = NULL, *cheat_raw = NULL;
  launch_error = NULL;
  uint8_t *cb_buf = load_file("/cd/PELICAN.BIN", &cb_size, &cb_raw);
  if (!cb_buf)
    return;
  if (cb_size < PELICAN_MIN_SIZE) {
    /* a different CodeBreaker version: the patches below would land outside it */
    sw_warn(SW_WARN_LAUNCH, "launch: PELICAN.BIN is %lu bytes, at least %lu needed", (unsigned long)cb_size, (unsigned long)PELICAN_MIN_SIZE);
    launch_error = "PELICAN.BIN is not a version SWIRL can start";
    free(cb_raw);
    return;
  }
  quiet();

  /* cheats for this game, else the full CodeBreaker list */
  char cheat_name[40];
  snprintf(cheat_name, sizeof(cheat_name), "/cd/cheats/%s.bin", disc->product);
  uint32_t csize = 0;
  uint8_t *cheat_buf = load_file(cheat_name, &csize, &cheat_raw);
  if (!cheat_buf) {
    /* SWIRL Card Manager's disc writer turns "-" into "_" (T-8101N.BIN is stored as T_8101N.BIN) */
    for (char *c = cheat_name + 11; *c; c++)
      if (*c == '-') *c = '_';
    cheat_buf = load_file(cheat_name, &csize, &cheat_raw);
  }
  if (!cheat_buf)
    cheat_buf = load_file("/cd/cheats/FCDCHEATS.BIN", &csize, &cheat_raw);
  if (cheat_buf && csize > 640 && !strncmp((const char *)cheat_buf, "XploderDC Cheats", 16)) {
    cheat_size = csize - 640;
    cheat_buf += 640;
    if (!((uint32_t *)cheat_buf)[0])
      cheat_size = 0;
  }
  if (cheat_size > CB_LOADER_AREA - CHEAT_AREA) {
    /* the cheats would run into the CD loader's place */
    sw_warn(SW_WARN_LAUNCH, "launch: cheat file is %lu bytes, at most %lu fit", (unsigned long)cheat_size,
            (unsigned long)(CB_LOADER_AREA - CHEAT_AREA));
    launch_error = "The cheat file is too large for CodeBreaker";
    free(cb_raw);
    free(cheat_raw);
    return;
  }
  if (!memory_fits()) {
    launch_error = "Out of memory: switch the console off and on";
    free(cb_raw);
    free(cheat_raw);
    return;
  }

  sw_watchdog_expect(LAUNCH_EXPECT_MS);
  if (gdemu_set_img_num((uint16_t)disc->slot_num) != 0) {
    launch_failed("GDEMU did not answer the image change");
    free(cb_raw);
    free(cheat_raw);
    return;
  }
  thd_sleep(200);
  send_game_id(disc);
  if (wait_cd_ready() != 0) {
    launch_failed("The game's disc did not become ready");
    free(cb_raw);
    free(cheat_raw);
    return;
  }

  ((uint16_t *)0xAC000198)[0] = 0xFF86;

  int status = 0, disc_type = 0;
  cdrom_get_status(&status, &disc_type);

  if (cheat_size) {
    uint16_t *pelican = (uint16_t *)cb_buf;
    pelican[128] = 0;
    pelican[129] = 0x90;
    pelican[10818] = (uint16_t)cheat_size;
    pelican[10819] = (cheat_size >> 16);
    pelican[10820] = 0;
    pelican[10821] = 0x8CD0;
    memcpy((void *)(CHEAT_AREA | 0x20000000u), cheat_buf, cheat_size);
  }

  if (disc_type != CD_GDROM) {
    CDROM_TOC toc;
    cdrom_read_toc(&toc, 0);
    uint32_t lba = cdrom_locate_data_track(&toc);
    uint16_t *pelican = (uint16_t *)cb_buf;
    pelican[4067] = 0x711F;
    pelican[4074] = 0xE500;
    pelican[4302] = (uint16_t)lba;
    pelican[4303] = (uint16_t)(lba >> 16);
    pelican[472] = 0x0009;
    pelican[4743] = 0x0018;
    pelican[4745] = 0x0018;
    pelican[5261] = 0x0008;
    pelican[5433] = 0x0009;
    pelican[5436] = 0x0009;
    pelican[5438] = 0x0008;
    pelican[5460] = 0x0009;
    pelican[5472] = 0x0009;
    pelican[5511] = 0x0008;
    pelican[310573] = 0x64C3;
    pelican[310648] = 0x0009;
    pelican[310666] = 0x0009;
    pelican[310708] = 0x0018;
    pelican[310784] = 0x0000;
    pelican[310785] = 0x8CE1;
    memcpy((void *)(CB_LOADER_AREA | 0x20000000u), cb_loader_data, cb_loader_size);
  }

  arch_exec(cb_buf, cb_size);
}

void bleem_launch(gd_item *disc) {
  uint32_t size = 0;
  uint8_t *raw = NULL;
  launch_error = NULL;
  uint8_t *buf = load_file("/cd/BLEEM.BIN", &size, &raw);
  if (!buf)
    return;
  if (size < BLEEM_MIN_SIZE || (uint32_t)altctrl_size > 256) {
    sw_warn(SW_WARN_LAUNCH, "launch: BLEEM.BIN is %lu bytes, at least %lu needed", (unsigned long)size, (unsigned long)BLEEM_MIN_SIZE);
    launch_error = "BLEEM.BIN is not a version SWIRL can start";
    free(raw);
    return;
  }
  if (!memory_fits()) {
    launch_error = "Out of memory: switch the console off and on";
    free(raw);
    return;
  }
  quiet();
  sw_watchdog_expect(LAUNCH_EXPECT_MS);
  if (gdemu_set_img_num((uint16_t)disc->slot_num) != 0) {
    launch_failed("GDEMU did not answer the image change");
    free(raw);
    return;
  }
  thd_sleep(200);
  send_game_id(disc);
  if (wait_cd_ready() != 0) {
    launch_failed("The game's disc did not become ready");
    free(raw);
    return;
  }

  ((uint16_t *)0xAC000198)[0] = 0xFF86;
  for (int i = 0; i < altctrl_size; i++)
    buf[i + 0x7079C] = altctrl_data[i];
  buf[0x49E6] = 0x06; /* restart the emulator: A+B+X+Y+Down */
  buf[0x49E7] = 0x0E; /* back to the menu: A+B+X+Y+Start */
  buf[0x1CA70] = 1;

  arch_exec(buf, size);
}

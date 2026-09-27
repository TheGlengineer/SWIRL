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
void run_game(const char *region, const char *product) __attribute__((noreturn));

/* called before handing the machine to another program (stops music and sound effects) */
void (*gdemu_before_launch)(void) = NULL;

void gd_reset_handles(void) {
}

/* Wait until the GD drive reports the newly selected image (up to ~10 s) */
static void wait_cd_ready(void) {
  for (int i = 0; i < 500; i++) {
    if (cdrom_reinit() == ERR_OK)
      return;
    thd_sleep(20);
  }
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

static void launch_loader(const char *region, int game_fix, const launch_opts *o) __attribute__((noreturn));
static void launch_loader(const char *region, int game_fix, const launch_opts *o) {
  static const launch_opts defaults = {LAUNCH_REGION_AUTO, 1, LAUNCH_BOOT_NONE};
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

  sw_trace("launch: waiting for the disc");
  wait_cd_ready();
  int status = 0, disc_type = 0;
  cdrom_get_status(&status, &disc_type);
  param.disc_type = (disc_type == CD_GDROM);
  {
    /* the loader's settings go to 0xACCFFF00 (0x8CCFFF00 cached): SWIRL's memory must end below it */
    extern void *sbrk(intptr_t);
    const uintptr_t heap_end = (uintptr_t)sbrk(0);
    sw_trace("launch: disc ready (type %d), memory ends at %08lx (loader data at 8ccfff00)", disc_type,
             (unsigned long)heap_end);
  }
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
  __builtin_unreachable();
}

void run_game(const char *region, const char *product) {
  (void)product;
  launch_loader(region, 0, NULL);
}

static int needs_fix(const gd_item *disc) {
  return !strncmp(disc->name, "PSO VER.2", 9) || !strncmp(disc->name, "SONIC ADVENTURE 2", 17);
}

void dreamcast_launch_disc_ex(gd_item *disc, const launch_opts *o) {
  quiet();
  sw_trace("launch: disc %u (%.12s)", disc->slot_num, disc->product);
  gdemu_set_img_num((uint16_t)disc->slot_num);
  thd_sleep(200);
  sw_trace("launch: Game ID");
  send_game_id(disc);
  launch_loader(disc->region, needs_fix(disc), o);
}

void dreamcast_launch_disc(gd_item *disc) {
  dreamcast_launch_disc_ex(disc, NULL);
}

/* read a whole file from the menu disc into a 32 byte aligned buffer */
static uint8_t *load_file(const char *path, uint32_t *size_out) {
  file_t fd = fs_open(path, O_RDONLY);
  if (fd == FILEHND_INVALID)
    return NULL;
  uint32_t size = fs_total(fd);
  uint8_t *raw = malloc(size + 64);
  if (!raw) {
    fs_close(fd);
    return NULL;
  }
  uint8_t *buf = (uint8_t *)(((uint32_t)raw + 31) & ~31u);
  fs_read(fd, buf, size);
  fs_close(fd);
  *size_out = size;
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
  uint8_t *cb_buf = load_file("/cd/PELICAN.BIN", &cb_size);
  if (!cb_buf)
    return;
  quiet();

  /* cheats for this game, else the full CodeBreaker list */
  char cheat_name[40];
  snprintf(cheat_name, sizeof(cheat_name), "/cd/cheats/%s.bin", disc->product);
  uint32_t csize = 0;
  uint8_t *cheat_buf = load_file(cheat_name, &csize);
  if (!cheat_buf) {
    /* SWIRL Card Manager's disc writer turns "-" into "_" (T-8101N.BIN is stored as T_8101N.BIN) */
    for (char *c = cheat_name + 11; *c; c++)
      if (*c == '-') *c = '_';
    cheat_buf = load_file(cheat_name, &csize);
  }
  if (!cheat_buf)
    cheat_buf = load_file("/cd/cheats/FCDCHEATS.BIN", &csize);
  if (cheat_buf && csize > 640 && !strncmp((const char *)cheat_buf, "XploderDC Cheats", 16)) {
    cheat_size = csize - 640;
    cheat_buf += 640;
    if (!((uint32_t *)cheat_buf)[0])
      cheat_size = 0;
  }

  gdemu_set_img_num((uint16_t)disc->slot_num);
  thd_sleep(200);
  send_game_id(disc);
  wait_cd_ready();

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
    memcpy((void *)0xACD00000, cheat_buf, cheat_size);
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
    memcpy((void *)0xACE10000, cb_loader_data, cb_loader_size);
  }

  arch_exec(cb_buf, cb_size);
}

void bleem_launch(gd_item *disc) {
  uint32_t size = 0;
  uint8_t *buf = load_file("/cd/BLEEM.BIN", &size);
  if (!buf)
    return;
  quiet();
  gdemu_set_img_num((uint16_t)disc->slot_num);
  thd_sleep(200);
  send_game_id(disc);
  wait_cd_ready();

  ((uint16_t *)0xAC000198)[0] = 0xFF86;
  for (int i = 0; i < altctrl_size; i++)
    buf[i + 0x7079C] = altctrl_data[i];
  buf[0x49E6] = 0x06; /* restart the emulator: A+B+X+Y+Down */
  buf[0x49E7] = 0x0E; /* back to the menu: A+B+X+Y+Start */
  buf[0x1CA70] = 1;

  arch_exec(buf, size);
}

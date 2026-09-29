
/*
 * File: main.c
 * Project: kos_pvr_texture_load
 * File Created: Wednesday, 23rd January 2019 8:07:09 pm
 * Author: Hayden Kowalchuk (hayden@hkowsoftware.com)
 * -----
 * Copyright (c) 2019 Hayden Kowalchuk
 */

#include <dc/cdrom.h>
#include <dc/flashrom.h>
#include <arch/timer.h>
#include <dc/maple.h>
#include <kos/thread.h>
#include <dc/maple/controller.h>
#include <dc/pvr.h>
#include <dc/video.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#include "backend/db_list.h"
#include "backend/gd_list.h"
#include "ui/common.h"
#include "ui/dc/input.h"
#include "ui/draw_prototypes.h"
#include "ui/global_settings.h"
#include "ui/swirl/sw_lib.h"
#include "ui/swirl/sw_trace.h"
#include "ui/swirl/sw_vmu.h"

/* UI Collection */
#include "ui/ui_grid.h"
#undef UI_NAME
#include "ui/ui_line_desc.h"
#undef UI_NAME
#include "ui/ui_gdmenu.h"
#undef UI_NAME
#include "ui/ui_bios.h"
#undef UI_NAME
#include "ui/ui_swirl.h"
#undef UI_NAME

#include "texture/txr_manager.h"

void (*current_ui_init)(void);
void (*current_ui_setup)(void);
void (*current_ui_draw_OP)(void);
void (*current_ui_draw_TR)(void);
void (*current_ui_handle_input)(unsigned int);

typedef struct ui_template {
  void (*init)(void);
  void (*setup)(void);
  void (*drawOP)(void);
  void (*drawTR)(void);
  void (*handle_input)(unsigned int);
} ui_template;

#define UI_TEMPLATE(name)                                                                                                                                                                \
  (ui_template) {                                                                                                                                                                        \
    .init = FUNC_NAME(name, init), .setup = FUNC_NAME(name, setup), .drawOP = FUNC_NAME(name, drawOP), .drawTR = FUNC_NAME(name, drawTR), .handle_input = FUNC_NAME(name, handle_input), \
  }

static ui_template ui_choices[] = {
    UI_TEMPLATE(LIST_DESC),
    UI_TEMPLATE(GRID_3),
    UI_TEMPLATE(GDMENU_EMU),
    UI_TEMPLATE(SWIRL), /* index must equal UI_SWIRL */
    UI_TEMPLATE(BIOS),
};

static const int num_ui_choices = sizeof(ui_choices) / sizeof(ui_template);
static int ui_choice_current = 0;

static void ui_set_choice(int choice) {
  if (choice < UI_START || choice >= num_ui_choices) {
    choice = UI_START;
  }
  current_ui_init = ui_choices[choice].init;
  current_ui_setup = ui_choices[choice].setup;
  current_ui_draw_OP = ui_choices[choice].drawOP;
  current_ui_draw_TR = ui_choices[choice].drawTR;
  current_ui_handle_input = ui_choices[choice].handle_input;

  ui_choice_current = choice;

  /* Call init & setup */
  (*current_ui_init)();
  (*current_ui_setup)();
}

int round(float x) {
  if (x < 0.0f)
    return (int)(x - 0.5f);
  else
    return (int)(x + 0.5f);
}

/* SWIRL: set when the style changes; the button that made the change (A on Save or Apply) is still down and must
   not count as a press in the new style (SWIRL would start the first game). Cleared once nothing is held. */
static int input_latched;

/* SWIRL: a style change asked for from inside a style's own input handler is applied once that handler has
   returned, never while it is still running (upstream PR #45 da47477 deferred it the same way after a "random
   freeze"). Before the main loop it is applied at once. */
static int ui_reload_pending;

static void apply_pending_reload(void) {
  if (!ui_reload_pending)
    return;
  ui_reload_pending = 0;
  openmenu_settings *settings = settings_get();
  sw_trace("style %d: loading", settings->ui);
  ui_set_choice(settings->ui);
  sw_trace("style %d: ready", settings->ui);
  input_latched = 1;
}

void reload_ui(void) {
  ui_reload_pending = 1;
}

static int init(void) {
  int ret = 0, r;

  /* Load settings */
  sw_trace("openMenu settings (VMU)");
  settings_init();

  /* SWIRL: each step is traced; a step that fails is noted and start up carries on (a missing picture file
     must not send the Dreamcast back to its BIOS) */
#define STEP(call)                                   \
  do {                                               \
    sw_trace("%s", #call);                           \
    if ((r = (call)) != 0) {                         \
      sw_warn(SW_WARN_STEP, "%s failed (%d)", #call, r);  \
      ret++;                                         \
    }                                                \
  } while (0)
  STEP(txr_create_small_pool());
  STEP(txr_create_large_pool());
  STEP(txr_load_DATs());
  STEP(list_read_default());
  STEP(db_load_DAT());
  STEP(theme_manager_load());
#undef STEP

  /* setup internal memory zones */
  sw_trace("draw_init");
  draw_init();

  /* SWIRL: never start a style this disc cannot run (hold Y at start up to force SWIRL) */
  settings_boot_guard();

  /* SWIRL: the VMU logo intro plays while the style loads its pictures. The settings file has been read by now,
     and the intro stops before SWIRL.DAT is touched; the Classic styles draw their own VMU screen. */
  if (settings_get()->ui == UI_SWIRL) {
    sw_trace("VMU logo intro");
    sw_vmu_boot_start();
  }

  /* Load UI */
  reload_ui();
  apply_pending_reload();

  return ret;
}

static void draw(void) {
  pvr_wait_ready();
  pvr_scene_begin();

  draw_set_list(PVR_LIST_OP_POLY);
  pvr_list_begin(PVR_LIST_OP_POLY);

  (*current_ui_draw_OP)();

  pvr_list_finish();

  draw_set_list(PVR_LIST_TR_POLY);
  pvr_list_begin(PVR_LIST_TR_POLY);

  (*current_ui_draw_TR)();

  pvr_list_finish();

  pvr_scene_finish();
}

/* one frame, for SWIRL to keep the screen moving while a memory card save finishes */
void main_draw_frame(void);
void main_draw_frame(void) {
  z_reset();
  draw();
  sw_trace_alive();
}

static void processInput(void) {
  inputs _input;
  unsigned int buttons;

  maple_device_t *cont;
  cont_state_t *state;

  cont = maple_enum_type(0, MAPLE_FUNC_CONTROLLER);
  if (!cont)
    return;
  state = (cont_state_t *)maple_dev_status(cont);

  buttons = state->buttons;

  /*  Reset Everything */
  memset(&_input, 0, sizeof(inputs));

  /* DPAD */
  _input.dpad = (state->buttons >> 4) & ~240;  // mrneo240 ;)

  /* BUTTONS */
  _input.btn_a = (uint8_t) !!(buttons & CONT_A);
  _input.btn_b = (uint8_t) !!(buttons & CONT_B);
  _input.btn_x = (uint8_t) !!(buttons & CONT_X);
  _input.btn_y = (uint8_t) !!(buttons & CONT_Y);
  _input.btn_start = (uint8_t) !!(buttons & CONT_START);

  /* ANALOG */
  _input.axes_1 = ((uint8_t)(state->joyx) + 128);
  _input.axes_2 = ((uint8_t)(state->joyy) + 128);

  /* TRIGGERS */
  _input.trg_left = (uint8_t)state->ltrig & 255;
  _input.trg_right = (uint8_t)state->rtrig & 255;

  INPT_ReceiveFromHost(_input);
}

static int translate_input(void) {
  processInput();
  if (INPT_DPADDirection(DPAD_LEFT)) {
    return LEFT;
  }
  if (INPT_DPADDirection(DPAD_RIGHT)) {
    return RIGHT;
  }
  if (INPT_DPADDirection(DPAD_UP)) {
    return UP;
  }
  if (INPT_DPADDirection(DPAD_DOWN)) {
    return DOWN;
  }

  if (INPT_AnalogI(AXES_X) < 128 - 24) {
    return LEFT;
  }
  if (INPT_AnalogI(AXES_X) > 128 + 24) {
    return RIGHT;
  }

  if (INPT_AnalogI(AXES_Y) < 128 - 24) {
    return UP;
  }
  if (INPT_AnalogI(AXES_Y) > 128 + 24) {
    return DOWN;
  }

  if (INPT_Button(BTN_A)) {
    return A;
  }
  if (INPT_Button(BTN_B)) {
    return B;
  }
  if (INPT_Button(BTN_X)) {
    return X;
  }
  if (INPT_Button(BTN_Y)) {
    return Y;
  }
  if (INPT_Button(BTN_START)) {
    return START;
  }

  /* Triggers */
  if (INPT_TriggerPressed(TRIGGER_L)) {
    return TRIG_L;
  }
  if (INPT_TriggerPressed(TRIGGER_R)) {
    return TRIG_R;
  }

  return NONE;
}

/* SWIRL picture quality, read from SWIRL.DAT on the VMU before the screen is set up:
   High = 32 bit frame buffer (smooth gradients) and horizontal anti-aliasing */
extern int sw_lib_early_quality(void);
extern float om_xscale;
void sw_fix_fsaa_clip(void);

static void init_gfx_pvr(void) {
  /* BlueCrab (c) 2014,
    This assumes that the video mode is initialized as KOS
   normally does, that is to 640x480 NTSC IL or 640x480 VGA */
  int dc_region, ct;

  dc_region = flashrom_get_region();
  ct = vid_check_cable();

  /* Prompt the user for whether to run in PAL50 or PAL60 if the flashrom says
       the Dreamcast is European and a VGA Box is not hooked up. */
  sw_trace("SWIRL.DAT picture quality (VMU)");
  int hq = sw_lib_early_quality();
#ifdef SW_FORCE_STD
  hq = 0;
#endif
  sw_trace("video: %s, cable %d, region %d", hq ? "high" : "standard", ct, dc_region);
  const int pm = hq ? PM_RGB888P : PM_RGB565;
  int dm = DM_640x480;
  if (dc_region == FLASHROM_REGION_EUROPE && ct != CT_VGA) {
    vid_set_mode(dm = DM_640x480_NTSC_IL, pm);
  } else if (hq) {
    vid_set_mode(DM_640x480, pm); /* VGA or TV, whichever cable is plugged in */
  }

  pvr_init_params_t params = {
      /* Opaque and translucent lists only */
      /* with anti-aliasing each tile covers half the width, so the opaque list (only the backdrop) needs less */
      .opb_sizes = {hq ? PVR_BINSIZE_16 : PVR_BINSIZE_32, PVR_BINSIZE_0, PVR_BINSIZE_32, PVR_BINSIZE_0, PVR_BINSIZE_0},
      .vertex_buf_size = 512 * 1024, /* SWIRL: dashboard draws more geometry than the classic views */
      .dma_enabled = 0,
      .fsaa_enabled = hq,
      .autosort_disabled = 0,
      .opb_overflow_count = hq ? 2 : 3, /* SWIRL: many layered translucent polygons per tile */
  };

  pvr_init(&params);
#ifdef SWIRL_TRACE_SCREEN
  /* diagnostic build: show the plain frame buffer again (the graphics chip takes over at the first picture).
     Not with anti-aliasing: setting the video mode again undoes what the graphics chip set up for it. */
  if (!hq) {
    vid_set_mode(dm, pm);
    sw_trace_redraw();
  }
#else
  (void)dm;
#endif
  sw_trace("graphics chip ready");
  if (hq) {
    om_xscale = 2.f;
    sw_fix_fsaa_clip();
  }
  draw_set_list(PVR_LIST_OP_POLY);
}

/* SWIRL: KallistiOS waits at start up until every device on every controller port has answered. A memory
   card that the controller reports but that never answers (a VM2 or VMU Pro busy switching cards, or a faulty
   VMU) makes that wait endless: a black screen with the VMU logo showing. This replaces KallistiOS's wait with
   one that gives up after a while (the linker's --wrap sends KallistiOS's call here). A device that turns up
   later is picked up by KallistiOS's normal hot plug scan. */
#define SWIRL_SCAN_WAIT_MS 1500
#ifndef SWIRL_VERSION_STR
#define SWIRL_VERSION_STR "2.13.2"
#endif
void __wrap_maple_wait_scan(void); /* linked in place of KallistiOS's maple_wait_scan (see Makefile) */
void __wrap_maple_wait_scan(void) {
  const uint64_t start = timer_ms_gettime64();
  while (maple_state.scan_ready_mask != 0xf) {
    if (timer_ms_gettime64() - start > SWIRL_SCAN_WAIT_MS) {
      sw_warn(SW_WARN_SCAN, "controller scan incomplete after %d ms (ports %x)", SWIRL_SCAN_WAIT_MS, maple_state.scan_ready_mask);
      return;
    }
    thd_pass();
  }
  sw_trace("controller scan done in %u ms", (unsigned)(timer_ms_gettime64() - start));
}

static void trace_devices(void) {
  for (int p = 0; p < MAPLE_PORT_COUNT; p++)
    for (int u = 0; u < MAPLE_UNIT_COUNT; u++) {
      maple_device_t *d = maple_enum_dev(p, u);
      if (d)
        sw_trace("  %c%d %.20s %08lx", 'A' + p, u, d->info.product_name, (unsigned long)d->info.functions);
    }
}

/* SWIRL: a memory card the start up scan missed (a VM2 still switching, a slow VMU) is looked for during the
   first seconds after the menu is up, and its files are read as if they had been there at start up. */
#define SWIRL_LATE_CARD_MS 8000
static void late_card_check(void) {
  int changed = sw_lib_late_card(); /* SWIRL.DAT first: its flag decides whether the CFG's style stands */
  if (settings_late_card())
    changed = 1;
  if (!changed)
    return;
  if ((int)settings_get()->ui != ui_choice_current) {
    sw_trace("style %d from the late card", settings_get()->ui);
    if (ui_choice_current == UI_SWIRL)
      ui_swirl_leave();
    reload_ui();
  } else if (ui_choice_current == UI_SWIRL) {
    ui_swirl_settings_changed();
  }
}

/* SWIRL: X held at power on asks for the boot log (sampled with the Y check in settings_boot_guard) */
static int boot_x;
void main_note_boot_x(int held) { boot_x = held; }

int main(int argc, char *argv[]) {
  /* unused */
  (void)argc;
  (void)argv;
  //extern void gdb_init();
  //gdb_init();

  fflush(stdout);
  setbuf(stdout, NULL);
  sw_trace_init();
  sw_trace("SWIRL " SWIRL_VERSION_STR " starting");
  trace_devices();
  init_gfx_pvr();

  if (init())
    sw_trace("start up had errors (carrying on)");
  sw_trace_done();
  if (boot_x) {
    /* X held at power on: the boot log as QR codes, before the first picture (any style) */
    sw_trace("X held at start: boot log");
    if (settings_get()->ui == UI_SWIRL)
      ui_swirl_boot_log();
    else
      sw_report_show(SW_REPORT_BOOT);
    input_latched = 1; /* the X still down is not a press for the menu */
  }
#ifdef SW_TEST_CRASH
  { void (*volatile bad)(void) = (void (*)(void))0x8c000002; bad(); } /* test only: an early crash */
#endif
#ifdef SW_TEST_HANG
  for (volatile int spin = 1; spin;) { } /* test only: a hang */
#endif
#ifdef SWIRL_TRACE_SCREEN
  sw_trace_report(); /* diagnostic build: the whole report, page by page, drawn by the graphics chip */
#endif

  /* SWIRL: a Y held since power on belongs to the style reset. It is never passed on to the menu (openMenu's
     Classic styles leave to the BIOS on Y) until it is let go, and held for about a second it switches a
     Classic style to SWIRL, in case the check at start up missed it. */
  int y_latched = 1, y_frames = 0;
  const uint64_t menu_up = timer_ms_gettime64();
  int late_window_over = 0;
  for (int frame = 0;; frame++) {
    z_reset();
    apply_pending_reload();
    if (frame % 30 == 15 && !late_window_over) {
      if (timer_ms_gettime64() - menu_up < SWIRL_LATE_CARD_MS) {
        late_card_check();
      } else {
        /* no card brought a settings file: openMenu wrote a fresh one at start up, SWIRL writes it now */
        late_window_over = 1;
        if (settings_cfg_missing()) {
          sw_trace("OPENMENU.CFG: none found, writing one");
          if (ui_choice_current == UI_SWIRL)
            ui_swirl_save_settings_soon();
          else
            settings_save();
        }
      }
    }
    enum control input = translate_input(); /* also reads the controller for INPT_Button below */
    if (input_latched) {
      if (input == NONE)
        input_latched = 0;
      else
        input = NONE;
    }
    const int y_now = INPT_Button(BTN_Y);
    if (y_latched && !y_now)
      y_latched = 0;
    if (y_latched) {
      if (input == Y)
        input = NONE;
      if (++y_frames == 45 && settings_get()->ui != UI_SWIRL) {
        settings_force_swirl();
        reload_ui();
        continue;
      }
    }
#ifdef SWIRL_TRACE_SCREEN
    {
      /* diagnostic build: both triggers fully down with X shows the log so far (for what happened after start
         up, like saves) */
      maple_device_t *c = maple_enum_type(0, MAPLE_FUNC_CONTROLLER);
      cont_state_t *st = c ? (cont_state_t *)maple_dev_status(c) : NULL;
      if (st && st->ltrig > 200 && st->rtrig > 200 && (st->buttons & CONT_X)) {
        sw_trace("log shown on request");
        sw_trace_report();
        continue;
      }
    }
#endif
    (*current_ui_handle_input)(input);
    apply_pending_reload();
    draw();
    sw_trace_alive();
    if (frame == 0)
      sw_trace("first picture drawn");
    if (frame == 60 || frame == 600)
      sw_trace("%d pictures drawn", frame);
  }

  return 0;
}

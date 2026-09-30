#pragma once
#include "sw_codes.h"
/* SWIRL start up trace. Every step of start up is recorded with its time, so a start that hangs or stops can be
   pinned to one step. With SWIRL_TRACE_SCREEN (the diagnostic build) the steps are also written on the screen as
   they happen, and SWIRL stops on a report screen instead of returning to the Dreamcast BIOS. */

/* at the start of main: catches asserts, clears the screen for the diagnostic build */
void sw_trace_init(void);
/* draws the steps again (after the video mode changed) */
void sw_trace_redraw(void);
/* the menu drew a picture (progress, for the hang watchdog in every build) */
void sw_trace_alive(void);
/* a known long wait starts (a launch, the BIOS): the watchdog stands down for ms */
void sw_watchdog_expect(unsigned ms);
void sw_trace(const char *fmt, ...) __attribute__((format(printf, 1, 2)));
/* the steps so far, one per line */
const char *sw_trace_text(void);
/* start up finished: the diagnostic build shows the report until A is pressed */
void sw_trace_done(void);
/* diagnostic build: the whole report, page by page, until A is pressed on the last page */
void sw_trace_report(void);
/* stops SWIRL on a report screen (the diagnostic build) or returns to the BIOS (normal build) */
void sw_trace_fatal(const char *why) __attribute__((noreturn));
/* checks the memory allocator's lists (a crash here means earlier damage); NULL: no trace line */
void sw_mem_check(const char *when);

/* ---------- warnings and reports (docs/DIAGNOSTICS.md) ---------- */
/* a problem that did not stop the menu: a code from sw_codes.h with a detail; counted, traced, reported */
void sw_warn(int code, const char *fmt, ...) __attribute__((format(printf, 2, 3)));
int sw_warn_count(void);
int sw_warn_get(int i, int *code, int *count, const char **detail);
const char *sw_code_words(int code);
/* how many games the library has (for the report header) */
void sw_trace_games(int n);
/* the short git hash this menu was built from ("dev" outside swirl/build.sh) */
const char *sw_build_id(void);
/* X is down on a controller (the boot log request) */
int sw_trace_x_held(void);
/* why a report is shown: its R number in the first line */
enum {
  SW_REPORT_CRASH = 1, SW_REPORT_HANG, SW_REPORT_USER, SW_REPORT_BOOT, SW_REPORT_ASSERT, SW_REPORT_ABORT,
  SW_REPORT_LAUNCH
};
/* the report as QR codes, from the menu: A turns the page, B comes back. The caller stops its music first. */
void sw_report_show(int reason);
/* stops SWIRL on the report screen with this reason */
void sw_trace_fatal_reason(int reason, const char *why) __attribute__((noreturn));

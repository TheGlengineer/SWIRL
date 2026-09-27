#pragma once
/* SWIRL start up trace. Every step of start up is recorded with its time, so a start that hangs or stops can be
   pinned to one step. With SWIRL_TRACE_SCREEN (the diagnostic build) the steps are also written on the screen as
   they happen, and SWIRL stops on a report screen instead of returning to the Dreamcast BIOS. */

/* at the start of main: catches asserts, clears the screen for the diagnostic build */
void sw_trace_init(void);
/* draws the steps again (after the video mode changed) */
void sw_trace_redraw(void);
/* the menu drew a picture (for the diagnostic build's hang detector) */
void sw_trace_alive(void);
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

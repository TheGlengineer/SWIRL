/*
 * SWIRL dashboard UI for openMenu.
 */
#pragma once

#define UI_NAME SWIRL

#define FUNC_NAME(name, func) name##_##func

#define MAKE_FN(name, func) void name##_##func(void)
#define FUNCTION(signal, func) MAKE_FN(signal, func)

#define MAKE_FN_INPUT(name, func) void name##_##func(unsigned int button)
#define FUNCTION_INPUT(signal, func) MAKE_FN_INPUT(signal, func)

FUNCTION(UI_NAME, init);
FUNCTION(UI_NAME, setup);
FUNCTION_INPUT(UI_NAME, handle_input);
FUNCTION(UI_NAME, drawOP);
FUNCTION(UI_NAME, drawTR);

/* SWIRL: a memory card that attached after start up brought settings; the views and colours follow them */
void ui_swirl_settings_changed(void);
/* SWIRL: another style is about to start (its own music and VMU screen) */
void ui_swirl_leave(void);
/* SWIRL: openMenu's settings file needs writing; the save banner counts down to it */
void ui_swirl_save_settings_soon(void);

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

/* SWIRL: menu music (BGM.ADP on the menu disc) and navigation sounds */
#pragma once

enum { SW_SFX_MOVE = 0, SW_SFX_TAB, SW_SFX_SELECT, SW_SFX_BACK, SW_SFX_FAV, SW_SFX_SURPRISE, SW_SFX_COUNT };

void sw_audio_init(void);
int sw_audio_has_music(void);
int sw_audio_settled(void);
void sw_audio_settings(int music_on, int music_level, int sfx_on, int sfx_level); /* levels 0..10 */
void sw_audio_poll(void);     /* once per frame */
void sw_audio_sfx(int which);
void sw_audio_fade_out(void); /* start a quick fade before launching */
int sw_audio_quiet(void);     /* the fade has finished (or nothing is playing) */
void sw_audio_shutdown(void); /* before launching anything */

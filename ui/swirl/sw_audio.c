/*
 * SWIRL: menu music and navigation sounds.
 *
 * Music streams /cd/BGM.ADP: a 32 byte "OMBG" header followed by AICA ADPCM, the format the maintained
 * openMenu fork uses (github.com/DerekPascarella/openMenu-Virtual-Folder-Bundle, backend/bgm.c, BSD).
 * SWIRL Card Manager converts the owner's own music file into it. The streaming approach below follows
 * that fork: a large ring buffer in main RAM so box art loading can use the drive without the music
 * stuttering.
 *
 * Navigation sounds are synthesised at start up (short soft blips), so no extra files are needed.
 */
#include "sw_audio.h"

#include <dc/g2bus.h>
#include <dc/sound/sfxmgr.h>
#include <dc/sound/sound.h>
#include <dc/sound/stream.h>
#include <dc/spu.h>
#include <kos.h>
#include <math.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#define BGM_FILE "/cd/BGM.ADP"
#define BGM_HEADER_LEN 32
#define BGM_RING_SIZE (512 << 10)
#define BGM_READ_CHUNK (64 << 10)
#define BGM_STAGING_SIZE (32 << 10)

#define AICA_REG(x) (0xa0700000 + (x))
#define AICA_TIMER_A AICA_REG(0x2890)
#define AICA_SCIEB AICA_REG(0x289c)
#define AICA_SCIRE AICA_REG(0x28a4)
#define AICA_SCILV0 AICA_REG(0x28a8)
#define AICA_SCILV1 AICA_REG(0x28ac)
#define AICA_SCILV2 AICA_REG(0x28b0)
#define AICA_MCIEB AICA_REG(0x28b4)
#define AICA_MCIRE AICA_REG(0x28bc)

static int snd_ready;
static int master_tick, fading_out;
static int bgm_ok, playing;
static file_t bgm_fd = FILEHND_INVALID;
static snd_stream_hnd_t stream_hnd = SND_STREAM_INVALID;
static uint32_t sample_rate;
static int stereo;
static int want_music, music_vol = 180;
/* fades: every volume change is ramped, because a sudden start or stop of the waveform is heard as a pop */
static int fade = 0, fade_target = 256, fade_step = 5;
static int master = 0; /* AICA master volume 0..15, raised gently after start up */
static uint8_t *ring;
static uint32_t ring_head, ring_tail, ring_level;
static uint8_t staging[BGM_STAGING_SIZE] __attribute__((aligned(32)));

static sfxhnd_t sfx[SW_SFX_COUNT];
static int sfx_on = 1, sfx_vol = 150;

static void arm_aica_timer(void) {
  /* the sound driver needs an AICA timer interrupt it never arms itself; after the BIOS it is gone */
  g2_fifo_wait();
  g2_write_32(AICA_SCIEB, 0);
  g2_write_32(AICA_MCIEB, 0);
  g2_write_32(AICA_SCIRE, 0x7ff);
  g2_write_32(AICA_MCIRE, 0x7ff);
  g2_fifo_wait();
  g2_write_32(AICA_SCILV0, 0x18);
  g2_write_32(AICA_SCILV1, 0x50);
  g2_write_32(AICA_SCILV2, 0x08);
  g2_fifo_wait();
  g2_write_32(AICA_TIMER_A, 256 - (44100 / 4410));
  g2_write_32(AICA_SCIEB, 0x40);
}

static int ensure_sound(void) {
  if (snd_ready)
    return 1;
  /* silent before the driver loads (the resets themselves are made quiet in sw_spu.c), then bring the
   * master volume up over the first half second */
  master = 0;
  spu_master_mixer(0, 1);
  if (snd_stream_init() < 0)
    return 0;
  spu_master_mixer(0, 1); /* the driver sets full volume when it starts */
  arm_aica_timer();
  snd_ready = 1;
  return 1;
}

/* ---------- navigation sounds ---------- */
static sfxhnd_t synth(float f0, float f1, int ms, float level) {
  const int rate = 22050;
  int n = rate * ms / 1000;
  int16_t *buf = memalign(32, n * 2 + 64);
  if (!buf)
    return SFXHND_INVALID;
  float phase = 0.f;
  for (int i = 0; i < n; i++) {
    float t = (float)i / n;
    float f = f0 + (f1 - f0) * t;
    phase += 2.f * 3.14159265f * f / rate;
    float env = (i < rate / 500 ? (float)i / (rate / 500) : 1.f) * (1.f - t) * (1.f - t);
    float v = sinf(phase) * 0.8f + sinf(phase * 2.f) * 0.2f;
    buf[i] = (int16_t)(v * env * level * 32767.f);
  }
  sfxhnd_t h = snd_sfx_load_raw_buf((char *)buf, n * 2, rate, 16, 1);
  free(buf);
  return h;
}

void sw_audio_init(void) {
  if (snd_ready)
    return; /* already running */
  if (!ensure_sound())
    return;
  sfx[SW_SFX_MOVE] = synth(1320.f, 1180.f, 28, 0.35f);
  sfx[SW_SFX_TAB] = synth(880.f, 990.f, 45, 0.40f);
  sfx[SW_SFX_SELECT] = synth(660.f, 1320.f, 90, 0.55f);
  sfx[SW_SFX_BACK] = synth(900.f, 520.f, 80, 0.45f);
  sfx[SW_SFX_FAV] = synth(1040.f, 1560.f, 120, 0.45f);
  sfx[SW_SFX_SURPRISE] = synth(520.f, 1560.f, 260, 0.45f);

  if (!ring)
    ring = memalign(32, BGM_RING_SIZE);
  ring_head = ring_tail = ring_level = 0;
  bgm_fd = fs_open(BGM_FILE, O_RDONLY);
  if (bgm_fd == FILEHND_INVALID || !ring)
    return;
  uint8_t h[BGM_HEADER_LEN];
  if (fs_read(bgm_fd, h, sizeof(h)) != (ssize_t)sizeof(h) || memcmp(h, "OMBG", 4)) {
    fs_close(bgm_fd);
    bgm_fd = FILEHND_INVALID;
    return;
  }
  uint32_t version = h[4] | h[5] << 8 | h[6] << 16 | (uint32_t)h[7] << 24;
  sample_rate = h[8] | h[9] << 8 | h[10] << 16 | (uint32_t)h[11] << 24;
  int channels = h[12] | h[13] << 8;
  if (version != 1 || sample_rate < 8000 || sample_rate > 44100 || (channels != 1 && channels != 2) ||
      fs_total(bgm_fd) <= BGM_HEADER_LEN) {
    fs_close(bgm_fd);
    bgm_fd = FILEHND_INVALID;
    return;
  }
  fs_seek(bgm_fd, BGM_HEADER_LEN, SEEK_SET);
  stereo = channels == 2;
  bgm_ok = 1;
}

/* true once the music has started (or there is none to wait for) */
int sw_audio_settled(void) {
  return !snd_ready || !bgm_ok || !want_music || music_vol == 0 || playing;
}

int sw_audio_has_music(void) {
  return bgm_ok;
}

void sw_audio_sfx(int which) {
  if (!snd_ready || !sfx_on || which < 0 || which >= SW_SFX_COUNT || sfx[which] == SFXHND_INVALID)
    return;
  snd_sfx_play(sfx[which], sfx_vol, 128);
}

/* ---------- music ---------- */
static void fill_chunk(void) {
  if (BGM_RING_SIZE - ring_level < BGM_READ_CHUNK)
    return;
  uint32_t run = BGM_RING_SIZE - ring_head;
  if (run > BGM_READ_CHUNK)
    run = BGM_READ_CHUNK;
  ssize_t got = fs_read(bgm_fd, ring + ring_head, run);
  if (got == 0) {
    fs_seek(bgm_fd, BGM_HEADER_LEN, SEEK_SET); /* loop */
    return;
  }
  if (got < 0)
    return;
  ring_head = (ring_head + (uint32_t)got) % BGM_RING_SIZE;
  ring_level += (uint32_t)got;
}

static void *stream_cb(snd_stream_hnd_t hnd, int req, int *recv) {
  (void)hnd;
  if (req > (int)sizeof(staging))
    req = sizeof(staging);
  uint32_t have = ring_level < (uint32_t)req ? ring_level : (uint32_t)req;
  uint32_t first = BGM_RING_SIZE - ring_tail;
  if (first > have)
    first = have;
  memcpy(staging, ring + ring_tail, first);
  memcpy(staging + first, ring, have - first);
  ring_tail = (ring_tail + have) % BGM_RING_SIZE;
  ring_level -= have;
  if (have < (uint32_t)req)
    memset(staging + have, 0, req - have);
  *recv = req;
  return staging;
}

static void apply_volume(void) {
  if (stream_hnd != SND_STREAM_INVALID)
    snd_stream_volume(stream_hnd, music_vol * fade / 256);
}

static void start_music(void) {
  if (stream_hnd == SND_STREAM_INVALID) {
    stream_hnd = snd_stream_alloc(stream_cb, BGM_STAGING_SIZE);
    if (stream_hnd == SND_STREAM_INVALID) {
      bgm_ok = 0;
      return;
    }
  }
  for (int i = 0; i < 8 && ring_level < 2 * BGM_READ_CHUNK; i++)
    fill_chunk();
  fade = 0;
  fade_target = 256;
  fade_step = 4; /* about a second to full volume */
  snd_stream_start_adpcm(stream_hnd, sample_rate, stereo);
  apply_volume();
  playing = 1;
}

static void stop_music(void) {
  snd_stream_stop(stream_hnd);
  ring_head = ring_tail = ring_level = 0;
  fs_seek(bgm_fd, BGM_HEADER_LEN, SEEK_SET);
  playing = 0;
}

void sw_audio_settings(int music_on, int music_level, int sfx_enabled, int sfx_level) {
  want_music = music_on;
  music_vol = music_level * 255 / 10;
  sfx_on = sfx_enabled;
  sfx_vol = sfx_level * 255 / 10;
  if (playing)
    apply_volume();
}

void sw_audio_poll(void) {
  if (snd_ready && master < 15 && ++master_tick >= 2) {
    master_tick = 0;
    spu_master_mixer(++master, 1);
  }
  if (!bgm_ok)
    return;
  int want = want_music && music_vol > 0 && !fading_out;
  if (want && !playing)
    start_music();
  else if (!want && playing && !fading_out) {
    /* turned off in System: fade, then stop */
    fade_target = 0;
    fade_step = 12;
    fading_out = 1;
  }
  if (!playing)
    return;
  if (fade != fade_target) {
    fade += fade < fade_target ? fade_step : -fade_step;
    if (fade > 256) fade = 256;
    if (fade < 0) fade = 0;
    if (fade_target == 0 && fade < fade_step) fade = 0;
    apply_volume();
  }
  if (fading_out && fade == 0) {
    stop_music();
    fading_out = 0;
    if (!want_music || music_vol == 0) {
      /* stays stopped until turned back on */
    }
    return;
  }
  fill_chunk();
  snd_stream_poll(stream_hnd);
}

void sw_audio_fade_out(void) {
  if (!playing)
    return;
  fade_target = 0;
  fade_step = 16; /* about a quarter of a second */
  fading_out = 1;
}

int sw_audio_quiet(void) {
  return !playing || fade == 0;
}

void sw_audio_shutdown(void) {
  if (!snd_ready)
    return;
  /* silence first, then stop, so nothing is cut off mid-wave */
  if (playing) {
    fade = 0;
    apply_volume();
    snd_stream_stop(stream_hnd);
  }
  snd_sfx_stop_all();
  spu_master_mixer(0, 1);
  if (stream_hnd != SND_STREAM_INVALID) {
    snd_stream_destroy(stream_hnd);
    stream_hnd = SND_STREAM_INVALID;
  }
  snd_sfx_unload_all();
  for (int i = 0; i < SW_SFX_COUNT; i++) sfx[i] = SFXHND_INVALID;
  snd_stream_shutdown();
  spu_master_mixer(0, 1); /* leave the chip muted for whatever runs next */
  if (bgm_fd != FILEHND_INVALID) {
    fs_close(bgm_fd);
    bgm_fd = FILEHND_INVALID;
  }
  bgm_ok = 0;
  snd_ready = 0;
  playing = 0;
  fading_out = 0;
}

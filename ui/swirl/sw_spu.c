/*
 * SWIRL: pop-free sound chip resets.
 *
 * KallistiOS resets the AICA sound chip at three moments: when the kernel starts (spu_init), when the
 * sound driver loads (snd_init: spu_disable, clear, spu_enable) and when handing over to a game
 * (arch_exec: spu_shutdown). Each reset cuts every channel off instantly and sets the master volume
 * straight to full (spu_reset_chans), and the kernel start also clears sound RAM while the DSP effects
 * the BIOS set up may still be reading it. Any of those can be heard as a click or pop on a TV.
 *
 * The linker sends those calls here instead (-Wl,--wrap=..., see the Makefile): the master volume is
 * first stepped down to silence, the DSP effect outputs are closed, and only then are channels cut and
 * memory cleared. SWIRL raises the volume again gently once its own sound is running (sw_audio.c).
 */
#include <arch/timer.h>
#include <dc/g2bus.h>
#include <dc/spu.h>
#include <stdint.h>

#define SNDREG(x) (0xa0700000 + (x))
#define CHNREG(ch, x) SNDREG(0x80 * (ch) + (x))

int __real_spu_init(void);
int __real_spu_shutdown(void);
void __real_spu_enable(void);
void __real_spu_disable(void);

/* step the master volume down to 0, a notch every 2 ms */
static void master_down(void) {
  g2_fifo_wait();
  uint32_t reg = g2_read_32(SNDREG(0x2800));
  int mv = (int)(reg & 0xf);
  uint32_t mono = reg & 0x8000;
  while (mv > 0) {
    mv--;
    g2_fifo_wait();
    g2_write_32(SNDREG(0x2800), mono | (uint32_t)mv);
    timer_spin_sleep(2);
  }
}

static void master_set(int v) {
  g2_fifo_wait();
  g2_write_32(SNDREG(0x2800), (uint32_t)(v & 0xf));
}

/* close the 16 DSP effect outputs (reverb and the like left running by the BIOS) */
static void dsp_outputs_off(void) {
  for (int i = 0; i < 16; i++) {
    if ((i & 3) == 0)
      g2_fifo_wait();
    g2_write_32(SNDREG(0x2000 + 4 * i), 0);
  }
}

/* stop every channel without touching the master volume */
static void channels_off(void) {
  for (int i = 0; i < 64; i++) {
    if ((i & 3) == 0)
      g2_fifo_wait();
    g2_write_32(CHNREG(i, 0), 0x8000);
    g2_write_32(CHNREG(i, 20), 0x1f);
  }
}

static void arm_stop(void) {
  g2_fifo_wait();
  g2_write_32(SNDREG(0x2c00), g2_read_32(SNDREG(0x2c00)) | 1);
}

static void arm_start(void) {
  g2_fifo_wait();
  g2_write_32(SNDREG(0x2c00), g2_read_32(SNDREG(0x2c00)) & ~1u);
}

void __wrap_spu_disable(void) {
  master_down();
  arm_stop();
  channels_off();
}

void __wrap_spu_enable(void) {
  channels_off();
  arm_start(); /* the KallistiOS sound driver sets its own master volume; SWIRL lowers it again at once */
}

int __wrap_spu_init(void) {
  master_down();
  dsp_outputs_off();
  arm_stop();
  channels_off();
  spu_memset_sq(0, 0, 0x200000);
  /* the same idle program the kernel loads, so CD audio keeps working */
  g2_fifo_wait();
  g2_write_32(SPU_RAM_UNCACHED_BASE, 0xeafffff8);
  arm_start();
  timer_spin_sleep(10);
  spu_cdda_volume(15, 15);
  spu_cdda_pan(0, 31);
  return 0; /* master volume stays at 0 until SWIRL's sound is up */
}

int __wrap_spu_shutdown(void) {
  master_down();
  arm_stop();
  channels_off();
  spu_memset_sq(0, 0, 0x200000);
  /* leave the volume where games expect it; everything is silent, so nothing is heard */
  master_set(15);
  return 0;
}

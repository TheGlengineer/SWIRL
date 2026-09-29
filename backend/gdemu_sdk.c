#include <kos.h>

#define ATA_SR_BSY 0x80
#define ATA_SR_DRDY 0x40
#define ATA_SR_DF 0x20
#define ATA_SR_DSC 0x10
#define ATA_SR_DRQ 0x08
#define ATA_SR_CORR 0x04
#define ATA_SR_IDX 0x02
#define ATA_SR_ERR 0x01

#define ATA_ER_BBK 0x80
#define ATA_ER_UNC 0x40
#define ATA_ER_MC 0x20
#define ATA_ER_IDNF 0x10
#define ATA_ER_MCR 0x08
#define ATA_ER_ABRT 0x04
#define ATA_ER_TK0NF 0x02
#define ATA_ER_AMNF 0x01

#define ATAPI_CMD_PACKET 0xA0

/* ATA-related registers. Some of these serve very different purposes when read
   than they do when written (hence why some addresses are duplicated). */
#define G1_ATA_ALTSTATUS 0xA05F7018     /* Read */
#define G1_ATA_CTRL 0xA05F7018          /* Write */
#define G1_ATA_DATA 0xA05F7080          /* Read/Write */
#define G1_ATA_ERROR 0xA05F7084         /* Read */
#define G1_ATA_FEATURES 0xA05F7084      /* Write */
#define G1_ATA_IRQ_REASON 0xA05F7088    /* Read */
#define G1_ATA_SECTOR_COUNT 0xA05F7088  /* Write */
#define G1_ATA_LBA_LOW 0xA05F708C       /* Read/Write */
#define G1_ATA_LBA_MID 0xA05F7090       /* Read/Write */
#define G1_ATA_LBA_HIGH 0xA05F7094      /* Read/Write */
#define G1_ATA_DEVICE_SELECT 0xA05F7098 /* Read/Write */
#define G1_ATA_STATUS_REG 0xA05F709C    /* Read */
#define G1_ATA_COMMAND_REG 0xA05F709C   /* Write */

/* status of the external interrupts
 * bit 3 = External Device interrupt
 * bit 2 = Modem interrupt
 * bit 1 = AICA interrupt
 * bit 0 = GD-ROM interrupt */
#define EXT_INT_STAT 0xA05F6904 /* Read */

/* Macros to access the ATA registers */
#define OUT16(addr, data) *((volatile uint16_t *)addr) = data
#define OUT8(addr, data) *((volatile uint8_t *)addr) = data
#define IN32(addr) *((volatile uint32_t *)addr)
#define IN16(addr) *((volatile uint16_t *)addr)
#define IN8(addr) *((volatile uint8_t *)addr)

/* SWIRL: every wait has a limit. openMenu's loops spun for ever, so a drive that did not answer (a clone board
   busy switching images, a wedged drive) froze the console with a blank screen. The GD-ROM lock is held while
   a packet is exchanged, so KallistiOS's own drive status poll (it runs every screen refresh) cannot slip in
   between the command bytes and the reply. A timed out exchange returns -1; the callers then say so instead
   of handing the console to the loader. */
#include <arch/timer.h>
#include <dc/g1ata.h>

#define G1_WAIT_MS 3000

static int wait_status_clear(uint8_t mask) {
  const uint64_t until = timer_ms_gettime64() + G1_WAIT_MS;
  while (IN8(G1_ATA_ALTSTATUS) & mask)
    if (timer_ms_gettime64() > until) return -1;
  return 0;
}

static int wait_status_set(uint8_t mask) {
  const uint64_t until = timer_ms_gettime64() + G1_WAIT_MS;
  while (!(IN8(G1_ATA_ALTSTATUS) & mask))
    if (timer_ms_gettime64() > until) return -1;
  return 0;
}

static int wait_interrupt(void) {
  const uint64_t until = timer_ms_gettime64() + G1_WAIT_MS;
  while (!(IN32(EXT_INT_STAT) & 1))
    if (timer_ms_gettime64() > until) return -1;
  return 0;
}

#define g1_ata_wait_interrupt() do { if (wait_interrupt() < 0) return -1; } while (0)
#define g1_ata_wait_drq() do { if (wait_status_set(ATA_SR_DRQ) < 0) return -1; } while (0)
#define g1_ata_wait_bsydrq() do { if (wait_status_clear(ATA_SR_DRQ | ATA_SR_BSY) < 0) return -1; } while (0)

static int send_packet_command_locked(uint16_t *cmd_buff) {
  g1_ata_wait_bsydrq();
  OUT8(G1_ATA_COMMAND_REG, ATAPI_CMD_PACKET);
  g1_ata_wait_drq();

  (void)IN32(G1_ATA_STATUS_REG);
  int i;
  for (i = 0; i < 6; i++) {
    OUT16(G1_ATA_DATA, cmd_buff[i]);
  }

  g1_ata_wait_interrupt();

  (void)IN32(G1_ATA_STATUS_REG);

  return (IN8(G1_ATA_ALTSTATUS) & ATA_SR_ERR);
}

static int send_packet_data_command_locked(uint16_t *cmd_buff, uint16_t *buffer, uint32_t *size) {
  g1_ata_wait_bsydrq();

  OUT8(G1_ATA_FEATURES, 0);
  OUT8(G1_ATA_COMMAND_REG, ATAPI_CMD_PACKET);
  g1_ata_wait_drq();

  (void)IN32(G1_ATA_STATUS_REG);
  int i;
  for (i = 0; i < 6; i++) {
    OUT16(G1_ATA_DATA, cmd_buff[i]);
  }

  g1_ata_wait_interrupt();

  if (!(IN8(G1_ATA_STATUS_REG) & ATA_SR_DRQ)) {
    if (size) {
      *size = 0;
    }

    return 0;
  }

  uint16_t len = (IN8(G1_ATA_LBA_MID) | (IN8(G1_ATA_LBA_HIGH) << 8));

  if (size) *size = (uint32_t)len;

  len >>= 1;

  (void)IN32(G1_ATA_STATUS_REG);
  /* the only caller (gdemu_get_version) hands in a small buffer: 16 words at most land in it, the rest is read
     and dropped so the drive finishes the transfer */
  for (i = 0; i < len; i++) {
    const uint16_t w = IN16(G1_ATA_DATA);
    if (i < 16) buffer[i] = w;
  }

  g1_ata_wait_interrupt();

  (void)IN32(G1_ATA_STATUS_REG);

  return (IN8(G1_ATA_ALTSTATUS) & ATA_SR_ERR);
}

static int send_packet_command(uint16_t *cmd_buff) {
  g1_ata_mutex_lock();
  const int rv = send_packet_command_locked(cmd_buff);
  g1_ata_mutex_unlock();
  return rv;
}

static int send_packet_data_command(uint16_t *cmd_buff, uint16_t *buffer, uint32_t *size) {
  g1_ata_mutex_lock();
  const int rv = send_packet_data_command_locked(cmd_buff, buffer, size);
  g1_ata_mutex_unlock();
  return rv;
}

/* return 8 byte: 00 00 09 01 00 00 14 05
 * 01 09 00 00 - internal bootloader version. don't used in early models
 * 05 14 00 00 - FW version (5.14.0)
*/

int gdemu_get_version(void *buffer, uint32_t *size) {
  uint8_t cmd_buff[12] __attribute__((aligned(4)));
  ((uint32_t *)cmd_buff)[0] = 0;
  ((uint32_t *)cmd_buff)[1] = 0;
  ((uint32_t *)cmd_buff)[2] = 0;

  cmd_buff[0] = 0x52;

  return send_packet_data_command((uint16_t *)cmd_buff, (uint16_t *)buffer, size);
}

/* param = 0x55 next img */
/* param = 0x44 prev img */
int gdemu_img_cmd(uint8_t cmd) {
  uint8_t cmd_buff[12] __attribute__((aligned(4)));
  ((uint32_t *)cmd_buff)[0] = 0;
  ((uint32_t *)cmd_buff)[1] = 0;
  ((uint32_t *)cmd_buff)[2] = 0;

  cmd_buff[0] = 0x52;
  cmd_buff[1] = 0x81;
  cmd_buff[2] = cmd;

  return send_packet_command((uint16_t *)cmd_buff);
}

/* 0 = reset to default img */
/* 1 to 999 = set image index */
int gdemu_set_img_num(uint16_t img_num) {
#ifdef SW_TEST_NO_GDEMU
  (void)img_num; /* emulator tests only: Flycast has no GDEMU, so the disc switch would never answer */
  return 0;
#endif
#ifdef SW_TEST_GDEMU_SILENT
  if (img_num != 1)
    return -1; /* emulator tests only: a drive that never answers a game switch (the menu disc still comes back) */
#endif
  uint8_t cmd_buff[12] __attribute__((aligned(4)));
  ((uint32_t *)cmd_buff)[0] = 0;
  ((uint32_t *)cmd_buff)[1] = 0;
  ((uint32_t *)cmd_buff)[2] = 0;

  cmd_buff[0] = 0x52;
  cmd_buff[1] = 0x82;
  cmd_buff[2] = (uint8_t)(img_num);
  cmd_buff[3] = (uint8_t)(img_num >> 8);

  /* only a drive that never answered is a failure (-1). The error bit of the reply is ignored on purpose:
     openMenu and GDMENU never looked at it and real drives are known to launch fine whatever it says. */
  return send_packet_command((uint16_t *)cmd_buff) < 0 ? -1 : 0;
}

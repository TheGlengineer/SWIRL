#ifndef CRAYON_SETUP_H
#define CRAYON_SETUP_H

#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

#include "savefile.h"

int16_t setup_vmu_icon_load(uint8_t ** vmu_lcd_icon, char * icon_path);
void setup_vmu_icon_apply(uint8_t * vmu_lcd_icon, uint8_t valid_vmu_screens);

#endif

#ifndef CRAYON_SAVEFILE_H
#define CRAYON_SAVEFILE_H

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h> //For the uintX_t types

#include <dc/maple/vmu.h>
#include <dc/vmu_pkg.h>
#include <dc/vmufs.h>
#include <kos/fs.h>

//The struct that contains all of a save file info. This is useful for passing
//by reference into a function and if you want to modify the save file data easily
//or even use different save files for one game
typedef struct crayon_savefile_details{
	uint8_t * savefile_data;
	size_t savefile_size;

	uint8_t * icon;
	uint16_t * icon_palette;
	uint8_t icon_anim_count;	//Decided to not go with a uint16_t (Largest VMU supports) because
								//when would you ever want more than 255 frames of animation here?
								//Also BIOS will only work with up to 3 icons
	uint16_t icon_anim_speed;	//Set to "0" for no animation (1st frame only)

	//Appears on the VMU screen on the left in the DC BIOS
	uint8_t * eyecatch_data;
	uint8_t eyecatch_type;		//PAL4BPP (4 Blocks), PAL8BPP (8.875 Blocks), plain ARGB4444 (15.75 Blocks)

	char desc_long[32];
	char desc_short[16];
	char app_id[16];
	char save_name[26];			//Name is 32 chars long max I think and its prefixed with "/vmu/XX/"

	uint8_t valid_memcards;		//Memory cards with enough space for a save file or an existing save
	uint8_t valid_saves;		//All VMUs with a savefile
	uint8_t valid_vmu_screens;	//All VMU screens (Only applies for DC)

	int8_t savefile_port;
	int8_t savefile_slot;
} crayon_savefile_details_t;


//---------------------Internal use------------------------


uint8_t crayon_savefile_check_for_save(crayon_savefile_details_t * savefile_details, int8_t savefile_port, int8_t savefile_slot);	//1 if save DNE. 0 if it does
uint8_t crayon_savefile_check_for_device(int8_t port, int8_t slot, uint32_t function);	//0 if device is valid
uint16_t crayon_savefile_bytes_to_blocks(size_t bytes);	//Takes a byte count and returns no. blocks needed to save it
int16_t crayon_savefile_get_save_block_count(crayon_savefile_details_t * savefile_details);	//Returns the number of blocks your save file will need (Uncompressed)


//---------------Both internal and external----------------


uint8_t crayon_savefile_get_vmu_bit(uint8_t vmu_bitmap, int8_t savefile_port, int8_t savefile_slot);	//Returns boolean
void crayon_savefile_set_vmu_bit(uint8_t * vmu_bitmap, int8_t savefile_port, int8_t savefile_slot);	//Updates vmu_bitmap
void crayon_savefile_clear_vmu_bit(uint8_t * vmu_bitmap, int8_t savefile_port, int8_t savefile_slot);


//------------------Called externally----------------------


void crayon_savefile_load_icon(crayon_savefile_details_t * savefile_details, char * image, char * palette);
void crayon_savefile_free_icon(crayon_savefile_details_t * savefile_details);

uint8_t crayon_savefile_load_eyecatch(crayon_savefile_details_t * savefile_details, char * eyecatch_path);	//Also sets eyecatch_type
void crayon_savefile_free_eyecatch(crayon_savefile_details_t * savefile_details);

//This will attach the var to the corresponding savefile_details vars
#define CRAY_SAVEFILE_UPDATE_MODE_MEMCARD_PRESENT (1 << 0)
#define CRAY_SAVEFILE_UPDATE_MODE_SAVE_PRESENT (1 << 1)
#define CRAY_SAVEFILE_UPDATE_MODE_BOTH CRAY_SAVEFILE_UPDATE_MODE_SAVE_PRESENT + CRAY_SAVEFILE_UPDATE_MODE_MEMCARD_PRESENT
void crayon_savefile_update_valid_saves(crayon_savefile_details_t * savefile_details, uint8_t modes);

//Returns an 8 bit var for each VMU (a1a2b1b2c1c2d1d2)
#define crayon_savefile_get_valid_screens()  crayon_savefile_get_valid_function(MAPLE_FUNC_LCD)	//A macro so you don't need to remember the function name
uint8_t crayon_savefile_get_valid_function(uint32_t function);	//Can be used to find all devices with function X

//Make sure to call this after making a new savefile struct otherwise you can get strange results
	//Also note the return value is 1 when the number of icons is greater than 3. The DC BIOS can't render icons with 4 or more frames
	//If you call this AFTER loading an eyecatcher it will override the eyecatch_type variable with zero. Call init FIRST
uint8_t crayon_savefile_init_savefile_details(crayon_savefile_details_t * savefile_details,  uint8_t * savefile_data, size_t savefile_size,
	uint8_t icon_anim_count, uint16_t icon_anim_speed, char * desc_long, char * desc_short, char * app_id, char * save_name);

//Returns 0 on success and 1 or more if failure. Handles loading and saving of uncompressed savefiles
uint8_t crayon_savefile_load(crayon_savefile_details_t * savefile_details);
uint8_t crayon_savefile_save(crayon_savefile_details_t * savefile_details);

//------------Not savefile, but VMU related---------------

void crayon_vmu_display_icon(uint8_t vmu_bitmap, void * icon);

//Add a savefile deletion function using this KOS function probably 
// int vmufs_delete(maple_device_t * dev, const char * fn)

#endif

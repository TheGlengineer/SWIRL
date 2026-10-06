/*
 * File: db_list.h
 * Project: backend
 * File Created: Wednesday, 16th June 2021 10:32:48 pm
 * Author: Hayden Kowalchuk
 * -----
 * Copyright (c) 2021 Hayden Kowalchuk, Hayden Kowalchuk
 * License: BSD 3-clause "New" or "Revised" License, http://www.opensource.org/licenses/BSD-3-Clause
 */

#pragma once

#include "db_item.h"

int db_load_DAT(void);
int db_get_meta(const char *id, struct db_item **item);
/* SWIRL 2.17: the description file of a language (META_DE.DAT ...), kept beside the English database.
   db_load_lang(NULL or "") unloads; 0 loaded (or already loaded), -1 no such file, -2 unusable (W22). */
int db_load_lang(const char *name);
void db_unload_lang(void);
const char *db_get_desc_lang(const char *id); /* the description in the loaded language, NULL when none */

const char *db_format_nplayers_str(int nplayers);
const char *db_format_vmu_blocks_str(int num_blocks);
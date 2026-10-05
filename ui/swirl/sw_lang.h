/* SWIRL: the menu's text in one table, so it can be shown in another language.

   Every string the SWIRL style shows is listed here with its English text. T(S_...) gives the string in
   the current language: the one loaded from LANG.DAT on the card (compiled by SWIRL Card Manager from
   swirl/lang/<code>.json), or the English here when there is no file, the file has no such language, or
   a string is missing or empty in it. Trace lines, file names and the openMenu styles stay English.

   Rules for a string:
   - keep printf specifiers (%d, %u, %s, %02u, %c) in the same order in every language; the Card Manager
     refuses a translation whose specifiers differ from the English
   - no "%s" plurals: a count has one string for one and one for many (S_PLAYED_ONCE, S_PLAYED_TIMES)
   - the fonts hold Latin-1 (western European letters); anything else draws as '?'
   The key order is the file order: swirl/tools/lang_table.py reads this file to write swirl/lang/en.json,
   and the Card Manager writes LANG.DAT in the same order. Adding a string at the end keeps older
   LANG.DAT files loading (the missing tail falls back to English); the table hash in the file still
   has to match, so the Card Manager rebuilds LANG.DAT whenever it updates the menu. */
#pragma once

#define SW_STRINGS(X) \
  /* the language's own name, shown in the System > Language row */ \
  X(S_LANG_NAME, "English") \
  /* tabs and footers */ \
  X(S_TAB_HOME, "Home") \
  X(S_TAB_LIBRARY, "Library") \
  X(S_TAB_COLLECTIONS, "Collections") \
  X(S_TAB_SYSTEM, "System") \
  X(S_FOOT_TABS, "L / R  Tabs") \
  X(S_FOOT_BROWSE, "D-Pad  Browse") \
  X(S_FOOT_SURPRISE, "Down  Surprise me") \
  X(S_FOOT_SETTINGS, "Start  Settings") \
  X(S_FOOT_SORT, "X  Sort") \
  X(S_FOOT_FAVORITE, "Y  Favorite") \
  X(S_FOOT_TYPE_TO_JUMP, "Keyboard  Type to jump") \
  X(S_FOOT_OPEN, "A  Open") \
  X(S_FOOT_COLLECTIONS, "Up / Down  Collections") \
  X(S_FOOT_BACK_COLLECTIONS, "B  Collections") \
  X(S_FOOT_RIGHT_GAMES, "Right  Games") \
  X(S_FOOT_CHANGE, "Left / Right  Change") \
  X(S_FOOT_SELECT, "A  Select") \
  X(S_FOOT_BACK, "B  Back") \
  X(S_FOOT_MEMORY_CARD, "L / R  Memory card") \
  X(S_FOOT_COPY, "Y  Copy") \
  X(S_FOOT_DELETE, "X  Delete") \
  X(S_FOOT_SCROLL, "Up / Down  Scroll") \
  X(S_FOOT_QR, "A  Show as QR codes") \
  X(S_POS_OF, "%d of %d") \
  X(S_RANGE_OF, "%d to %d of %d") \
  /* saving */ \
  X(S_SAVING_WAIT, "Saving... please wait") \
  X(S_SAVING, "Saving...") \
  X(S_SAVED_TO_VMU, "Saved to VMU") \
  X(S_UNSAVED_IN, "Unsaved changes. Saving in %d") \
  X(S_RETRY_IN, "%s. Trying again in %d") \
  X(S_VMU_BUSY, "VMU busy") \
  X(S_NOT_SAVED, "Not saved") \
  X(S_NOT_SAVED_NO_VMU, "Not saved: no VMU with space") \
  X(S_NOT_SAVED_NO_SPACE, "Not saved: no space on VMU") \
  X(S_NOT_SAVED_CHECK, "Not saved: check the VMU") \
  X(S_NO_VMU_SPACE_LONG, "No VMU with space. Changes not saved") \
  X(S_NO_SPACE_FREE_ONE, "No space on VMU. Free 1 block in VMU saves") \
  X(S_NO_SPACE_FREE_N, "No space on VMU. Free %d blocks in VMU saves") \
  X(S_CARD_BACK, "Memory card found: your settings are back") \
  /* home */ \
  X(S_CONTINUE_PLAYING, "Continue Playing") \
  X(S_FAVORITE, "Favorite") \
  X(S_UNFAVORITE, "Unfavorite") \
  X(S_YOUR_LIBRARY, "Your Library") \
  X(S_NO_GAMES, "No games found") \
  X(S_NO_GAMES_HELP, "Add games to your SD card with GDMENUCardManager, then rebuild the menu.") \
  X(S_SLOT_ON_CARD, "Slot %02u on your SD card.") \
  X(S_PLAY, "Play") \
  X(S_DETAILS, "Details") \
  X(S_RECENTLY_PLAYED, "Recently Played") \
  X(S_ALL_GAMES, "All Games") \
  X(S_THEN_ALL_GAMES, "then All Games %d") \
  X(S_N_GAMES, "%d games") \
  X(S_ONE_GAME, "1 game") \
  X(S_LAST_PLAYED, "Last Played %s") \
  X(S_SURPRISE_PICK, "Surprise pick. Press A to play") \
  X(S_WELCOME_BACK, "Welcome back") \
  X(S_ADDED_FAV, "Added to Favorites") \
  X(S_REMOVED_FAV, "Removed from Favorites") \
  /* chips */ \
  X(S_ONE_PLAYER, "1 Player") \
  X(S_N_PLAYERS, "1 to %d Players") \
  X(S_N_DISCS, "%d Discs") \
  X(S_VGA, "VGA") \
  X(S_JUMP_PACK, "Jump Pack") \
  X(S_ONLINE, "Online") \
  /* library */ \
  X(S_SORT_NAME, "Name") \
  X(S_SORT_RECENT, "Recently played") \
  X(S_SORT_MOST, "Most played") \
  X(S_SORT_YEAR, "Release year") \
  X(S_SORT_CARD, "Card order") \
  X(S_SORT_PREFIX, "Sort: %s") \
  X(S_PLAYED_ONCE, "Played once. Last played %s.") \
  X(S_PLAYED_TIMES, "Played %u times. Last played %s.") \
  X(S_RELEASED_NOT_PLAYED, "Released %u. Not played yet.") \
  X(S_NOT_PLAYED, "Not played yet.") \
  /* collections */ \
  X(S_FAV_EMPTY, "Press Y on any game to add it to Favorites.") \
  X(S_NOTHING_HERE, "Nothing here yet.") \
  X(S_COL_ALL, "All Games") \
  X(S_COL_FAVORITES, "Favorites") \
  X(S_COL_RECENT, "Recently Played") \
  X(S_COL_MOST, "Most Played") \
  X(S_COL_PARTY, "Party Night (3+)") \
  X(S_COL_ONLINE, "Online") \
  X(S_COL_LIGHTGUN, "Light Gun") \
  X(S_COL_VGA, "VGA Compatible") \
  X(S_COL_IMPORTS, "Imports") \
  X(S_COL_OTHER, "Homebrew & Other") \
  X(S_GENRE_ACTION, "Action") \
  X(S_GENRE_RACING, "Racing") \
  X(S_GENRE_SIMULATION, "Simulation") \
  X(S_GENRE_SPORTS, "Sports") \
  X(S_GENRE_LIGHTGUN, "Light Gun Games") \
  X(S_GENRE_FIGHTING, "Fighting") \
  X(S_GENRE_SHOOTER, "Shooter") \
  X(S_GENRE_SURVIVAL, "Survival") \
  X(S_GENRE_ADVENTURE, "Adventure") \
  X(S_GENRE_PLATFORMER, "Platformer") \
  X(S_GENRE_RPG, "RPG") \
  X(S_GENRE_SHMUP, "Shoot 'em Up") \
  X(S_GENRE_STRATEGY, "Strategy") \
  X(S_GENRE_PUZZLE, "Puzzle") \
  X(S_GENRE_ARCADE, "Arcade") \
  X(S_GENRE_MUSIC, "Music") \
  /* system rows */ \
  X(S_SYS_STYLE, "Menu style") \
  X(S_SYS_ACCENT, "Accent colour") \
  X(S_SYS_BACKDROP, "Backdrop") \
  X(S_SYS_LOGO, "Header logo") \
  X(S_SYS_QUALITY, "Picture quality") \
  X(S_SYS_MUSIC, "Menu music") \
  X(S_SYS_MUSIC_VOL, "Music volume") \
  X(S_SYS_SFX, "Navigation sounds") \
  X(S_SYS_SFX_VOL, "Sound volume") \
  X(S_SYS_RESUME, "Start on") \
  X(S_SYS_CLOCK, "Clock") \
  X(S_SYS_RUMBLE, "Rumble on launch") \
  X(S_SYS_START, "Start games with") \
  X(S_SYS_VIDEO, "Video") \
  X(S_SYS_BEEP, "VMU beep on save") \
  X(S_SYS_GAMEID, "VM2 / VMU Pro game cards") \
  X(S_SYS_ATTRACT, "Screen saver") \
  X(S_SYS_SAVER_STYLE, "Screen saver style") \
  X(S_SYS_SAVER_TIME, "Start screen saver") \
  X(S_SYS_SAVER_TEST, "Preview screen saver") \
  X(S_SYS_VMU, "VMU saves") \
  X(S_SYS_SAVE, "Save settings to VMU") \
  X(S_SYS_PADTEST, "Controller test") \
  X(S_SYS_BIOS, "Exit to Dreamcast BIOS") \
  X(S_SYS_DIAG, "Diagnostics") \
  X(S_SYS_LANGUAGE, "Language") \
  /* system values */ \
  X(S_ON, "On") \
  X(S_OFF, "Off") \
  X(S_STYLE_SWIRL, "SWIRL") \
  X(S_STYLE_LIST, "Classic list") \
  X(S_STYLE_GRID, "Classic grid") \
  X(S_STYLE_GDMENU, "GDMENU") \
  X(S_NEEDS_UPDATE, "%s (needs Update SWIRL)") \
  X(S_ACCENT_ORANGE, "Orange") \
  X(S_ACCENT_BLUE, "Blue") \
  X(S_ACCENT_GREEN, "Green") \
  X(S_ACCENT_PINK, "Pink") \
  X(S_ACCENT_PURPLE, "Purple") \
  X(S_ACCENT_RED, "Red") \
  X(S_ACCENT_GOLD, "Gold") \
  X(S_ACCENT_TEAL, "Teal") \
  X(S_SET_BY_SEASON, "Set by season") \
  X(S_BACKDROP_COVER, "Cover colour") \
  X(S_BACKDROP_NIGHT, "Night") \
  X(S_BACKDROP_SEASONAL, "Seasonal") \
  X(S_SEASONAL_PREFIX, "Seasonal: %s") \
  X(S_SEASON_WINTER, "Winter") \
  X(S_SEASON_VALENTINE, "Valentine") \
  X(S_SEASON_SPRING, "Spring") \
  X(S_SEASON_EARLY_SUMMER, "Early summer") \
  X(S_SEASON_SUMMER, "Summer") \
  X(S_SEASON_HARVEST, "Harvest") \
  X(S_SEASON_HALLOWEEN, "Halloween") \
  X(S_SEASON_AUTUMN, "Autumn") \
  X(S_SEASON_HOLIDAYS, "Holidays") \
  X(S_LOGO_OLD, "Orange (Update SWIRL for blue)") \
  X(S_LOGO_NONE, "No logo on card") \
  X(S_LOGO_ORANGE, "Orange (USA, Japan)") \
  X(S_LOGO_BLUE, "Blue (Europe)") \
  X(S_LOGO_REGION_BLUE, "By region: blue") \
  X(S_LOGO_REGION_ORANGE, "By region: orange") \
  X(S_QUALITY_STANDARD, "Standard") \
  X(S_QUALITY_HIGH, "High") \
  X(S_NO_MUSIC, "No music on card") \
  X(S_N_OF_10, "%d / 10") \
  X(S_RESUME_LAST, "Last played game") \
  X(S_RESUME_HOME, "Home") \
  X(S_CLOCK_24, "24 hour") \
  X(S_CLOCK_12, "12 hour") \
  X(S_BOOT_NONE, "Straight to the game") \
  X(S_BOOT_ANIMATION, "Boot animation") \
  X(S_BOOT_LICENSE, "SEGA screen") \
  X(S_BOOT_BOTH, "Animation and SEGA") \
  X(S_VIDEO_DEFAULT, "Game default") \
  X(S_VIDEO_VGA, "Force VGA") \
  X(S_SAVER_DRIFT, "Cover drift") \
  X(S_SAVER_SHOWCASE, "Game showcase") \
  X(S_SAVER_SWIRL, "Swirl") \
  X(S_SAVER_BOUNCE, "Bouncing logo") \
  X(S_SAVER_DIM, "Dim the screen") \
  X(S_AFTER_MIN, "After %d min") \
  X(S_PRESS_A, "Press A") \
  X(S_NO_WARNINGS, "No warnings") \
  X(S_ONE_WARNING, "1 warning") \
  X(S_N_WARNINGS, "%d warnings") \
  X(S_LANG_AUTO, "Console setting (%s)") \
  X(S_LANG_MISSING, "%s (not on this card)") \
  /* system panels */ \
  X(S_PANEL_LIBRARY, "Your library") \
  X(S_GAMES_ON_CARD, "Games on SD card") \
  X(S_FAVORITES, "Favorites") \
  X(S_GAMES_PLAYED, "Games played") \
  X(S_TOTAL_LAUNCHES, "Total launches") \
  X(S_ABOUT, "About SWIRL") \
  X(S_VERSION, "Version %s") \
  X(S_BUILD, "%s, build %s") \
  X(S_CREDITS, "Built on openMenu by mrneo240. Fonts: Sora and Barlow (OFL).") \
  X(S_DIAG_NONE, "Diagnostics: no warnings") \
  X(S_DIAG_ONE, "Diagnostics: 1 warning") \
  X(S_DIAG_N, "Diagnostics: %d warnings") \
  /* system hints and toasts */ \
  X(S_HINT_UPDATE_FIRST, "Update SWIRL in Card Manager first") \
  X(S_HINT_NOT_SEASONAL, "Pick a backdrop other than Seasonal first") \
  X(S_HINT_LOGO_UPDATE, "Update SWIRL from Card Manager 2.15 to choose") \
  X(S_HINT_LOGO_NONE, "No logo on this card") \
  X(S_HINT_NEXT_START, "Applies the next time SWIRL starts") \
  X(S_HINT_ADD_MUSIC, "Add music with SWIRL Card Manager") \
  X(S_HINT_START_WITH, "The way the console starts a game") \
  X(S_HINT_GAME_START_WINS, "A game's own Start with setting still wins") \
  X(S_HINT_GAME_VIDEO_WINS, "A game's own Video setting still wins") \
  X(S_HINT_SWITCH_STYLE, "Press A to switch to %s. It saves right away.") \
  X(S_HINT_LANGUAGE, "Menus and messages. Game names stay as they are on the card") \
  X(S_HINT_LANG_MISSING, "Update SWIRL in Card Manager to put this language on the card") \
  /* details */ \
  X(S_VMU_PREVIEW, "VMU PREVIEW") \
  X(S_NEEDS_BLOCKS, "Needs %d VMU blocks") \
  X(S_NEEDS_ONE_BLOCK, "Needs 1 VMU block") \
  X(S_REGION_USA, "USA") \
  X(S_REGION_JAPAN, "Japan") \
  X(S_REGION_EUROPE, "Europe") \
  X(S_REGION_DEFAULT, "Game default") \
  X(S_META_FULL, "Released %u     Region %s     Slot %02u") \
  X(S_META_SHORT, "Region %s     Slot %02u") \
  X(S_SCREENS, "SCREENS") \
  X(S_CHOOSE_DISC, "CHOOSE DISC") \
  X(S_PLAY_DISC, "Play Disc %d") \
  X(S_PLAY_BLEEM, "Play in Bleem") \
  X(S_OPTIONS_CUSTOM, "Options (custom)") \
  X(S_OPTIONS, "Options") \
  X(S_BACK, "Back") \
  X(S_SLOT_ON_SD, "Slot %02u on SD") \
  X(S_NO_BLEEM, "BLEEM.BIN is not on the menu disc") \
  X(S_NO_LAUNCHER, "That launcher is not on the menu disc") \
  X(S_NO_META, "No description in META.DAT for this title.") \
  /* launch and resume */ \
  X(S_LOADING, "LOADING") \
  X(S_STARTING_CB, "STARTING WITH CODEBREAKER") \
  X(S_STARTING_BLEEM, "STARTING IN BLEEM") \
  X(S_STARTING, "STARTING") \
  X(S_SAVING_HISTORY, "Saving your history to the VMU") \
  X(S_RESUMING, "RESUMING YOUR LAST GAME") \
  X(S_STARTING_IN, "Starting in %d") \
  X(S_START_NOW, "Start now") \
  X(S_STAY, "Stay in SWIRL") \
  /* launch options */ \
  X(S_LAUNCH_OPTIONS, "Launch options") \
  X(S_PLAY_CB, "Play with CodeBreaker cheats") \
  X(S_ADD_IN_CM, "Add it in Card Manager") \
  X(S_OPT_REGION, "Region") \
  X(S_OPT_VIDEO, "Video") \
  X(S_OPT_START_WITH, "Start with") \
  X(S_OPT_RESET, "Reset to defaults") \
  X(S_OPT_HELP, "Change these only for games that fail to start. Saved to your VMU.") \
  X(S_ADD_CB_FIRST, "Add CodeBreaker in Card Manager first") \
  X(S_OPT_RESET_DONE, "Launch options reset to the System defaults") \
  /* VMU saves */ \
  X(S_VMU_TITLE, "VMU saves") \
  X(S_VMU_NONE, "No VMU or memory card found. Plug one into a controller and open this screen again.") \
  X(S_VMU_UNREADABLE, "This memory card could not be read.") \
  X(S_VMU_EMPTY, "No saves on this memory card.") \
  X(S_VMU_SWIRL_FILE, "SWIRL settings and history") \
  X(S_VMU_SPACE, "Space") \
  X(S_VMU_BLOCKS_FREE, "blocks free") \
  X(S_VMU_SUMMARY, "%d saves, %d blocks used") \
  X(S_ONE_BLOCK, "1 block") \
  X(S_N_BLOCKS, "%d blocks") \
  X(S_VMU_SAVED_LABEL, "Saved") \
  X(S_VMU_GAME, "VMU game") \
  X(S_VMU_SAVE_FILE, "Save file") \
  X(S_COPY_TITLE, "Copy this save") \
  X(S_COPY_TARGET, "< to %c%d >   %d blocks free") \
  X(S_COPY_PROTECTED, "This save is copy protected.") \
  X(S_COPY_NO_ROOM, "Not enough room on that card.") \
  X(S_COPY_REPLACES, "Replaces the save of the same name there.") \
  X(S_COPY_KEEPS, "The original stays where it is.") \
  X(S_COPY, "Copy") \
  X(S_CANCEL, "Cancel") \
  X(S_DELETE_ASK, "Delete this save?") \
  X(S_DELETE_WARN, "This cannot be undone.") \
  X(S_DELETE, "Delete") \
  X(S_KEEP, "Keep it") \
  X(S_COPY_PROTECTED_TOAST, "That save is copy protected") \
  X(S_COPY_NO_ROOM_TOAST, "Not enough room on that card") \
  X(S_COPYING_TO, "Copying to %c%d...") \
  X(S_COPY_READ_FAIL, "Could not read that save") \
  X(S_COPY_WRITE_FAIL, "The copy could not be written") \
  X(S_COPY_VERIFY_FAIL, "The copy did not read back the same") \
  X(S_COPIED, "Copied") \
  X(S_DELETED, "Save deleted") \
  X(S_DELETE_FAIL, "Could not delete that save") \
  X(S_COPY_NEED_SECOND, "Plug in a second memory card to copy to") \
  /* controller test */ \
  X(S_PAD_TITLE, "Controller test") \
  X(S_PAD_NONE, "No controller in port A") \
  X(S_PAD_START, "Start") \
  X(S_PAD_L, "L trigger") \
  X(S_PAD_R, "R trigger") \
  X(S_PAD_LEAVE, "Hold B and Start together to leave") \
  /* diagnostics */ \
  X(S_DIAG_TITLE, "Diagnostics") \
  X(S_DIAG_EMPTY, "No warnings since power on.") \
  X(S_DIAG_HELP, "A warning is a problem SWIRL got past: a save that failed, a picture it could not use, a line in OPENMENU.INI it skipped. They are listed here with a code.") \
  X(S_DIAG_QR_HELP, "A shows the full report as QR codes to photograph for a bug report. It holds no game names beyond the last few steps.") \
  /* 2.17: backdrops and custom pictures */ \
  X(S_SYS_MOTION, "Backdrop motion") \
  X(S_BACKDROP_TIDE, "Tide") \
  X(S_BACKDROP_SPIRAL, "Spiral") \
  X(S_BACKDROP_STARFIELD, "Starfield") \
  X(S_BACKDROP_EMBERS, "Embers") \
  X(S_BACKDROP_HORIZON, "Horizon") \
  X(S_SYS_PIC_DIM, "Picture dim") \
  X(S_HINT_PIC_DAMAGED, "This picture could not be used, so Cover colour is shown") \
  X(S_HINT_MOTION_OFF, "Nothing in the backdrop moves now") \
  X(S_HINT_MOTION_ON, "Backdrops move again") \
  X(S_PIC_FAILED, "%s (not usable)")

enum {
#define SW_LANG_ENUM(id, text) id,
  SW_STRINGS(SW_LANG_ENUM)
#undef SW_LANG_ENUM
  S_COUNT
};

/* the string in the current language (English when it has no translation) */
const char *sw_t(int id);
#define T(id) sw_t(id)

/* The languages a LANG.DAT can hold, by the Dreamcast's own language setting order (flashrom). */
enum { SW_LANG_AUTO = 0, SW_LANG_EN, SW_LANG_DE, SW_LANG_FR, SW_LANG_ES, SW_LANG_IT, SW_LANG_PT, SW_LANG_COUNT };

/* Reads LANG.DAT from the card (once; 0 on success, -1 none, -2 unusable) and keeps the languages it holds. */
int sw_lang_load(const char *path);
/* Picks the language: SW_LANG_AUTO follows the console's setting, else one of SW_LANG_*. A language the
   file does not hold shows English. */
void sw_lang_select(int lang);
int sw_lang_current(void);                  /* the language in use after select (never AUTO) */
int sw_lang_available(int lang);            /* 1 when the file holds it (English always) */
const char *sw_lang_name(int lang);         /* its name in that language ("Deutsch") */
int sw_lang_console(void);                  /* the console's own language setting as SW_LANG_* */
const char *sw_lang_key(int id);            /* the key name of a string, for diagnostics */

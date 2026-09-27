# Changelog

SWIRL (the menu) and SWIRL Card Manager share one version number. Card Manager patch releases (2.11.1)
may ship without menu changes; the menu then keeps its major.minor version (2.11).

## [2.13.2]

### Fixes

- SWIRL no longer stops on a black screen when a memory card doesn't answer at power on (a VM2 or VMU Pro switching cards, or a faulty VMU). It waits up to 1.5 seconds, then starts; the card is picked up when it answers
- Settings and favorites are kept when a memory card is busy or slow: a failed save is tried again after a few seconds, every save is read back to check it, and the result shows in System > Save settings to VMU
- If SWIRL started before the memory card was ready, it reads the card's save and merges it before writing, instead of replacing it with default settings
- Saves always finish before SWIRL starts a game, changes style or opens the BIOS: the screen shows "Saving to VMU" and keeps moving meanwhile. Only one part of SWIRL uses the memory card at a time. A card that stops answering still ends the save with an error instead of holding the Dreamcast
- "VMU beep on save" is now saved like the other settings (it was lost at power off unless a game was started)
- A damaged SWIRL save file is replaced at the next save instead of being ignored
- Holding Y at power on to go back to the SWIRL style now sticks: the style is saved to the VMU straight away. Before, the Classic style came back at the next power on unless a game was started
- Holding Y at power on is read several times, and holding it for about a second after a Classic style appears also switches back to SWIRL. A Y held from power on is never passed on to a Classic style (where Y leaves to the BIOS, which could then stop on the Dreamcast logo)
- Start up is recorded step by step. If SWIRL ever stops with an error, it now shows a report screen to photograph instead of restarting the Dreamcast over and over

### Saving

- A banner shows where every change is: "Unsaved changes. Saving in 3", then "Saving... please wait" (kept up 2 seconds after the save is done), then "Saved to VMU" or why it couldn't save. It shows on every screen, so favourites and launch options are covered too
- Menu style: picking a style now says "Press A to switch to ... It saves right away." Pressing A saves at once, shows the result, then switches. A style picked but not switched to is dropped when you leave the row
- No VMU, or no space: the banner says so once instead of trying again
- Switching style from a Classic style's settings (Save or Apply) no longer starts the first game in SWIRL: the button that made the switch is ignored until it is let go
- A style change made with Apply in the Classic settings is now saved too, like SWIRL's own Menu style, so the old style doesn't come back at the next power on

### CodeBreaker

- "Play with CodeBreaker cheats" is always listed in a game's launch options; without CodeBreaker on the card it is greyed out and says how to add it
- SWIRL Card Manager: Art and info > CodeBreaker adds your own PELICAN.BIN (and a CHEATS folder or FCDCHEATS.BIN next to it) to the menu disc
- Cheats for games whose serial has a dash (for example T-8101N) are found

### SWIRL Card Manager

- VMU screens from the games moved from Look and sound to Art and info, next to the new CodeBreaker panel

## [2.13.1]

### Fixes

- Choosing Classic list, Classic grid or GDMENU no longer freezes the Dreamcast on cards set up from GDMENU or from scratch. Update SWIRL now adds the openMenu theme files and fonts those styles need (from openMenu's official release, checked by its SHA-256; files already on the card are kept)
- A card whose Classic style can't run starts in SWIRL instead of freezing, so cards affected by this start again after the update. The Style setting shows "needs Update SWIRL" until the files are there
- Hold Y while the Dreamcast starts to go back to the SWIRL style from any style

## [2.13.0]

### SWIRL Card Manager for macOS (beta)

- The Mac version is a beta. Please report anything that does not work, and pick "SWIRL Card Manager on a Mac (beta)" in the bug report
- SWIRL Card Manager now runs on Macs (macOS 10.15 or newer, Apple Silicon and Intel) with every feature of the Windows app: adding games and archives, names, art, screenshots, collections, music, backups, the health check, New card from scratch, Preview in Flycast and VMU screen capture
- Download the .dmg and drag the app to Applications, or use Move to Applications from inside the app; it updates itself like the Windows version
- Formatting uses the same FAT32 writer as on Windows, asks for your Mac password, and refuses the startup disk, internal drives and disk images; you confirm by typing the card's name
- A card is only called write protected when macOS reports its lock switch is on. When macOS does not let the app write to the card directly, it formats it with macOS Disk Utility instead
- The app window uses Chrome, Edge or Brave in app mode when one is installed, otherwise Safari
- Spotlight is kept from indexing cards formatted by the app, so macOS does not write its index onto them
- The hidden "._" files macOS writes next to every file on a FAT32 card are removed from the card, and are never mistaken for a disc (they made a freshly updated card show "No menu yet")
- The patched Flycast used for Preview and VMU capture is built for the Mac by the release workflow; its source changes are in swirl/vmucap

### Fixes (Windows and Mac)

- Art from the game discs no longer shows an error when there is nothing to add; it now says every game already has art
- Hidden "._" files copied from a Mac are never mistaken for a disc
- Version tags are left out of game names: a game added as "Toy Commander v1.022" is named "Toy Commander" even when its serial is not in the Redump list, and Tidy names offers the same for games already on the card

### SWIRL menu

- Version 2.13 to match; no changes on the Dreamcast.

## [2.12.1]

### SWIRL Card Manager

- A Buy me a coffee link in the About window, for anyone who wants to say thanks. SWIRL stays free.

## [2.12.0]

### SWIRL menu

- The Sega Dreamcast logo is back at the top left of every screen, on every card. It is taken from openMenu's USA theme picture on the menu disc, and SWIRL now finds it even when the disc has no `EMPTY.PVR`.

### SWIRL Card Manager

- Update SWIRL adds openMenu's USA theme picture (the source of the header logo) to menu discs that do not have it, such as new cards. It is downloaded once from openMenu's own GitHub release, checked against a fixed checksum and kept with the app's data. Without an internet connection SWIRL shows its name in the header instead, as before.

## [2.11.0]

First public release.

### SWIRL menu

- Home, Library, Collections and System tabs with a 24 bit, anti aliased renderer and 8 bit alpha fonts
- Box art that loads sharp, with cross fades and prefetching, and a cover coloured backdrop
- Collections: favorites, recently and most played, party games, online, light gun, every genre, VGA, imports, homebrew, and your own collections
- Game page with description, two screenshots, VMU preview, play stats and a disc picker for multi disc games
- Per game launch options: region, video (Force VGA or game default), boot animation and SEGA screen, CodeBreaker
- Region free and VGA by default for every game
- Menu music with a built in theme (SWIRL Main Theme), navigation sounds, adjustable volumes, and pop free sound chip handling
- Smooth fade in from the boot screen once art and music are ready
- Five animated screen savers (cover drift, game showcase, swirl, bouncing logo, dim the screen), 1 to 30 minutes, with a preview
- Accent colours, night and seasonal backdrops, 12 or 24 hour clock, rumble, VMU beep, start on Home or the last played game
- VMU logo and game text on the VMU, VMU save manager, controller test, exit to BIOS
- The classic openMenu list, grid and GDMENU styles are still available
- Built with GCC 15.1 and KallistiOS 2.2.1

### SWIRL Card Manager

- The SWIRL logo in the app header, the About window, the app icon and shortcuts, and on the VMU
- One Windows exe that can install itself (Start menu, desktop shortcut, Settings > Apps uninstall) and opens in its own window
- Checks GitHub for new versions and updates itself, verifying the SHA-256 checksum
- New card from scratch: FAT32 format with GDEMU friendly settings, pick games one by one or copy a folder
- Add games from folders, disc images and .zip, .7z and .rar archives (including solid and multi part), unpacked straight onto the card
- Duplicate detection by serial and disc number, proper names from the Redump list, multi disc grouping
- List and cover grid views, drag to reorder, Edit window for names, art, VMU screens, screenshots and details
- openMenu art and info databases, art from the discs, screenshots from libretro thumbnails, VMU screen capture in an emulator
- Your own menu music and VMU logo, collections, GDEMU.INI settings, card health check and Flycast preview
- Full card backups to your PC with incremental updates and cancel, restore through New card from scratch
- Old menus and removed games kept in SWIRL_BACKUP with restore
- About window (F1) with shortcuts, credits and copyable details; F5 and Ctrl+F shortcuts

[2.13.0]: https://github.com/TheGlengineer/SWIRL/releases/tag/v2.13.0
[2.12.1]: https://github.com/TheGlengineer/SWIRL/releases/tag/v2.12.1
[2.12.0]: https://github.com/TheGlengineer/SWIRL/releases/tag/v2.12.0
[2.11.0]: https://github.com/TheGlengineer/SWIRL/releases/tag/v2.11.0

# Changelog

SWIRL (the menu) and SWIRL Card Manager share one version number. Card Manager patch releases (2.11.1)
may ship without menu changes; the menu then keeps its major.minor version (2.11).

## [2.13.0]

### SWIRL Card Manager for macOS

- SWIRL Card Manager now runs on Macs (macOS 10.15 or newer, Apple Silicon and Intel) with every feature of the Windows app: adding games and archives, names, art, screenshots, collections, music, backups, the health check, New card from scratch, Preview in Flycast and VMU screen capture
- Download the .dmg and drag the app to Applications, or use Move to Applications from inside the app; it updates itself like the Windows version
- Formatting uses the same FAT32 writer as on Windows, asks for your Mac password, and refuses the startup disk, internal drives and disk images; you confirm by typing the card's name
- The app window uses Chrome, Edge or Brave in app mode when one is installed, otherwise Safari
- Spotlight is kept from indexing cards formatted by the app, so macOS does not write its index onto them
- The hidden "._" files macOS writes next to every file on a FAT32 card are removed from the card, and are never mistaken for a disc (they made a freshly updated card show "No menu yet")
- The patched Flycast used for Preview and VMU capture is built for the Mac by the release workflow; its source changes are in swirl/vmucap

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

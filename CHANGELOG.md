# Changelog

SWIRL (the menu) and SWIRL Card Manager share one version number. Card Manager patch releases (2.11.1)
may ship without menu changes; the menu then keeps its major.minor version (2.11).

## [2.14.0-preview.4]

**This is a preview for testing.** Card Manager offers it to anyone who chose **Try the preview**. It carries everything planned for 2.14.0 and for the 2.14.1 that was going to follow it; the stable 2.14.0 is this preview after the hardware checks.

### Games with no VGA mode

- A few games (Hydro Thunder is one) have no VGA output of their own: over a VGA cable they reach the SEGA screen and then the display shows nothing. Card Manager now carries community VGA patches for such games and applies them in place on the card, a few bytes, with an undo. The Games tab shows **Needs patch**, **Patched**, or **No (patch removed)**, and **Edit** has **Apply VGA patch** and **Remove VGA patch** with the patch author
- The first time a card holds a game that needs a patch, the SD card page asks how your Dreamcast is connected. **VGA cable**: those games are patched at once, and any game you add later that needs one is patched when it is found. **TV**: nothing is patched. Change the answer under **About**. A patch you removed by hand stays off
- First patch in the catalog: Hydro Thunder v1.020 (USA) by TapamN. You can add your own: a `catalog.json` and `.dcp` files in Card Manager's `patches` folder

### Card Manager: going back is always possible

- The menu that was on the card before SWIRL (openMenu, GDMENU, the Virtual Folder Bundle) is kept as `01_original_...` and never pruned. Backups lists it first with **Go back to openMenu**
- Whatever is in folder 01 is recognised: a GDI or CDI menu, a game, an unknown image. A game in 01 blocks the install with a message instead of being moved
- Every menu backup has a manifest (which menu, when, why, which games were in which slot). A backup with a missing or short file is not restored; one whose games no longer match the card asks first
- An art or info file (BOX, ICON, META, VMU, SHOT.DAT) that Card Manager cannot read is kept and reported, never replaced with an empty one; the scan checks DAT versions
- Coming from the Virtual Folder Bundle: `DISCDB.JSON` is kept in step with the folders when games are added, removed or renumbered (Glen's own card had folder 17 as Hydro Thunder while the database said Dino Crisis); virtual folders become collections once; `type.txt` and `disc.txt` are carried; a database that cannot be read is set aside with a note
- The same corrected Sega serials as the menu, from one table (`serials.tsv`)
- Folder numbers up to 9999, and **Close the gaps** in the health check for the numbering gaps GDEMU stops at (done through the journal, so a failure undoes itself)
- Taking over from openMenu or GDMENU keeps what their list knew: names, regions, VGA and dates that differ from the disc become SWIRL edits, unknown keys are carried and written back, `BLEEM.BIN` comes along from a GDMENU disc

### Card Manager: safer writes

- One lock per card: two operations can no longer run into each other
- The cover grid closes the menu disc before any write, so a Windows rename can no longer fail half way (the cause of a card left without a menu)
- Replacing the menu is two folder renames with a staged copy; a restore copies the backup instead of moving it
- Renumbering is journaled and undone on failure; added games are copied as `NN.part` and renamed when complete; every card write is flushed to the card

### Menu: memory safety in the inherited code

- A bad line, a five digit or stray slot, or an under reported count in OPENMENU.INI is skipped and reported instead of crashing, hanging the boot or dropping every game after it
- A picture that does not fit its slot or has a bad header shows the missing picture instead of corrupting video memory; theme files are checked by size and header, not only by name
- The Classic styles no longer freeze on a long unbroken description, and custom themes on the card are listed and load
- A DAT file with a bad header or table is refused or trimmed instead of read past its end; a damaged OPENMENU.CFG is reported, never copied from an unset pointer
- A multi disc set with more than six entries no longer shifts the slot table in the Classic list

### Menu: saves, memory cards and launches

- Two copies of SWIRL.DAT on the VMU: a save writes the new copy, checks it, then retires the old one, so a card pulled mid write never loses your library. Older SWIRL builds read the file unchanged
- A full VMU says **No space on VMU. Free N blocks** instead of "check the VMU", and System no longer says "retrying" when nothing retries
- A memory card that answers after start up (a VM2 switching cards, a slow VMU) is read within eight seconds instead of ignored until the next power on
- OPENMENU.CFG stays openMenu's version 1 on the VMU, so stock openMenu and the Virtual Folder Bundle keep reading your settings when you switch back; the "SWIRL took over once" flag lives in SWIRL.DAT
- A game marked `type=other` (an audio CD, a demo disc) goes to the console's own menu
- A launch that would write over the loader's memory stops with a message instead of corrupting it; CodeBreaker, Bleem and cheat files are checked for size before loading
- A disc change under a read no longer freezes the menu; the sort choice and an older SWIRL.DAT are saved on their own

### Report a problem

- SWIRL now keeps a log of what it does from power on and counts anything that goes wrong: a save that failed, a picture it could not use, a launch that came back, a line in OPENMENU.INI it could not read. **System, Diagnostics** on the Dreamcast lists them in plain words, and **A** there shows the whole log as QR codes. After a crash or a hang the codes are on the screen already; hold **X** while the console turns on for the start up log
- Card Manager has a new **Report a problem** page. Scan the codes with your phone and paste the text, in any order, or drop photos of the screen on the page. It shows the report in plain words: what happened, the console and what was plugged in, each warning and what it means, where a crash stopped, and the steps SWIRL took. **Copy for GitHub** puts a ready to paste issue on the clipboard with the raw report attached. Nothing is sent anywhere by itself
- The **Report a problem** button on the preview bar opens this page, and About explains the console side under **Diagnostics**

### What to test

- Start every game you own once. Anything that does not reach the game: note the name and what the screen did
- Pull the VMU out during **Saving... please wait** once, put it back, power cycle: your favourites and history should still be there and Diagnostics should say one copy was damaged
- If you have a VM2 or VMU Pro: power on with it switching cards; SWIRL should pick it up within eight seconds
- Coming from the Virtual Folder Bundle: after Update SWIRL, restore the original menu from Backups and check the Bundle still sees every game and your settings
- On the Dreamcast, open **System**, **Diagnostics**, press **A**, and scan the codes with your phone. Paste what it read into **Report a problem** in Card Manager: does the page tell the story of what you did?
- Photograph the TV instead and drop the photos on the page. A sharp photo with the whole code in the frame should read; say so in an issue if yours does not

## [2.14.0-preview.3]

**This is a preview for testing.** Card Manager offers it to anyone who chose **Try the preview**.

### Game launches

- Games now start with the boot animation and the SEGA screen, as they do from openMenu and GDMENU. SWIRL used to skip both by default, which is not how any game was tested by its makers. **Start with** in a game's launch options still lets you skip them per game, and options you already set are kept
- SWIRL no longer waits for ever on a GDEMU that does not answer. Every wait for the drive has a limit; if the image change is not answered or the game's disc does not become ready, SWIRL switches back to the menu disc and tells you why instead of leaving a blank screen
- The drive is locked while SWIRL talks to the GDEMU, so the system's own drive status poll can no longer interrupt a command half way

### What to test

- Start a few games. Do they all reach the game, with the animation and SEGA screen first?
- A game that shows the SEGA screen and then gives your display **no signal** is running in a video mode your display can't show, not a launch problem. Hydro Thunder is the known case: its original release has no VGA mode, and the All Stars re release outputs an unstable VGA picture that many monitors and scalers refuse. Try **Video: Game default** in its launch options, or a TV connection
- In a game's launch options, set **Start with: Straight to the game** and start it: it should skip the animation and SEGA screen as before

## [2.14.0-preview.2]

**This is a preview for testing.** Card Manager offers it to anyone who chose **Try the preview**.

### Animated VMU screen

- **Power on:** the SWIRL logo on the VMU breaks apart, spins into a three armed galaxy that collapses into the centre, and the swirl paints itself back like a brush stroke. The word types in and the screen flashes twice. It plays while SWIRL loads, and only after the settings have been read, so start up is unchanged
- **System tab:** the logo plays its intro, then a glint of light runs along the swirl every few seconds with sparkles around it
- **Your own logo (LOGO.VMU from Card Manager):** it spirals in from the centre, flashes, and a shine sweeps across it every few seconds
- **Saving:** the VMU follows the banner. A 3, 2, 1 countdown with a draining ring; a "saving" picture while the card is written; bits flowing into a little VMU during "Saving... please wait"; then a tick that draws itself with "SAVED". A failed save shows a shaking cross with the reason (NO SPACE, VMU BUSY, CHECK VMU)
- Nothing is sent to the VMU while a save is writing, and the VMU is left alone once a game starts
- The VMU is drawn on its own thread, so the menu never waits for it

### Fixes

- Fixed a memory overflow when loading full size pictures (the header logo and the Classic style backgrounds): 32 bytes were written past the end of a buffer at every start up. It could make a game launch stop with an error report
- If SWIRL ever stops with an error, the report is now shown as QR codes that take turns every few seconds. Photograph each one for a bug report

### What to test

- Power on a few times with a VMU in: does the intro play, and does SWIRL start as before?
- Change a setting and watch the VMU through the countdown, saving and saved
- VM2 or VMU Pro owners: does the screen animate on your device too?

## [2.14.0-preview.1]

**This is a preview for testing.** It is offered in SWIRL Card Manager 2.13.3 or newer as **Try the preview**.

### VM2 and VMU Pro game cards

- When a game starts, SWIRL tells a VM2, VMU Pro, USB4MAPLE or Pico2Maple which game it is (the Game ID), so it switches to that game's own memory card, as the openMenu Virtual Folder Bundle does
- It is only sent when a game starts, after SWIRL has saved, and only to those devices. A standard VMU is never sent anything new. It works from SWIRL and from the Classic styles, and for CodeBreaker and Bleem launches too
- New setting: **System > VM2 / VMU Pro game cards** (on by default). Turn it off to keep one card for everything
- A device that is busy or doesn't answer can't hold up a game: SWIRL waits at most about a second in total, then starts the game anyway

### What to test

- **VM2 or VMU Pro owners:** start a few games. Does your device switch to each game's own card, and do the games save and load there?
- Turn the setting off, start a game: does the device stay on its current card?
- Go back to SWIRL from a game with the in game reset (A+B+X+Y+Start). Are your SWIRL settings and favourites still there, or does SWIRL start with default settings? Please tell us either way
- **Standard VMU owners:** everything should work exactly as in 2.13.2

## [2.13.4]

### Fixes

- Fixed a memory overflow when SWIRL loads full size pictures (the Sega Dreamcast header logo, and the backgrounds of the Classic styles): 32 bytes were written past the end of a buffer at every start up. It could corrupt SWIRL's memory and make the Dreamcast stop later, for example when starting a game

## [2.13.3]

Card Manager only. The menu on your card stays 2.13.2.

### Previews

- Card Manager can now offer **previews** of the next version for testing. A green bar says when one is up, with **See what's new**, **Try the preview** and **Not now** (hides it until the next preview)
- A preview is never installed unless you click **Try the preview**; **Update now** and the daily check only install released versions
- While you use a preview, a bar says so and links to **Report a problem**. **About > Back to the released version** puts the released version back
- **About > Tell me about preview versions** (on by default) turns the offers off

## [2.13.2]

### Fixes

- SWIRL no longer stops on a black screen when a memory card doesn't answer at power on (a VM2 or VMU Pro switching cards, or a faulty VMU). It waits up to 1.5 seconds, then starts; the card is picked up when it answers
- Settings and favorites are kept when a memory card is busy or slow: a failed save is tried again after a few seconds, every save is read back to check it, and the result shows in System > Save settings to VMU
- If SWIRL started before the memory card was ready, it reads the card's save and merges it before writing, instead of replacing it with default settings
- Saves always finish before SWIRL starts a game, changes style or opens the BIOS: the screen shows "Saving... please wait" and keeps moving meanwhile. Only one part of SWIRL uses the memory card at a time. A card that stops answering still ends the save with an error instead of holding the Dreamcast
- "VMU beep on save" is now saved like the other settings (it was lost at power off unless a game was started)
- A damaged SWIRL save file is replaced at the next save instead of being ignored
- Holding Y at power on to go back to the SWIRL style now sticks: the style is saved to the VMU straight away. Before, the Classic style came back at the next power on unless a game was started
- Holding Y at power on is read several times, and holding it for about a second after a Classic style appears also switches back to SWIRL. A Y held from power on is never passed on to a Classic style (where Y leaves to the BIOS, which could then stop on the Dreamcast logo)
- Start up is recorded step by step. If SWIRL ever stops with an error, it now shows a report screen to photograph instead of restarting the Dreamcast over and over

### Start up

- SWIRL appears sooner at power on: the save file is read once instead of twice (about half a second), and the fade in starts after at most 1 second (was 2.5) and takes 0.45 seconds (was 0.7). The sound start is unchanged, so no pops

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

# Troubleshooting

## SWIRL Card Manager

**Windows says "Windows protected your PC".** The app is not code signed. Click **More info**, then **Run
anyway**. You can check the download against `SHA256SUMS.txt` on the release page first:
`certutil -hashfile SWIRL-Card-Manager.exe SHA256`.

**macOS says the app cannot be opened or checked.** The app is not signed with an Apple Developer ID.
Open **System Settings > Privacy & Security** and click **Open Anyway** next to the message about SWIRL
Card Manager (see [Getting started](GETTING_STARTED.md#on-a-mac)). Updates installed from inside the app do
not ask again.

**On a Mac, the card or my Downloads folder shows as empty.** macOS asked for permission and it was
declined. Open **System Settings > Privacy & Security > Files and Folders**, find SWIRL Card Manager and
turn on **Removable Volumes** (and Downloads or Documents if needed).

**On a Mac, opening the app again does nothing.** An earlier copy is still busy finishing a task in the
background (it quits by itself when the task ends), or it was closed while the window was open. Wait a
minute and open it again, or quit it in Activity Monitor.

**The app opens in a browser tab instead of its own window.** It uses Microsoft Edge or Chrome (on a Mac also Brave)
in app mode. If none is installed it falls back to your default browser. Everything works the same.

**My card is not in the list.** Type its drive letter in the box on the SD card page and click **Scan**. Cards
must be formatted FAT32 for GDEMU; **New card from scratch** does that for you.

**New games do not show on the Dreamcast.** Click **Update SWIRL** after adding, removing or editing games.
Edits are saved on the card straight away, but only reach the menu when it is rebuilt.

**An archive shows an error in the file browser.** The archive is damaged or not a real .zip, .7z or .rar.
Multi part RAR sets need every part in the same folder; pick the first part.

**Update now is missing and there is a Download button instead.** That release has no verified exe for this
computer. Download it from the release page.

**Where are my settings?** In `%LOCALAPPDATA%\SWIRL Card Manager` on Windows and `~/Library/Caches/SWIRL Card Manager` on a Mac. **About > Copy details** shows the exact paths.

## On the Dreamcast

**SWIRL froze after choosing a Classic style, or no longer starts with the VMU in.** Hold **Y** on the
controller while the Dreamcast starts, and keep holding it until SWIRL appears: SWIRL starts in its own style
again. Then run **Update SWIRL** in Card
Manager (2.13.1 or newer), which adds the openMenu theme files the Classic styles need. Before 2.13.1, cards
set up from GDMENU or from scratch did not have them. Card Manager downloads them from openMenu's official
release, so it needs an internet connection the first time.

**The Dreamcast stops on its logo, then goes to its own menu after about a minute.** This happens when the
Dreamcast is switched back on before it has fully powered down: the GDEMU doesn't get a clean start and the
console can't read the menu. It isn't caused by SWIRL and happens with any menu. Switch off, wait until the
console is fully off (a few seconds), then switch on. If you just changed a setting, wait for "Saved to VMU"
to disappear before switching off.

**GDEMU boots straight into a game.** Folder `01` has no menu. Run **Update SWIRL**, or check the card with
**Health and preview > Run health check**.

**Some games are missing from the list.** GDEMU stops at the first gap in folder numbers. The health check
finds gaps; removing games in Card Manager renumbers folders for you.

**A game will not start, or shows a black screen.** Open its game page, press **X** for launch options and try
**Video: Game default**, then a specific **Region**. Make sure **Start with** is **Animation and SEGA**: a game may
rely on what the BIOS sets up during the SEGA screen.

**The SEGA screen shows, then the display says "no signal".** The game is running, in a video mode your display
can't show. Some games have no VGA mode (the BIOS normally refuses them on a VGA cable; **Force VGA** gets past
that check, and the game then outputs a picture a VGA display can't lock to), and a few output an unstable VGA
signal that monitors and scalers reject (Hydro Thunder is the known one). Set **Video: Game default** for that
game, or play it over a TV connection. GDMENU and openMenu behave the same way; it is the game, not the menu.
If a game has never worked, test the image itself in an emulator.

**No music.** Check **System > Menu music** and **Music volume**. Music comes from `BGM.ADP` on the menu disc;
**Update SWIRL** puts it there.

**Settings are not kept after power off.** SWIRL saves a few seconds after a change, to the memory card that
already has its save or else the first one with 2 free blocks. The banner at the bottom counts down to the
save, shows "Saving... please wait" while it runs, then "Saved to VMU". **System > Save
settings to VMU** shows where it saved, or why it couldn't (no space, or the card not answering). Check
**System > VMU saves** for space. With a VM2 or VMU Pro, the card that is active when SWIRL starts is used.

**Black screen at power on, but the VMU shows the logo.** Fixed in 2.13.2: SWIRL no longer waits forever for
a memory card that doesn't answer. Update SWIRL with Card Manager 2.13.2 or newer. Until then, start with the
memory card removed, or switch a VM2 or VMU Pro to another card.

**Getting back to SWIRL from a game.** Hold **A + B + X + Y** and press **Start**.

## Reporting a problem

Open an [issue](https://github.com/TheGlengineer/SWIRL/issues/new/choose). In Card Manager, **About > Copy
details** gives the versions and paths to paste in. For the Dreamcast, **System > About SWIRL** shows the
menu version.

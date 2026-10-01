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
finds gaps and **Close the gaps** renumbers the folders; removing games in Card Manager renumbers them too.
Folder numbers go up to 9999.

**A game will not start, or shows a black screen.** Open its game page, press **X** for launch options and try
**Video: Game default**, then a specific **Region**. Make sure **Start with** is **Animation and SEGA** (the
default): a game may rely on what the BIOS sets up during the SEGA screen.

**SWIRL says "GDEMU did not answer the image change" or "The disc did not become ready".** The GDEMU did not
switch to the game within three seconds, or the disc did not become ready within ten, so SWIRL came back to the
menu instead of leaving a blank screen. Try again; if it keeps happening with one game, check its image with the
health check, and if it happens with every game, switch the console off and on. A clone board or an older
firmware that answers late can see this where openMenu used to wait; report it with the QR codes (below) and the
board and firmware. If the menu disc does not come back either, SWIRL stops on a report screen that says
"GDEMU did not answer. Switch the console off and on."

**SWIRL says "Out of memory: switch the console off and on".** The launch would have written over the game
loader's memory, so it stopped instead. Switch off and on; if it comes back, report it.

**The SEGA screen shows, then the display says "no signal".** The game is running, in a video mode your display
can't show. Some games have no VGA mode of their own (Hydro Thunder is the known one): the BIOS normally refuses
them on a VGA cable, **Force VGA** gets past that check, and the game then outputs a picture a VGA display can't
lock to. Card Manager carries community VGA patches for such games: on the SD card page, answer **VGA cable**
when it asks how your Dreamcast is connected, or open the game's Edit page and click **Apply VGA patch**. Without
a patch, set **Video: Game default** for that game, or play it over a TV connection. GDMENU and openMenu behave
the same way; it is the game, not the menu. If a game has never worked, test the image itself in an emulator.

**No music.** Check **System > Menu music** and **Music volume**. Music comes from `BGM.ADP` on the menu disc;
**Update SWIRL** puts it there. With several tracks, a track that is not in the same format as the first (an
`.adp` file from another tool) is skipped; the boot log (hold X) says which.

**Settings are not kept after power off.** SWIRL saves a few seconds after a change, to the memory card that
already has its save or else the first one with room. The banner at the bottom counts down to the
save, shows "Saving... please wait" while it runs, then "Saved to VMU". **System > Save
settings to VMU** shows where it saved, or why it couldn't (no space, or the card not answering). With a VM2 or
VMU Pro, the card that is active when SWIRL starts is used. **System > Diagnostics** lists a failed save as W01.

**"No space on VMU. Free N blocks in VMU saves".** SWIRL keeps two copies of its save (`SWIRL.DAT` and
`SWIRL.BAK`) so a card pulled mid write loses nothing, and needs room for the new copy on top of the old one.
Delete a save you no longer need in **System > VMU saves**, or use another card.

**I pulled the VMU while it was saving.** Nothing is lost: the previous copy is still on the card, and SWIRL starts
from the newest copy that reads back intact. Put the card back and let the next save finish ("Saved to VMU").

**Black screen at power on, but the VMU shows the logo.** Fixed in 2.13.2: SWIRL no longer waits forever for
a memory card that doesn't answer. Update SWIRL with Card Manager 2.13.2 or newer. Until then, start with the
memory card removed, or switch a VM2 or VMU Pro to another card.

**Settings and favorites come back a few seconds after the menu appears.** The memory card answered after
SWIRL had started (a VM2 or VMU Pro still switching cards, a slow VMU). SWIRL watches the ports for the first
8 seconds and reads its files from a card that turns up then, as if it had been there at power on: the
favorites and settings return, and a saved Classic style starts. A card that turns up later is read before
the first save, so nothing is written over it.

**Getting back to SWIRL from a game.** Hold **A + B + X + Y** and press **Start**. It works from games that
route the reset through the BIOS (Crazy Taxi, Sonic Adventure and most Sega titles). Games that reset to their
own title screen (18 Wheeler, AeroWings, Hydro Thunder and most arcade ports) never call the BIOS, so nothing
SWIRL can do catches them; switch the console off and on. It needs `reset_goto = 1` in `GDEMU.INI`, which Card
Manager sets under GDEMU settings.

**An audio CD or a demo disc goes to the Dreamcast's own menu.** That is on purpose: an entry marked `type=other`
(as the Virtual Folder Bundle marks them) is selected on the GDEMU and handed to the console's menu, which plays
or starts it. Use the in game reset or power cycle to get back.

**I went back to openMenu and my settings were still there.** SWIRL writes `OPENMENU.CFG` on the VMU in openMenu's
own format (version 1), so stock openMenu and the Virtual Folder Bundle read the style, theme and sort you set.
SWIRL's own settings and favorites stay in `SWIRL.DAT` and come back when you return.

## Reporting a problem

Open an [issue](https://github.com/TheGlengineer/SWIRL/issues/new/choose). In Card Manager, **About > Copy
details** gives the versions and paths to paste in. For the Dreamcast, **System > About SWIRL** shows the
menu version.

**A report from the Dreamcast.** **System > Diagnostics** lists the warnings since power on (a save that
failed, a picture or an INI line SWIRL could not use, a launch that came back) in plain words with a code; the
codes are listed in [Using SWIRL](USING_SWIRL.md#diagnostics). Press **A** there for the whole report as QR
codes. If SWIRL stops on a report screen by itself (a crash, or no progress for 12 seconds), the codes take
turns on their own. Holding **X** while the Dreamcast starts shows the start up log the same way. The report
holds no game names beyond the last few steps. The format is in [DIAGNOSTICS.md](DIAGNOSTICS.md).

Then open **Report a problem** in Card Manager (in the list on the left; the bar shown while you test a
preview version has the same button). Scan
the codes with your phone and paste the text, in any order, or drop photos of the screen on the page. It shows
the report in plain words and **Copy for GitHub** puts a ready to paste issue on the clipboard. Nothing is sent
anywhere by itself. A photo needs to be sharp with the whole code in the frame; if it does not read, scan the
code with your phone's camera app and paste the text instead. Check the
[known limits](../CHANGELOG.md#known-limits-and-open-questions) first; it may already be there.

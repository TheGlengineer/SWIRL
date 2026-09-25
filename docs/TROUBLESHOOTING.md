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

**GDEMU boots straight into a game.** Folder `01` has no menu. Run **Update SWIRL**, or check the card with
**Health and preview > Run health check**.

**Some games are missing from the list.** GDEMU stops at the first gap in folder numbers. The health check
finds gaps; removing games in Card Manager renumbers folders for you.

**A game will not start, or shows a black screen.** Open its game page, press **X** for launch options and try
**Video: Game default**, then a specific **Region**. Some games also need **Start with: Boot animation**.
If a game has never worked, test the image itself in an emulator.

**No music.** Check **System > Menu music** and **Music volume**. Music comes from `BGM.ADP` on the menu disc;
**Update SWIRL** puts it there.

**Settings are not kept after power off.** SWIRL saves to the first VMU with a few free blocks. Check
**System > VMU saves** for space.

**Getting back to SWIRL from a game.** Hold **A + B + X + Y** and press **Start**.

## Reporting a problem

Open an [issue](https://github.com/TheGlengineer/SWIRL/issues/new/choose). In Card Manager, **About > Copy
details** gives the versions and paths to paste in. For the Dreamcast, **System > About SWIRL** shows the
menu version.

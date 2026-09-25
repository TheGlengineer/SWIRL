# Getting started

This guide takes you from download to playing, on a card that already has games or on an empty one.

## What you need

- A Sega Dreamcast with a GDEMU installed (original or clone board)
- The GDEMU's SD card and a card reader for your PC
- A Windows 10 or 11 PC, or a Mac with macOS 10.15 Catalina or newer (Apple Silicon or Intel)
- Your own game backups (.gdi, .cdi, .mds or .ccd, loose or in .zip, .7z or .rar archives)

## 1. Get SWIRL Card Manager

On a Mac, skip to [On a Mac](#on-a-mac) below, then carry on from step 2.

1. Open the [latest release](https://github.com/TheGlengineer/SWIRL/releases/latest) and download **SWIRL-Card-Manager.exe**.
2. Double click it. Because the app is not code signed, Windows SmartScreen may show "Windows protected your PC". Click **More info**, then **Run anyway**. This is only needed the first time.
3. The app opens in its own window. It uses the Edge engine that comes with Windows (or Chrome if Edge is missing), so there is nothing else to install.

Everything the app needs is inside the exe: the SWIRL menu, the archive unpackers, the fonts, the default music and the emulator used for previews.

### Install it on your PC (optional)

On the **SD card** page, click **Install on this PC**. This copies the app to
`%LOCALAPPDATA%\Programs\SWIRL Card Manager`, adds a Start menu shortcut (and a desktop shortcut if you leave
that ticked) and registers it under **Settings > Apps** so it can be removed cleanly. No administrator
rights are needed.

You can also keep running the downloaded exe. Installing just makes it easier to find and lets updates install themselves.

### On a Mac

1. From the [latest release](https://github.com/TheGlengineer/SWIRL/releases/latest), download **SWIRL-Card-Manager-macOS.dmg** and open it.
2. Drag **SWIRL Card Manager** onto **Applications**, then eject the disk image.
3. Open SWIRL Card Manager from Applications or Launchpad. Because the app is not signed with an Apple Developer ID, macOS says it cannot check it for malicious software. Click **Done** (or **OK**).
4. Open **System Settings > Privacy & Security**, scroll down to the message about SWIRL Card Manager and click **Open Anyway**, then enter your password. On macOS 14 and older you can instead Control click the app in Applications and choose **Open**. This is only needed once.
5. The app opens in its own window using Chrome, Edge or Brave if one is installed, otherwise in a Safari tab. It has no Dock icon of its own; the window is the app.
6. The first time it reads your SD card, macOS asks whether SWIRL Card Manager may access files on a removable volume. Click **Allow**. It may ask the same for your Downloads or Documents folder when you browse there.

Everything else works as on Windows, including New card from scratch (macOS asks for your password to format the card), Preview in Flycast and VMU screen capture. Updates install themselves from inside the app.

## 2. Pick your SD card

Put the card in your computer. The **SD card** page lists your drives; pick the card, or type its drive letter (Windows) or its path such as `/Volumes/GDEMU` (Mac) and click **Scan**.
The header then shows the card, what menu it has and how many games it holds.

## 3a. A card that already has games

If the card was set up with GDMENUCardManager, openMenu or GDMENU, it already follows the GDEMU layout
(folder `01` for the menu, `02` and up for games). Click **Update SWIRL** at the top right.

- Only folder `01` is rebuilt. Game folders are never changed by an update.
- The previous menu is moved to `SWIRL_BACKUP` on the card, and the **Backups** page can put it back.
- Existing box art and game info in the menu disc are kept.

## 3b. An empty or new card

Click **New card from scratch**.

1. **The SD card to erase**: pick the card. Everything on it will be erased.
2. **Games to put on it** (optional): click **Pick games** to choose games one by one, from any folders, or copy everything from one folder, such as a backup of an old card.
3. **Box art and descriptions** (optional): point at a folder with `BOX.DAT`, `ICON.DAT` and `META.DAT`, and/or tick the box to download missing art from the openMenu databases.
4. **Confirm**: type the card's drive letter (Windows) or its name (Mac) and click **Erase and set up card**. Windows asks for administrator permission, and macOS for your password, to format the card.

The card is formatted FAT32 with the cluster size GDEMU likes, SWIRL is installed and your games are copied over.

A card that only has SWIRL on it (no games yet) is fine: add games at any time with **Games > Add games**.

## 4. Add more games

**Games > Add games** opens a file browser that shows game folders, disc images and archives. It looks
inside archives, marks games that are already on the card and unpacks archives straight onto the card.
Tick what you want and click **Add**. New games get proper names from the Redump list, and discs of the
same game are kept together.

Then click **Update SWIRL** so the menu on the card lists them.

## 5. Play

Put the card back in the GDEMU and switch the Dreamcast on. SWIRL starts in a couple of seconds.
[Using SWIRL](USING_SWIRL.md) explains every screen.

## Updating

SWIRL Card Manager checks GitHub once a day and shows a bar when a new version is out. Click **Update now**:
it downloads the new version, checks it against the published checksum, installs it and reopens. Your
settings, cards and games are kept. You can also click **Check for updates** in **About** (F1), or turn the
daily check off there.

The SWIRL menu is built into Card Manager, so after updating the app, click **Update SWIRL** to put the
new menu on your card.

## Uninstalling

Use **Settings > Apps > SWIRL Card Manager > Uninstall**, or the **Uninstall** button on the SD card page.
It asks whether to also delete the downloaded art database, emulator files and settings (kept in
`%LOCALAPPDATA%\SWIRL Card Manager`). Your SD cards are never touched.

To go back to your old menu on a card, open **Backups** and click **Restore** next to it.

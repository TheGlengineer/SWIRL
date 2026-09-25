<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/logo/swirl-logo.png">
  <img src="docs/images/logo/swirl-logo-light.png" alt="SWIRL" width="320">
</picture>

**A modern dashboard menu for the Sega Dreamcast and GDEMU, and a Windows app that sets up your SD card for it.**

[![Latest release](https://img.shields.io/github/v/release/TheGlengineer/SWIRL?label=release&color=f28c28)](https://github.com/TheGlengineer/SWIRL/releases/latest)
[![Build](https://img.shields.io/github/actions/workflow/status/TheGlengineer/SWIRL/ci.yml?branch=master&label=build)](https://github.com/TheGlengineer/SWIRL/actions/workflows/ci.yml)
[![License: BSD 3-Clause](https://img.shields.io/badge/license-BSD%203--Clause-blue)](LICENSE.md)
[![Built on openMenu](https://img.shields.io/badge/built%20on-openMenu-555)](https://github.com/mrneo240/openMenu)

<img src="docs/images/swirl-home.png" alt="SWIRL home screen on a Dreamcast" width="720">

</div>

SWIRL replaces the list of files you normally see when a GDEMU equipped Dreamcast boots. You get big box art,
smooth 60 fps motion, collections, play history, screenshots, menu music, a live VMU and screen savers. It is
built on [openMenu](https://github.com/mrneo240/openMenu), so it keeps the same SD card layout and the same art
databases. Nothing about your games has to change.

**SWIRL Card Manager** is the companion Windows app. It puts SWIRL on the card, adds games (including straight
from .zip, .7z and .rar), fixes their names, fetches art, backs the card up and keeps everything up to date.

> [!NOTE]
> SWIRL does not include any games. Use backups of discs you own.

## Contents

- [Highlights](#highlights)
- [Screenshots](#screenshots)
- [Quick start](#quick-start)
- [Controls](#controls)
- [How it works with your SD card](#how-it-works-with-your-sd-card)
- [Documentation](#documentation)
- [Building from source](#building-from-source)
- [Credits](#credits)
- [License](#license)

## Highlights

**On the Dreamcast**

- Home, Library, Collections and System tabs with animated, anti aliased 24 bit graphics at 60 fps
- Box art that loads sharp, with smooth cross fades and a cover coloured backdrop
- Collections built from your library: favorites, recently played, most played, party games, every genre, plus your own
- Game pages with description, screenshots, VMU preview, play count and a disc picker for multi disc games
- Every game boots region free and in VGA, with per game launch options (region, video, boot screens)
- Menu music with a built in theme, navigation sounds, rumble and VMU pictures
- Five animated screen savers, including a simple dim overlay
- Favorites, history and settings saved to your VMU
- Classic openMenu list, grid and GDMENU styles still one setting away

**In SWIRL Card Manager (Windows)**

- One exe with nothing else to install; it can install itself with Start menu shortcuts and an uninstaller
- Add games from folders, disc images or archives (.zip, .7z, .rar, including multi part), unpacked straight onto the card
- Duplicate detection, proper game names from the Redump list and automatic grouping of multi disc games
- List or cover grid view, drag to reorder, and an Edit window for names, art, VMU screens, screenshots and details
- Art and info from the openMenu databases, screenshots from libretro thumbnails, and VMU screens captured by booting each game in an emulator
- Your own menu music and VMU logo, collections, GDEMU settings and a card health check
- New card from scratch (formats the card for GDEMU and fills it), full card backups to your PC, and restore
- Checks GitHub for new versions and updates itself

## Screenshots

### On the Dreamcast

| Home | Library |
|---|---|
| ![Home](docs/images/swirl-home.png) | ![Library](docs/images/swirl-library.png) |
| **Game page** | **Launch options** |
| ![Game page](docs/images/swirl-detail.png) | ![Launch options](docs/images/swirl-launch-options.png) |
| **Collections** | **System settings** |
| ![Collections](docs/images/swirl-collections.png) | ![System settings](docs/images/swirl-system.png) |

![Screen savers](docs/images/swirl-screensavers.png)

### SWIRL Card Manager

![Games in the cover grid](docs/images/cm-games-covers.png)

| Games list | Edit a game |
|---|---|
| ![Games list](docs/images/cm-games-list.png) | ![Edit a game](docs/images/cm-edit.png) |
| **Add games (archives supported)** | **New card from scratch** |
| ![Add games](docs/images/cm-browser.png) | ![New card](docs/images/cm-newcard.png) |
| **Look and sound** | **Backups** |
| ![Look and sound](docs/images/cm-look.png) | ![Backups](docs/images/cm-backups.png) |

More screens are in the [Card Manager guide](docs/CARD_MANAGER.md).

## Quick start

You need a Dreamcast with a GDEMU (original or clone), its SD card, and a Windows 10 or 11 PC.

1. Download **SWIRL-Card-Manager.exe** from the [latest release](https://github.com/TheGlengineer/SWIRL/releases/latest).
2. Run it. Windows may say "Windows protected your PC" because the app is not code signed. Click **More info**, then **Run anyway**.
3. Put the SD card in your PC and pick it at the top of the window.
4. If the card already has games (for example from GDMENUCardManager), click **Update SWIRL**. Only folder `01`, the menu, is rebuilt. Your game folders are not touched, and the old menu is kept in `SWIRL_BACKUP` on the card.
5. For an empty card, use **New card from scratch** or **Games > Add games**.
6. Put the card back in the GDEMU and switch the Dreamcast on.

The full walk through, including installing the app on your PC, is in [Getting started](docs/GETTING_STARTED.md).

## Controls

| Button | What it does |
|---|---|
| L / R | Switch tabs |
| D-Pad | Browse |
| A | Play (Home) or open |
| X | Game page (Home), change sort (Library), launch options (game page) |
| Y | Favorite |
| B | Back |
| Start | System settings |
| Down on Home | Surprise me: picks a random game |
| Keyboard | Type in the Library to jump to a game |
| A + B + X + Y + Start in a game | Back to SWIRL |

See [Using SWIRL](docs/USING_SWIRL.md) for every screen and setting.

## How it works with your SD card

SWIRL keeps the GDEMU layout that GDMENUCardManager and openMenu use:

```
SD card
├── 01/                 the menu disc (SWIRL lives here)
├── 02/, 03/, ...       one folder per game disc, unchanged
├── GDEMU.INI           GDEMU settings
├── SWIRL/              your edits: names, art, collections (Card Manager)
└── SWIRL_BACKUP/       previous menus and removed games
```

The menu disc holds `OPENMENU.INI` and the same `BOX.DAT`, `ICON.DAT` and `META.DAT` art files as openMenu,
plus a few optional files that SWIRL adds (screenshots, VMU screens, music, collections). Details are in
[SD card layout](docs/SD_CARD_LAYOUT.md).

## Documentation

| Guide | For |
|---|---|
| [Getting started](docs/GETTING_STARTED.md) | Installing, your first card, updating |
| [Using SWIRL](docs/USING_SWIRL.md) | The menu on the Dreamcast: screens, controls, settings |
| [SWIRL Card Manager](docs/CARD_MANAGER.md) | Every page of the Windows app |
| [SD card layout](docs/SD_CARD_LAYOUT.md) | Which files live where, and which are optional |
| [Troubleshooting](docs/TROUBLESHOOTING.md) | Common problems and fixes |
| [Building from source](docs/BUILDING.md) | Toolchain, building the menu and the app, tests |
| [Releasing](docs/RELEASING.md) | Versions, GitHub releases and the in app updater |
| [Contributing](CONTRIBUTING.md) | How to help, forks and pull requests |
| [Changelog](CHANGELOG.md) | What changed in each version |

## Building from source

The short version, on Linux or WSL2 (Ubuntu 22.04):

```sh
swirl/setup_toolchain.sh              # GCC 15.1 + KallistiOS 2.2.1 into ~/dreamcast
swirl/build.sh                        # builds swirl/build/gdemu/1ST_READ.BIN

cd swirl/cardmanager                  # Go 1.21 or newer
cp ../build/gdemu/1ST_READ.BIN assets/1ST_READ.BIN
go test ./...
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o SWIRL-Card-Manager.exe .
```

The Card Manager builds on its own too: a current menu build is already in `swirl/cardmanager/assets`, and
all Go dependencies are vendored. See [Building from source](docs/BUILDING.md).

## Project layout

```
.                       openMenu, with SWIRL added as a new UI mode
├── ui/ui_swirl.c       the SWIRL dashboard
├── ui/swirl/           SWIRL graphics, audio, library, VMU and fonts
├── backend/            game list, launching (GDMENU and CodeBreaker loaders)
├── swirl/cardmanager/  SWIRL Card Manager (Go, web UI in web/index.html)
├── swirl/vmucap/       Flycast patch for VMU screen capture (GPL 2.0)
├── swirl/tools/        logo, font generator, test library, image tools
└── docs/               guides and screenshots
```

## Credits

SWIRL and SWIRL Card Manager are made by **Glen Huszar** ([@TheGlengineer](https://github.com/TheGlengineer)).

They stand on the work of:

- [openMenu](https://github.com/mrneo240/openMenu) by **mrneo240** (Hayden Kowalchuk), the menu SWIRL is built on, and its [box art](https://github.com/mrneo240/openMenu_imagedb) and [game info](https://github.com/mrneo240/openMenu_metadb) databases
- **GDEMU** by Deunan, and the **GDMENU** loader by megavolt85
- [openMenu Virtual Folder Bundle](https://github.com/DerekPascarella/openMenu-Virtual-Folder-Bundle) by Derek Pascarella
- [KallistiOS](https://github.com/KallistiOS/KallistiOS) and the Dreamcast homebrew community
- [Flycast](https://github.com/flyinghead/flycast) by flyinghead, used for Preview and VMU capture
- [Redump](http://redump.org/) and [libretro-database](https://github.com/libretro/libretro-database) for game titles, [libretro-thumbnails](https://github.com/libretro-thumbnails/Sega_-_Dreamcast) for screenshots
- The Sora, Barlow and Silkscreen fonts, and the Go libraries listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)

## The SWIRL logo

The SWIRL mark is three tapered strokes turning around a hub, in the menu's orange: a nod to the swirl of
the Dreamcast era, drawn from scratch for this project. It is generated by `swirl/tools/make_logo.py`,
which also writes the app icon and the VMU logo. The files are in [docs/images/logo](docs/images/logo).

## License

SWIRL and SWIRL Card Manager are released under the [BSD 3-Clause license](LICENSE.md), the same license as openMenu.
The VMU capture tool in `swirl/vmucap` is GPL 2.0 because it is based on Flycast. Other included parts keep
their own licenses, listed in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

Sega and Dreamcast are trademarks of SEGA Corporation. This is an independent fan project, not affiliated with SEGA or with the maker of GDEMU.

# SD card layout

SWIRL uses the same layout as GDEMU, GDMENUCardManager and openMenu. A card set up by any of them works
with SWIRL, and SWIRL does not stop you from going back.

```
SD card
├── 01/                 the menu disc: SWIRL (GDI with 5 tracks)
├── 02/                 first game disc
├── 03/                 next game disc, and so on (no gaps: GDEMU stops at the first gap)
├── GDEMU.INI           GDEMU settings (optional)
├── SWIRL/              Card Manager's working files (never read by the Dreamcast)
└── SWIRL_BACKUP/       previous menus and removed games
```

## Game folders (02 and up)

Each game disc is one folder holding a `.gdi` with its tracks, or a `.cdi`, `.mds` or `.ccd` image.
Multi disc games use one folder per disc. SWIRL reads each disc's serial number from the image, which is
how art, info and names are matched.

Card Manager never changes the files inside a game folder once the game is on the card. Removing a game
moves its folder to `SWIRL_BACKUP` and renumbers the folders after it.

## The menu disc (folder 01)

Folder `01` is a small GD-ROM image that GDEMU boots first. Inside it:

| File | Used for | Needed |
|---|---|---|
| `1ST_READ.BIN` | The SWIRL program | Yes |
| `OPENMENU.INI` | The list of games: folder, name, serial, disc, region, VGA, date | Yes |
| `BOX.DAT`, `ICON.DAT` | Box art, large and small (openMenu format) | Optional |
| `BOX_EX.DAT`, `ICON_EX.DAT` | Extra art (openMenu format) | Optional |
| `META.DAT` | Descriptions, players, genres, accessories, VMU blocks, online (openMenu format) | Optional |
| `VMU.DAT` | The picture each game shows on the VMU | Optional, added by SWIRL |
| `SHOT.DAT` | Two screenshots per game | Optional, added by SWIRL |
| `BGM.ADP` | Menu music (AICA ADPCM) | Optional, added by SWIRL |
| `COLLECT.TXT` | Your own collections | Optional, added by SWIRL |
| `LOGO.VMU` | Your own VMU logo | Optional, added by SWIRL |
| `THEME/` | openMenu themes. SWIRL takes the Sega Dreamcast logo for its header from `THEME/NTSC_U/BG_U_L.PVR`; Card Manager adds that one file from openMenu's release when it is missing | Optional |
| `PELICAN.BIN` | CodeBreaker, for the "Play with CodeBreaker cheats" option | Optional |
| `BLEEM.BIN` | Bleem, for PlayStation discs | Optional |
| `GDEMUNFO.TXT` | Notes which tool made the disc | Optional |

Every optional file can be missing: SWIRL shows what it has and falls back gracefully.

## SWIRL folder

Card Manager keeps your edits here so they survive menu rebuilds and moving the card between PCs:

| File | Holds |
|---|---|
| `games.json` | Names, details and art choices you made in the Edit window |
| `art/` | Box art, VMU screens and screenshots you uploaded |
| `collections.json` | Your collections |
| `BGM.ADP` | Your own menu music, converted |
| `BGM.THEME` | Marks that you chose the SWIRL theme music |
| `card-id.txt` | A random ID so PC backups of this card are recognised |

## SWIRL_BACKUP folder

| Folder | Holds |
|---|---|
| `01_YYYYMMDD_HHMMSS` | A previous menu, kept by every install. **Backups > Restore** puts it back. |
| `removed_...` | Games you removed. Delete them from the Backups page to free the space. |

## What the Dreamcast writes

Nothing on the SD card. Favorites, history, launch options and settings go to your VMU as `SWIRL.DAT`.

# swirl/

Everything SWIRL adds outside the openMenu source tree. The dashboard itself is in `ui/ui_swirl.c` and `ui/swirl/`.

| Path | What |
|---|---|
| `cardmanager/` | SWIRL Card Manager, the Windows app ([guide](../docs/CARD_MANAGER.md), [developer notes](cardmanager/README.md)) |
| `setup_toolchain.sh` | Installs the prebuilt GCC 15.1 + KallistiOS 2.2.1 toolchain (checksum pinned) |
| `build.sh` | Clean build of the menu into `build/gdemu/1ST_READ.BIN` and `build/cdi/1ST_READ.BIN` |
| `make_test_gdi.sh` | Builds a Flycast test disc with a sample library |
| `tools/make_logo.py` | Draws the SWIRL logo and writes the app icon, favicon, VMU logo and README images |
| `tools/gen_fonts.py` | Generates the UI font atlases and the VMU font |
| `tools/make_test_library.py` | Generates a 25 game sample library with placeholder art |
| `tools/pvr2png.py` | Converts PVR textures from the art DATs to PNG |
| `assets/fonts/` | Sora, Barlow and Silkscreen (SIL Open Font License) |
| `vmucap/` | Patch to Flycast for capturing VMU screens (GPL 2.0) |

See [Building from source](../docs/BUILDING.md).

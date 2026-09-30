# Building from source

SWIRL has two parts that build separately:

| Part | Language | Toolchain | Output |
|---|---|---|---|
| The SWIRL menu | C | GCC 15.1 for SH4 + KallistiOS 2.2.1 | `1ST_READ.BIN` |
| SWIRL Card Manager | Go, with an HTML/JS UI | Go 1.21 or newer | `SWIRL-Card-Manager.exe` |

The menu binary is embedded in Card Manager (`swirl/cardmanager/assets/1ST_READ.BIN`). A current build is
committed, so you can work on Card Manager without the Dreamcast toolchain.

## The SWIRL menu

Linux or WSL2 with Ubuntu 22.04 (other distributions work if the prebuilt toolchain runs on them).

```sh
sudo apt install build-essential curl python3 python3-pil
swirl/setup_toolchain.sh             # downloads GCC 15.1 + KallistiOS 2.2.1 into ~/dreamcast (checksum verified)
swirl/build.sh                       # clean build
```

`build.sh` sources `~/dreamcast/kos/environ.sh` (set `KOS_ENV` to use another path), runs `make` and writes:

| File | For |
|---|---|
| `swirl/build/gdemu/1ST_READ.BIN` | GDEMU (unscrambled, GD-ROM boot). This is the one Card Manager embeds. |
| `swirl/build/cdi/1ST_READ.BIN` | Burning a CD-R test disc (scrambled) |

After changing a header, run `make clean` (the Makefile does not track header dependencies), or just use `build.sh`.

### Fonts

The UI fonts are compiled into the binary as `ui/swirl/assets/*.swf` (8 bit alpha glyph atlases) and the VMU
font as `ui/swirl/vmu_font.h`. To change them, edit `swirl/tools/gen_fonts.py` and run it (needs Pillow),
then rebuild.

### Where things are

| Path | What |
|---|---|
| `main.c` | Start up and the UI mode table (SWIRL is one mode next to openMenu's list, grid and GDMENU styles) |
| `ui/ui_swirl.c` | The SWIRL dashboard: tabs, game page, launch options, system settings, screen savers, intro |
| `ui/swirl/sw_gfx.c`, `sw_prim.c` | PowerVR drawing: textures, fonts, anti aliased shapes, glows |
| `ui/swirl/sw_lib.c` | Game library, collections, play stats and settings (saved to the VMU as `SWIRL.DAT`) |
| `ui/swirl/sw_audio.c`, `sw_spu.c` | Menu music and sounds, and pop free sound chip resets |
| `ui/swirl/sw_vmu.c` | VMU logo and text, and the animated VMU screen |
| `ui/swirl/sw_trace.c`, `sw_codes.h` | The trace, the warning codes, the hang watchdog and the QR report screen |
| `ui/swirl/sw_version.h` | The version shown in System > About SWIRL |
| `backend/` | Reading `OPENMENU.INI` and the DAT files, launching games |
| `texture/` | Texture cache for box art |

## Testing the menu in Flycast

```sh
swirl/make_test_gdi.sh               # builds swirl/build/flycast_test/disc.gdi
```

This builds a 5 track menu disc like a real card's, using a generated sample library of 25 games with
placeholder art (`swirl/tools/make_test_library.py`). Open `disc.gdi` in [Flycast](https://github.com/flyinghead/flycast).
Set Flycast's region to USA and cable to VGA. Launching games needs a real GDEMU.

Card Manager's **Health and preview > Open preview** does the same with your real card.

### The host parser check

`tests/host/run.sh` builds the `OPENMENU.INI` and DAT readers (`backend/gd_list.c`, `texture/dat_reader.c`) with
the address and undefined behaviour sanitizers on the host and runs them over every crafted bad file that
`tests/host/mkcases.py` makes (bad lines, stray slots, under reported counts, short DAT tables). It needs gcc and
python3 and a base library in `swirl/test`, and takes seconds; run it after touching either reader.

### The emulator harness

The launch, GDEMU, VMU and report paths were verified in a scripted Flycast harness with a simulated GDEMU and
VM2, driven by the `SW_TEST_*` defines listed in [DIAGNOSTICS.md](DIAGNOSTICS.md#test-defines) (`OPTIONS="-DSW_TEST_HANG"
swirl/build.sh`, then a menu disc from the build). It lives outside the repository; a build with a test define
is never the one in `swirl/cardmanager/assets`.

## SWIRL Card Manager

```sh
cd swirl/cardmanager
go test ./...
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o SWIRL-Card-Manager.exe .
```

All dependencies are vendored in `vendor/`, so no network is needed. It also builds and runs on Linux and
macOS (`go build .`) for development: the app serves its UI on a local port and opens it in a browser. Some
features (formatting cards, installing, updating, Preview and VMU capture) are built for Windows and macOS only.

Useful environment variables while developing:

| Variable | Effect |
|---|---|
| `SWIRL_NO_WINDOW=1` | Do not open a window; open the printed URL yourself |
| `SWIRL_1ST_READ=path` | Install this menu binary instead of the embedded one |
| `SWIRL_UPDATE_REPO=owner/name` | Check a different GitHub repository for updates |
| `SWIRL_UPDATE_API=url` | Use a test server instead of api.github.com |
| `SWIRL_PATCH_DIR=path` | Read the owner's VGA patch catalog from this folder instead of the data folder's `patches` |

### Updating the embedded menu

```sh
cp swirl/build/gdemu/1ST_READ.BIN swirl/cardmanager/assets/1ST_READ.BIN
```

The menu and Card Manager share one version number. `ui/swirl/sw_version.h` holds `2.14`, and `const version`
in `swirl/cardmanager/main.go` holds `2.14.x`. The tests fail if the embedded menu reports a different
major.minor. Build the embedded menu with `swirl/build.sh` from a clean tree, so its reports carry the git hash
and not `-dirty` or `dev`.

### The Mac app

On a Mac with Xcode and Go:

```sh
swirl/vmucap/build_macos.sh          # the patched Flycast (universal), once; about 20 minutes
cd swirl/cardmanager
macos/make_app.sh 2.14.0             # SWIRL Card Manager.app, plus the .zip and .dmg, in dist/
```

The app is the Go program built for Apple Silicon and Intel and joined with `lipo`, in a bundle made from
`macos/Info.plist.in` and `macos/AppIcon.icns`, with an ad hoc signature. It embeds the Flycast zip from
`assets/swirl-vmucap-macos.zip` (an empty placeholder in the repository, so a build without it simply
leaves out Preview and VMU capture). The Mac specific code is in the `*_darwin.go` files; the rules for
which disks may be formatted are in `macdisk.go`, tested on every platform.

Without a Mac, run **Actions > Mac preview build > Run workflow** on GitHub: it builds the same .dmg and
attaches it to the run.

### Windows resources

`rsrc_windows_amd64.syso` holds the icon, the version info and the manifest. After changing the version run:

```sh
sudo apt install binutils-mingw-w64-x86-64 gcc-mingw-w64-x86-64   # windres uses the C preprocessor
sh winres/make.sh 2.14.0
```

### Dependencies

`third_party/rardecode` is patched (see `third_party/README.md`). To change dependencies, run
`third_party/fetch-mirrors.sh` once, then `go mod tidy` and `go mod vendor` as usual.

### Tests

`go test ./...` runs everything that needs no outside files: card scans, installs, archives (zip, 7z,
solid rar), names, disc sets, backups and their manifests, VGA patches, the report decoder, music conversion,
updates against a fake GitHub server and more. The fixture helpers (`makeMenuIn01`, `ipSector`
in `txn_test.go`, `swirl_test.go` and `menubackup_test.go`, with `buildMenuDisc` from `card.go`) build a small card in a temporary folder for a test.
A few tests only run when you point them at real data (see the comment at the top of each):

| Variable | Test |
|---|---|
| `SWIRL_FIXTURES` | Builds a folder of sample downloads for trying the file browser |
| `SWIRL_CARD` | Reading art from a real card |
| `SWIRL_DB_DIR` | Merging a downloaded openMenu database |
| `EXTRAS_CARD`, `EXTRAS_TUNE` | Music and extras on a real card |
| `LOGO_PIC` | VMU logo conversion |

### The VMU capture tool

`assets/swirl-vmucap.exe.gz` is Flycast with the patch in `swirl/vmucap/`, cross compiled with mingw-w64.
The README there has the exact commit and build flags. It is GPL 2.0.

## Continuous integration

`.github/workflows/ci.yml` runs on every push and pull request: it builds the menu with the same toolchain,
warns if the committed `1ST_READ.BIN` differs from a fresh build, runs the Card Manager tests and builds the
Windows exe. See [Releasing](RELEASING.md) for the release workflow.

# Contributing

Thanks for wanting to help. Bug reports, testing on real hardware, ideas and pull requests are all welcome.

## Reporting bugs

Use the [bug report form](https://github.com/TheGlengineer/SWIRL/issues/new/choose). Please include:

- The versions: **System > About SWIRL** on the Dreamcast, and **About > Copy details** in Card Manager
- Your GDEMU (original or clone, firmware if you know it) and video cable (VGA, composite, HDMI adapter)
- What you did, what happened, and what you expected
- For a game that will not start: its name, serial and image format (.gdi or .cdi), and which launch options you tried

Missing or wrong box art and game info come from the openMenu databases. Report those to
[openMenu_imagedb](https://github.com/mrneo240/openMenu_imagedb) or [openMenu_metadb](https://github.com/mrneo240/openMenu_metadb).

## Working on the code

- [Building from source](docs/BUILDING.md) covers both toolchains and the tests.
- Keep the SD card layout and the openMenu file formats working. Cards made by GDMENUCardManager and openMenu must keep working, and new files must be optional.
- The menu must hold 60 fps on real hardware. Test with Flycast, and on a Dreamcast when you can.
- Card Manager must never change a game folder's files, and must never touch anything outside the card and its own data folder without the user asking.
- Run `go test ./...` in `swirl/cardmanager` and build the menu before opening a pull request. CI does both.
- C code follows `.clang-format`. Go code is `gofmt`ed.
- UI text is plain English, short and specific. Avoid jargon where a plain word works.

## Pull requests

1. Fork the repository and create a branch.
2. Make the change, with tests for Card Manager changes where practical.
3. Update the docs and add a line to `CHANGELOG.md` under an `## [Unreleased]` heading.
4. Open the pull request and describe what you tested (emulator, hardware, which GDEMU).

By contributing you agree that your contribution is released under the project's [BSD 3-Clause license](LICENSE.md).

## Forking

You are welcome to fork SWIRL and make it your own. Please keep the credits (openMenu, GDEMU and the
others in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)) and the license notices. Your fork's release
workflow builds Card Manager so that it checks your fork for updates, not this repository.

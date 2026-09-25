# swirl-vmucap

A patched build of the Flycast Dreamcast emulator (GPL-2.0, https://github.com/flyinghead/flycast,
based on commit 869038f40ac8cddc7741c3a35d545de057cc0dd5) used by SWIRL Card Manager to capture the
picture each game draws on the VMU screen.

Changes (vmucap_flycast.patch plus vmucap.cpp, which goes in core/):
- When FLYCAST_VMUCAP=<file> is set, every VMU LCD write is logged (u64 SH4 cycles, 4 byte port, 192 raw bytes).
- Start is pressed at the seconds listed in FLYCAST_VMUCAP_START, and A at those in FLYCAST_VMUCAP_A; the
  emulator exits after FLYCAST_VMUCAP_SECS seconds of emulated time.
- In automatic capture mode the window is hidden and the null audio backend does not throttle speed.
  FLYCAST_VMUCAP_MANUAL=1 logs while a person plays in a normal window (Capture by playing).
- FLYCAST_AUDIODUMP=<file> writes the null audio backend's output to a raw 16 bit stereo file (used to
  test SWIRL's menu sound).
- vmucap.cpp is compiled into every desktop build, including the macOS app bundle.

Windows build: cross-compiled with mingw-w64 (posix threads), static, with
-DUSE_VULKAN=OFF -DUSE_DX9=OFF -DUSE_DX11=OFF -DUSE_LUA=OFF -DUSE_BREAKPAD=OFF, Spout disabled.
The binary is embedded in SWIRL Card Manager (cardmanager/assets/swirl-vmucap.exe.gz).

macOS build: `build_macos.sh` on a Mac with Xcode (the release workflow does this on GitHub's macOS
runners). It builds a universal Flycast.app (Apple Silicon and Intel, macOS 10.15 or newer) with OpenGL
only, and zips it to cardmanager/assets/swirl-vmucap-macos.zip, which the macOS Card Manager embeds.

This tool is GPL 2.0, like Flycast. Its source is this folder plus the Flycast commit named above.

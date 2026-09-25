# swirl-vmucap

A patched build of the Flycast Dreamcast emulator (GPL-2.0, https://github.com/flyinghead/flycast,
based on commit 869038f40ac8cddc7741c3a35d545de057cc0dd5) used by SWIRL Card Manager to capture the
picture each game draws on the VMU screen.

Changes (vmucap_flycast.patch plus vmucap.cpp):
- When FLYCAST_VMUCAP=<file> is set, every VMU LCD write is logged (u64 SH4 cycles, 4 byte port, 192 raw bytes).
- Start is pressed at the seconds listed in FLYCAST_VMUCAP_START; the emulator exits after FLYCAST_VMUCAP_SECS.
- In capture mode the window is hidden and the null audio backend does not throttle speed.

Windows build: cross-compiled with mingw-w64 (posix threads), static, with
-DUSE_VULKAN=OFF -DUSE_DX9=OFF -DUSE_DX11=OFF -DUSE_LUA=OFF -DUSE_BREAKPAD=OFF, Spout disabled.
The binary is embedded in SWIRL Card Manager (cardmanager/assets/swirl-vmucap.exe).
If SWIRL is ever shared, this source must be shared with it (GPL).

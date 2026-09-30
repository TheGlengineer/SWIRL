# SWIRL apps platform

The design for letting SWIRL run community apps, widgets and themes without any of them being able to break the menu, and for Card Manager to browse, check and install them. This document covers the framework only: the runtime, the SDK, the package format, the catalog and the Card Manager screens. The apps themselves (music, save vault, achievements and so on) are separate items built on top of it.

Written 2026-09-28 from Glen's design discussion with Claude. Backlog entries: G10 to G23, decisions D16 to D22. Mockup: the "SWIRL App Store" canvas (claude.ai/artifact/FxKnJNXCNWUMiAY4GiHf1t), screens "Apps tab with memory budget" and "App detail and package files". Icon set: `swirl-app-icons.zip` (9 sample icons, SVG and 256 x 256 PNG, same tile style as the Card Manager icon).

Status: design only. Nothing here is built. Features are paused for hardening (D12); this work starts after 2.15. Section 13 (2026-09-29) lists the adjustments that came out of the audit validation; where it conflicts with an earlier section, section 13 wins.

## 1. Goals and rules

- Anyone can build an app with an SDK, and it can be added to a card without changing SWIRL's code.
- An app can never crash the menu, lose a save, or stop a game from launching. If an app misbehaves, SWIRL closes it and shows the report screen (T6), and the menu carries on.
- Every invariant in BACKLOG section 1 holds. In particular: apps live only in folder 01 and the `SWIRL/` namespace (invariant 1), app data never touches SWIRL.DAT or OPENMENU.CFG (invariant 2), a missing or damaged app file shows a fallback (invariant 10), every format is versioned from day one.
- Same release bar as the rest of SWIRL: no user caveats, works on any card and any memory card type, or it doesn't ship.

## 2. Three kinds of extension (D16)

| Kind | What it is | Can it break the menu | Examples |
|---|---|---|---|
| Theme | Data only: colours, fonts, layout values, sounds, pictures | No, it is never executed | Neo Tokyo 2001 |
| Script app, widget or service | Lua code run inside SWIRL's sandbox with a memory cap, a CPU watchdog and error trapping | No, the sandbox closes it | Dream Radio, Clock and Weather, Achievements |
| Native app | Its own disc image, launched like a game; SWIRL is not running while it runs | No, SWIRL is not in memory | Keyboard Quest |

Why an interpreter: KOS has no memory protection, so native code loaded into SWIRL could overwrite anything. Lua is small, proven on embedded hardware, and lets SWIRL control every allocation through its own allocator function (`lua_newstate` with a capped allocator). The choice of Lua is confirmed by the G12 spike, not assumed.

Script extensions come in three shapes:

- **App:** full screen, opened from the menu, one at a time.
- **Widget:** a small tile on the dashboard and optionally the VMU screen, loaded with the menu.
- **Service:** no screen of its own, loaded with the menu (for example reading VMU saves for achievements).

## 3. Package format (G11)

One folder per app, delivered as a zip with a fixed extension (working name `.swirlapp`):

| File | Required | Notes |
|---|---|---|
| `manifest.json` | Yes | Versioned schema, see below |
| `icon.png` | Yes | 256 x 256, shown in Card Manager |
| `icon.pvr` | No | 64 x 64 menu icon. Card Manager builds it from `icon.png` on write if missing (same code path as art today, `pvr.go`) |
| `icon_vmu.bin` | No | 32 x 32 one bit picture for the VMU screen |
| `main.lua` and other `.lua` | Script kinds | Entry point named in the manifest |
| `assets/` | No | Pictures, sounds, fonts the app loads through the SDK |
| `disc.gdi` or `.cdi` and tracks | Native kind | The disc image |
| `screenshots/` | No | For the catalog, 640 x 480 |

Example manifest:

```json
{
  "manifest": 1,
  "id": "com.brushdc.pixelpaint",
  "name": "Pixel Paint",
  "version": "1.0.2",
  "sdk": 2,
  "kind": "app",
  "entry": "main.lua",
  "author": { "name": "brushdc", "url": "https://github.com/brushdc" },
  "memory": { "main_kb": 5734, "video_kb": 1843, "sound_kb": 205 },
  "needs": {
    "all": ["storage.serial_sd"],
    "any": [],
    "optional": ["input.mouse"]
  },
  "permissions": ["storage.sd"],
  "min_swirl": "2.17.0"
}
```

- `manifest` is the schema version. Card Manager and SWIRL keep a reader for every version ever shipped.
- `id` is reverse domain and never changes. App data lives under `SWIRL/apps/<id>/` on the serial SD, or in a per app VMU file.
- `needs.all` must all be present; each entry in `needs.any` is a list where one must be present; `needs.optional` improves the app but isn't required. IDs come from the capability table (section 6).
- `memory` is enforced, not trusted (section 5).
- `permissions` is what the SDK lets it touch (storage, network, VMU screen, VMU saves read only, and so on). Shown to the user in plain words before install.

## 4. The runtime in SWIRL (G12) and the SDK (G13)

**Lifecycle**

- Themes load at start instead of the built in theme; a broken theme falls back to the default (invariant 10).
- Widgets and services load after the game list, within their declared budget.
- Apps open from an Apps row in the menu, run full screen, and return to the menu on exit.
- **Before any game launch, SWIRL closes every app, widget and service**, giving each a short, bounded chance to save, through the same hook that stops music today (`gdemu_before_launch` in `backend/gdemu_control.c`). The launch path itself does not change (D8).
- In game reset boots SWIRL fresh; nothing from before the game is still in memory. Apps must save anything they want to keep.

**Sandbox**

- Capped allocator per Lua state. Going over the declared `main_kb` fails the allocation, the app gets a Lua error, SWIRL closes it.
- Instruction count hook as a CPU watchdog, so an endless loop can't freeze the menu.
- Every error is trapped, logged through `sw_trace` and reported with a `sw_warn` code (T6), so a crash in an app produces the same QR report as any other failure, naming the app and its version.
- No access to KOS, raw memory, maple frames or the GDEMU from Lua. Everything goes through the SDK.

**SDK surface (first version)**

| Module | What it gives |
|---|---|
| `swirl.ui` | Draw pictures, text, rectangles; SWIRL's fonts and theme colours; standard dialogs |
| `swirl.input` | Controller, keyboard, mouse, light gun events in one model |
| `swirl.audio` | Play sound effects and streams within the app's sound budget |
| `swirl.vmu` | Draw on the VMU screen; read save file lists and headers (read only) |
| `swirl.storage` | Read and write the app's own folder on the serial SD, or its own VMU file |
| `swirl.net` | HTTP through the modem or BBA, only with the network permission |
| `swirl.system` | Hardware present, SWIRL version, language, time |

The SDK has its own version number (`sdk` in the manifest). An app built for SDK 1 keeps working on every later SWIRL; an app needing a newer SDK is shown as needing a SWIRL update.

## 5. Memory (G19, D19)

- Dreamcast pools: 16 MB main RAM, 8 MB video RAM, 2 MB sound RAM. Each is checked on its own.
- Apps open one at a time. Widgets, services and the theme stay loaded. **The worst case is SWIRL's own use + everything always loaded + the largest app,** per pool. That is the number Card Manager checks, not the sum of every installed app.
- Everything is freed when a game starts (section 4). Apps never cost a game anything. Native apps don't count, because SWIRL isn't running while they run.
- **SWIRL's own use, measured (T15, 2026-09-30, 2.15 harness build, Flycast, "before launch" after the Library grid has loaded covers).** The mockup's figure (SWIRL 9 MB) was a placeholder; these are the numbers.

| Games on the card | Heap grown (KB) | Main RAM left below the loader ceiling | Video RAM free, High | Video RAM free, Standard | Sound RAM free |
|---|---|---|---|---|---|
| 25 | 1316 | 10.8 MB | 1148 KB + 384 KB of the 1 MB texture scratch | 1837 KB + 384 KB | 1856 KB of 2048 |
| 46 (Glen's card) | 1416 | 10.7 MB | same | same | same |
| 250 | 1328 | 10.8 MB | same | same | same |
| 1000 | 1464 | 10.7 MB | same | same | same |

  What it means: main RAM is not the constraint. 1000 games cost about 150 KB more heap than 25 (the INI slot table and the 20 byte `sw_game` entries), and over 10.5 MB stays free below the ceiling whatever the card. Video RAM is the tight pool: with High picture quality about 1.5 MB is free (1.1 MB outside the scratch plus what the scratch has left), with Standard about 2.2 MB; the difference between the modes is 690 KB, not the 3 MB first assumed, because the anti aliased buffers are 640 x 480 (the 1280 wide figure was wrong) and the opaque bin sizes differ. Sound RAM has 1.8 MB free with the menu music streaming. The game count does not move the video or sound figures at all; covers live in the fixed 1 MB scratch.
- Consequences: the apps main RAM budget can be a flat figure (about 10 MB minus a margin) with no per card prediction; the video budget is per quality mode and small, so widgets and always loaded pieces must be frugal with textures; and raising the game limit (I6) costs 20 KB of static RAM per 1000 games, so the limit is boot time and the INI parser, not memory.
- A small safety margin stays free in main RAM: 512 KB, taken from the measurement (the heap grows about 150 KB between boot and the first launch on a large card).
- Declared numbers are enforced by the runtime cap and measured in CI (section 8), so a wrong declaration fails review instead of failing on a user's console.

## 6. Hardware capabilities (G16, G17, D20)

One capability ID table, shared by SWIRL (C), Card Manager (Go) and the catalog CI, in the same way as the serial table (T12).

| ID | Hardware | How SWIRL finds it |
|---|---|---|
| `net.modem` | Built in modem | Detected |
| `net.dreampi` | DreamPi on the modem line | User says so (a DreamPi looks like any phone line) |
| `net.bba` | Broadband Adapter | Detected |
| `card.vmu` | Standard VMU (screen) | Detected, maple LCD function |
| `card.vm2` | VM2 | Detected, ALLINFO name "VM2 by Dreamware" (already read for Game ID, I4) |
| `card.vmupro` | VMU Pro | Detected, ALLINFO name "8BITMODS VMUPro " |
| `card.nolcd` | Memory card with no screen (4X and similar) | Detected, memory card without LCD function |
| `storage.serial_sd` | Serial SD card reader | Detected by probing (F5 code) |
| `input.keyboard`, `input.mouse`, `input.lightgun`, `input.stick`, `input.mic` | Peripherals | Detected, maple function codes |
| `video.vga` | VGA box or HDMI mod | Detected, cable check |
| `board.gdemu` | GDEMU | Always present |

Correction to the earlier discussion: VM2 and VMU Pro do not need the user to switch them on; SWIRL can tell them apart today. Only the DreamPi has to be declared by the user.

**Getting the profile to Card Manager.** Card Manager can't see the console, and the console can't write the SD card. SWIRL gets a My Dreamcast screen that detects everything above and shows a short code and a QR code (same pattern as the report screen). The user types or scans it into Card Manager's My Dreamcast section, then switches on anything that can't be detected. The profile is stored on the PC and in `SWIRL/` on the card so it travels with the card.

**Rule to respect:** in the Virtual Folder Bundle, serial SD and Game ID can't be on together (F5 forward note). If SWIRL keeps that rule, `storage.serial_sd` apps and VM2 / VMU Pro Game ID conflict, and the Apps tab must explain it.

## 7. Card Manager screens (G17, G18)

Mockup screens on the canvas. Both use Card Manager's existing styles and fonts from `web/index.html`.

**My Dreamcast** (new nav item): switches grouped as Online, Memory cards, Storage, Controls, Video and mods; "Found by SWIRL" badge on detected items; code entry; a panel with how many catalog apps the console runs and which hardware would unlock the most.

**Apps** (new nav item):

- Stats: works on your Dreamcast, installed and updates, worst case main RAM, space on the card.
- Filters: All, Works on my Dreamcast, Apps, Widgets and services, Themes, Own disc. Search.
- Each app: icon, name, trust badge (Official, Verified, Community), kind, description, a chip per hardware need (have, missing, optional), memory figures, author.
- Actions in order of checks: Remove and Update when installed; "Needs hardware" with "Your Dreamcast is missing: ..." when a required capability is absent; "Won't fit" with the pool and amount when the memory check fails; otherwise Add.
- Installed apps whose hardware has gone are flagged "won't run without ...".
- Memory budget panel with one bar per pool (SWIRL, always loaded, largest app, safety margin).
- Ready to write panel: staged adds, removes and updates. Nothing touches the card until Write to card, which goes through the `cardFS` journal like every other card operation (D14).
- Detail page: screenshots, hardware needs with reasons, permissions in plain words, memory, package contents, manifest.

## 8. Catalog and how apps arrive (G20, D18)

Same pattern Card Manager already uses for art (`onlinedb.go`, GitHub Releases), updates (`updates.go`) and thumbnails (`shots.go`). No server to run.

1. Developer builds with the SDK and tests in SWIRL Player (G21).
2. Pull request to a catalog repo (working name `TheGlengineer/swirl-apps`): one folder per app with manifest, icon, screenshots and a link to the package on the developer's own GitHub Release, plus its SHA-256.
3. CI on the pull request: manifest schema, capability and permission IDs known, hash matches, icon sizes, headless run in SWIRL Player with the real memory peak recorded and compared with the declaration. Failures never reach review.
4. Glen reviews and merges. Trust tier: Community on first merge, Verified after review and hardware test, Official for Glen's own.
5. Release job builds one signed `catalog.json` (every app's summary, version, hash, needs, memory, icon URL, tier) and a revoked list, published on the catalog repo's Releases. Signing shares its key design with CM-21 (updater signing).
6. Card Manager fetches `catalog.json` on open, checks the signature, caches it on the PC, works offline from the cache.
7. Packages download only at Write to card: hash checked, PVR icon built, app placed in folder 01's `SWIRL/apps/`.

**Updates:** a pull request bumping version and hash, same checks. **Revocation:** an ID on the revoked list is flagged in Card Manager and removed at the next write with the user's OK. **Beta channel:** optional `catalog-beta.json`, turned on in Card Manager settings, same model as the preview channel (D6). **Sideloading (G22):** drag a package onto the Apps tab; shown as Unverified with a warning; still goes through hardware, memory and hash checks.

## 9. Developer kit (G21, G23)

- **SWIRL Player:** a PC program that runs script apps against the same SDK with the same memory caps, draws a 640 x 480 window and a VMU screen, and reports peak memory per pool. Runs headless in the catalog CI. Built on the SDK's host implementation, not on Flycast (Flycast stays the whole menu test harness).
- **Template and docs:** a starter app, a widget and a theme template, the SDK reference, the manifest schema, the capability table, the review rules, and how to submit.

## 10. Native apps (G15)

Native apps are disc images, so they need a GDEMU folder. Invariant 1 says 02 and up are games with no gaps, and other menus would list a native app as a game. Options to decide before G15 is built: (a) native apps are just games with an `apps` tag, shown in an Apps collection in SWIRL and as games elsewhere; (b) a reserved range at the end of the card. Option (a) keeps invariant 1 untouched and is the working proposal.

## 11. Build order

| Phase | Items | Depends on |
|---|---|---|
| 1. Foundations | G11 manifest and package format, G16 capability table and detection, SWIRL memory measurement for G19 (both picture quality modes, per game count), G12 runtime spike (Lua in SWIRL, capped allocator, watchdog, trapped errors) | 2.14.0 hardening (SW-4 memory ceiling, T6 diagnostics codes, OM-6, OM-14), T11 string table |
| 2. Platform | G12 runtime complete, G13 SDK v1, G14 widgets, services and themes, G21 SWIRL Player, G23 template and docs | Phase 1 |
| 3. Store | G17 My Dreamcast, G18 Apps tab, G19 memory check, G20 catalog repo and CI, G22 sideloading | Phase 2, WS-A `cardFS` journal, CM-21 signing, T9 incremental rebuild |
| 4. Native | G15 native apps | Decision on section 10 |

The first first party apps (candidates: G1 music, save vault from F5, G3 achievements, F11 dashboard widgets) come after phase 3 and are their own backlog items.

## 12. Open questions

- Add anyway: should a user be able to add an app whose required hardware is missing (bought, not arrived)? Mockup says no.
- Lua or another interpreter: confirm with the G12 spike (size, speed on the SH4, allocator control).
- Answered 2026-09-30 (section 5): about 10.7 MB of main RAM whatever the card; 1.5 MB (High) or 2.2 MB (Standard) of video RAM; 1.8 MB of sound RAM.
- Serial SD and Game ID together: keep the Bundle's rule or not?
- Native apps: option (a) or (b) in section 10.
- Catalog repo name and who besides Glen can review.

## 13. Adjustments from the 2.13.4 and 2.14.0 findings (2026-09-29)

Recorded after the audit validation (AUDIT section 11). Each changes a section above.

1. **Memory ceiling and pools (sections 5, 12; G19, D19).** Apps do not get 16 MB minus SWIRL. SWIRL's memory must end below 0x8CCFFF00 (the loader's parameter block, SW-4) with the KOS stack above it, so everything from the binary at 0x8C010000 to that line, about 12.9 MB, is the whole main RAM pool for code, BSS, heap and apps. The "73 KB free on a 1000 game card" figure in G19 was mallinfo's free space inside the heap grown so far (915 KB), not free RAM; the right number is `sbrk(0)` against the ceiling, which the launch trace already prints. Video RAM: High picture quality runs 1280 x 480 32 bit double buffered with anti aliasing, roughly 4.9 MB of the 8 MB before any texture, so the video budget depends on the quality setting and is measured in both modes. Sound RAM is 2 MB minus the music stream buffers. The measurement (phase 1) can run on the cloud Flycast harness now.

2. **Picture quality and the app budget (D22).** Quality is fixed when the graphics chip is initialised at boot, so it stays a boot time choice and is never switched per app. System > Picture quality shows what each mode leaves for apps ("High: leaves 1.9 MB of video RAM for apps", "Standard: leaves 4.9 MB"; High costs more because its anti aliased 32 bit frame buffers are larger, so less is left; figures are illustrative until T15 measures them). Card Manager's memory panel has a High and a Standard column per pool; each app shows "Fits on High" or "Needs Standard"; the console's current setting comes from the My Dreamcast profile. An app that only fits on Standard can be installed and is greyed in SWIRL with "Needs Standard picture quality" until the user switches. Apps declare needs and never change the setting (invariant 5). Widgets, services and themes are always loaded, so they must fit on High or the card cannot enable them.

3. **No text parsing on the console (sections 3, 4; G11, G12).** SWIRL never reads `manifest.json` or any text file for apps. Card Manager compiles the installed apps into a binary, versioned, bounds checked index (`APPS.DAT`, same header shape as the other DATs) read by the hardened DAT reader; the reader joins the host fuzz suite (T4). The INI runs (OM-2, OM-8: crash, boot hang, games silently gone) are the reason.

4. **On disc names (section 3; G11).** The ISO writer (`isowrite.go`) upper cases, maps every character outside A to Z, 0 to 9, `_` and `.` to `_`, writes no Joliet names and does not check for collisions, so `com.brushdc.pixelpaint` or any ID with a dash mangles or collides silently. Rule: the on disc folder is `APPS/` plus a short hash of the ID, the index maps ID to folder, and Card Manager rejects package file names that are not 8.3 safe at package check.

5. **Runtime lifecycle (section 4; G12, D21).** The preview.2 launch crash was one live thread at `arch_exec`. The runtime has no threads of its own; the before launch hook closes, joins and frees everything, and the SW-4 ceiling check runs on the result. Apps load their assets when opened, behind a loading state, never per frame: a main thread SD read drops input (L12) and a read error while the disc is switching is the SW-5 deadlock. SDK file operations carry timeouts.

6. **Themes follow the I1 rule (section 2; G14).** A theme choice is saved to the VMU only after the theme has drawn its first frame; at boot theme files are validated by size and header (not existence only, OM-5) and a failure falls back to the default; hold Y at power on resets the theme as well as the style. OM-6 (theme path) and OM-14 (texture allocator uploads before checking space) are prerequisites.

7. **App data on the VMU (section 4; G13).** SWIRL.DAT and OPENMENU.CFG already take 4 blocks, the full card path is weak (SW-2), and with Game ID on a VM2 or VMU Pro switches cards per game, so app data on the VMU lands on whichever card is current. Any app VMU write goes through the one save worker (L2), the same space check, and a small per app block cap; the serial SD is the preferred store and the SDK docs say so.

8. **Rebuild cost (section 7; G18).** Every app add or remove is a full 01 rebuild today (audit M13) and 01 grows with each app. T9 (skip or incremental rebuild) becomes a phase 3 prerequisite, and Card Manager checks capacity against the GD high density area as 01 grows.

9. **Diagnostics and detection (sections 4, 6; T6, G16).** The production watchdog knows an app is running: the Lua instruction hook trips first, the 12 s watchdog second; the report names app ID and version; a code range is reserved for the runtime. The ALLINFO capability read reuses I4's frame handling, including the KOS resend lesson (L9), and never runs at power on.

10. **Small.** The My Dreamcast QR is drawn as a texture while the menu runs (the report screen writes the frame buffer only because the graphics chip is stopped). `min_swirl` comparison handles preview versions with a dash (D6). `swirl.net` inherits the timeout rules F7 and G3 settle on.

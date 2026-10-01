# SWIRL diagnostics: warnings, the hang watchdog and the QR report

SWIRL keeps a trace of what it did since power on (64 lines in memory, also on the serial port), counts
warnings, and can show a report as QR codes. Card Manager's report decoder reads the text below.

## When a report is shown

| Reason | R | When |
|---|---|---|
| Crash | R1 | A CPU exception. The screen stays until power off. |
| Hang | R2 | The menu made no progress for 12 s (no frame drawn, no trace line). Every build has this watchdog. Known long waits (a launch, the exit to the BIOS) stand it down. A hang with interrupts off cannot be caught; that is only the hand over to a game. |
| Diagnostics | R3 | The user pressed A on System > Diagnostics. B returns to the menu. |
| Boot log | R4 | X was held at power on. Shown once the menu has loaded, before its first picture. B continues. |
| Assert | R5 | A KallistiOS assert. Stays until power off. |
| Abort | R6 | abort(). Stays until power off. |
| Launch | R7 | The GDEMU did not answer and the menu disc did not come back. Stays until power off. |

On a screen that stays (R1, R2, R5, R6, R7) the codes take turns every 6 s (a single code stays for a minute
before it is drawn again). On the screens the user opened (R3, R4) A turns the page, B leaves, and a page turns
by itself after 8 s. The plain text above the code is red on a screen that stays and black on one the user
opened; the last line says "Then switch off. Hold Y at power on for SWIRL." or "A: next code   B: back to the menu".

## The QR codes

- Each code is plain text, at most 500 bytes of payload, QR error correction M, at most version 25.
- The first line of every code is `SWIRL <version> <part>/<parts>`, for example `SWIRL 2.14 2/3`, with the menu
  version (major.minor). Parts are numbered so they can be scanned in any order.
- The payload is the report text below, cut at line ends; the parts joined in order give the whole text.
- At most 12 parts (6000 bytes). A longer trace is cut at the start: the header and the latest steps stay.
- The first line of the report is also drawn as plain text above the code, so a photo that cannot be scanned
  still gives the version, the build, the reason and the first warning.

## The report text

```
SWIRL <full version> <build> R<reason>
b=<bios> r=<region> c=<cable> v=<width>x<height>/<pixel mode> m=<free KB>/<arena KB> u=<uptime s> n=<games>
d=<port><unit>:<device name>,...
w=W<code>x<count>,...
x=<pc> <pr> <stack address>...
t=
<ms> <trace line>
<ms> <trace line>
...
```

| Field | Meaning |
|---|---|
| full version | Card Manager's version string, for example `2.14.0`, read from `main.go` by `swirl/build.sh` |
| build | The short git hash the menu was built from, `-dirty` when uncommitted changes were built in, `dev` outside `swirl/build.sh` |
| R | The reason number from the table above |
| b | The console's BIOS version string (5 characters at 0x8c0007CC; `?????` when not printable, as in emulators) |
| r | `flashrom_get_region()`: 0 Japan, 1 USA, 2 Europe |
| c | `vid_check_cable()`: 0 VGA, 2 RGB, 3 composite |
| v | Video mode: width x height and the KallistiOS pixel mode (1 RGB565, 2 RGB888 packed) |
| m | Memory: free KB and arena KB from `mallinfo()` for R3 and R4; for a stopped menu (R1, R2, R5 to R7) free is 0 and arena is the heap size from `sbrk`, since walking the allocator's lists after a crash is not safe |
| u | Seconds since power on |
| n | Games in the library |
| d | Every maple device: port letter A to D, unit 0 to 5, the product name (up to 20 characters, trailing spaces removed) |
| w | The warning counters, in the order the warnings first happened; empty when there were none |
| x | Only for a crash or a hang: the program counter, the return address, then up to 16 code addresses found on the stack (for `sh-elf-addr2line -e themeMenu.elf` with the matching build). Card Manager's Report page names the functions itself when it has the build's ELF: `themeMenu.elf` dropped on the page, or `SWIRL-<version>-symbols.elf` from the release next to the exe or in the `symbols` folder under its data folder; it checks the build id in the file's `.rodata` before trusting it |
| t | The trace: the first line (the version) and the latest 39, each as milliseconds since power on, a space, the text |

A warning appears in the trace as `W<code> <detail>`, for example `W04 OPENMENU.INI: bad line 85 skipped`.

## Warning codes

The table lives in `ui/swirl/sw_codes.h` (the menu) and `swirl/diag/codes.tsv` (Card Manager). A code is never
reused, only added.

| Code | Key | Plain words |
|---|---|---|
| W01 | save-failed | A save to the VMU failed |
| W02 | swirl-dat-damaged | SWIRL.DAT on the VMU was damaged |
| W03 | picture-skipped | A picture on the SD card could not be used |
| W04 | ini-line | OPENMENU.INI has lines SWIRL could not use |
| W05 | gdemu-no-answer | The GDEMU did not answer |
| W06 | launch-returned | A game did not start and SWIRL came back |
| W07 | late-card | A memory card answered after start up |
| W08 | memory-line | SWIRL's memory reached the loader's line |
| W09 | cfg-damaged | OPENMENU.CFG on the VMU was damaged |
| W10 | meta-dat | META.DAT could not be used |
| W11 | theme | A theme file on the SD card could not be used |
| W12 | multidisc | A game has more discs than SWIRL shows |
| W13 | vmu-full | No room on the VMU for SWIRL's save |
| W14 | dat-file | A DAT file on the SD card could not be used |
| W15 | video-memory | Not enough video memory for the pictures |
| W16 | controller-scan | A controller port did not answer at start up |
| W17 | startup-step | A start up step failed |
| W18 | style-files | The saved style needs theme files this disc lacks |
| W19 | disc-read | The menu disc could not be read |
| W20 | language-file | LANG.DAT could not be used, showing English. Run Update SWIRL in Card Manager so the card gets a language file made for this menu |

System > Diagnostics lists the warnings since power on in these words, with the count and the first detail
(six to a page, Up and Down scroll), and the About card on the System tab says how many there are. Card
Manager's Report page shows the same words for each `w=` entry.

## Privacy

The report holds no game names except in the last trace lines (a launch line names the disc's product id).
The Diagnostics screen says so.

## Where it is made

`ui/swirl/sw_trace.c`: the trace, `sw_warn`, the watchdog thread (32 KB of stack, wakes every 500 ms), the
report text and the QR screen. `ui/swirl/sw_codes.h`: the codes. `ui/ui_swirl.c`: System > Diagnostics and
the boot log call. `main.c`: X held at power on. `swirl/build.sh`: the build id.

## Test defines

`SW_TEST_CRASH` (an exception after start up), `SW_TEST_HANG` (a spin after start up, the watchdog fires),
`SW_TEST_GDEMU_SILENT` (a launch whose image change is never answered), `SW_TEST_BIG_HEAP` (a launch with the
heap past the loader's line), `SW_TEST_LATE_VMU` (memory cards invisible for 12 s), `SW_TEST_DISC_CHG` (a read
that answers "disc changed" 15 s after power on).

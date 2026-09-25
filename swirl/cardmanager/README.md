# SWIRL Card Manager: developer notes

A Go program with an embedded web UI (`web/index.html`, plain HTML, CSS and JavaScript, no build step). It
serves the UI on `127.0.0.1` with a random port and a per run token, and opens it in an Edge or Chrome app
window. User documentation is in [docs/CARD_MANAGER.md](../../docs/CARD_MANAGER.md).

## Build and test

```sh
go test ./...
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui -s -w" -o SWIRL-Card-Manager.exe .
SWIRL_NO_WINDOW=1 go run .          # development on any OS: open the printed URL yourself
```

## Source map

| File | What |
|---|---|
| `main.go` | Flags, the HTTP API and start up |
| `card.go` | Scanning a card, installing SWIRL (building the menu disc), extras (music, logo, collections) |
| `disc.go`, `discfs.go`, `isowrite.go` | Reading disc images and IP.BIN, reading and writing the menu disc's ISO filesystem |
| `dat.go`, `pvr.go` | openMenu DAT files and PVR textures |
| `manage.go`, `newcard.go` | Adding, removing and reordering games, copying, new cards, background jobs |
| `archive.go` | .zip, .7z and .rar listing and unpacking |
| `browse.go`, `drives_*.go` | The file browser |
| `titles.go`, `discsets.go` | Proper names from the Redump list, multi disc sets |
| `edits.go`, `collections.go` | Edits and collections saved in the card's `SWIRL` folder |
| `onlinedb.go`, `shots.go`, `thumbs.go` | Art and info databases, screenshots, cover thumbnails |
| `music.go`, `vmulogo.go`, `vmucap*.go` | Music conversion, VMU logo, VMU screen capture |
| `health.go`, `preview.go`, `cleanup.go`, `gdemuini.go`, `fat32.go` | Health check, Flycast preview, junk files, GDEMU.INI, FAT32 formatting |
| `backup.go` | Card backups to the PC |
| `updates.go` | Checking GitHub for updates and installing them |
| `app*.go` | Install, uninstall, single instance, the app window |
| `assets/` | Embedded files: the menu (`1ST_READ.BIN`), menu disc IP.BIN, icon, default music and logo, titles, the VMU capture tool |
| `tools/gen_titles.py` | Regenerates `assets/titles.tsv.gz` from libretro-database |
| `winres/` | Windows icon, manifest and version info (`make.sh <version>`) |
| `third_party/` | Patched rardecode, go-mp3, and a script to fetch module mirrors |
| `vendor/` | All Go dependencies |

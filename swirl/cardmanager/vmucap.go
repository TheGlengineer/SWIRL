package main

// VMU screen capture: each game is booted for a short time in a hidden, patched copy of the Flycast
// emulator (swirl-vmucap.exe). Every picture the game sends to the VMU screen is logged; SWIRL keeps the
// one shown longest and offers the rest as alternatives.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	capSeconds  = 40.0
	capStartAt  = "12,18,24,30" // presses Start to get past title screens
	sh4Clock    = 200_000_000
	capRecBytes = 8 + 4 + 192
)

type VMUShot struct {
	Bits    []byte  `json:"bits"`    // 192 bytes, SWIRL layout (row major, MSB left, 1 = dark)
	Seconds float64 `json:"seconds"` // how long the game showed it
	First   float64 `json:"first"`   // when it first appeared
}

type VMUCapture struct {
	Folder  string    `json:"folder"`
	Name    string    `json:"name"`
	Product string    `json:"product"`
	Shots   []VMUShot `json:"shots"` // best first
	Error   string    `json:"error,omitempty"`
	Chosen  int       `json:"chosen"` // index into Shots, -1 = keep what the card has
}

// rawToSwirl turns a raw VMU LCD block (as the game sends it) into SWIRL's upright bitmap.
func rawToSwirl(raw []byte) []byte {
	out := make([]byte, vmuBytes)
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			// the game's data is rotated 180 degrees against how the screen is viewed
			b := raw[6*y+5-x/8]
			on := b&(1<<(x%8)) != 0
			if on {
				oy := 31 - y
				out[oy*6+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return out
}

func blankVMU(bits []byte) bool {
	n := 0
	for _, b := range bits {
		for ; b != 0; b &= b - 1 {
			n++
		}
	}
	return n < 12 || n > 48*32-12
}

// parseCapture reads the emulator's log and ranks the distinct pictures from the first VMU (A1).
func parseCapture(b []byte, total float64) []VMUShot {
	type frame struct {
		t    float64
		bits []byte
	}
	var frames []frame
	for i := 0; i+capRecBytes <= len(b); i += capRecBytes {
		port := strings.TrimRight(string(b[i+8:i+12]), "\x00")
		if port != "A1" {
			continue
		}
		t := float64(binary.LittleEndian.Uint64(b[i:])) / sh4Clock
		frames = append(frames, frame{t, rawToSwirl(b[i+12 : i+capRecBytes])})
	}
	type agg struct {
		shot  VMUShot
		order int
	}
	byKey := map[string]*agg{}
	for i, f := range frames {
		end := total
		if i+1 < len(frames) {
			end = frames[i+1].t
		}
		if blankVMU(f.bits) {
			continue
		}
		k := string(f.bits)
		a := byKey[k]
		if a == nil {
			a = &agg{shot: VMUShot{Bits: f.bits, First: f.t}, order: len(byKey)}
			byKey[k] = a
		}
		a.shot.Seconds += end - f.t
	}
	var list []*agg
	for _, a := range byKey {
		list = append(list, a)
	}
	sort.Slice(list, func(i, j int) bool {
		if d := list[i].shot.Seconds - list[j].shot.Seconds; d > 0.5 || d < -0.5 {
			return d > 0
		}
		return list[i].order < list[j].order
	})
	var out []VMUShot
	for i, a := range list {
		if i == 6 {
			break
		}
		out = append(out, a.shot)
	}
	return out
}

func findDisc(dir string) string {
	if g := findGDI(dir); g != "" {
		return g
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".cdi") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// vmucapBaseEnv is this program's environment plus what the emulator needs on this platform.
func vmucapBaseEnv() []string { return append(os.Environ(), vmucapEnv()...) }

// captureCommand is replaced in tests (Linux build of the same patched emulator).
var captureCommand = func(ctx context.Context, disc, logFile string) (*exec.Cmd, error) {
	exe, err := vmucapExe()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, exe, "-config", "config:UseReios=yes", "-config", "audio:backend=null", disc)
	cmd.Dir = filepath.Dir(exe)
	// start every game with an empty VMU, as on a fresh console
	old, _ := filepath.Glob(filepath.Join(vmucapDataDir(exe), "vmu_save_*"))
	for _, f := range old {
		os.Remove(f)
	}
	return cmd, nil
}

// capturePass describes one automatic run: how long, and when to press Start and A.
type capturePass struct {
	secs     float64
	start, a string
}

var capturePasses = []capturePass{
	{capSeconds, capStartAt, ""},
	// second try for games that draw later: longer, and A to get through title menus and save checks
	{90, "10,16,22,28,34,42,50,58,66", "38,62,74"},
}

func captureGame(disc string) ([]VMUShot, error) {
	var lastErr error
	for _, p := range capturePasses {
		shots, err := capturePassRun(disc, p)
		if err == nil && len(shots) > 0 {
			return shots, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func capturePassRun(disc string, p capturePass) ([]VMUShot, error) {
	dir, err := os.MkdirTemp("", "swirl_vmucap_")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	logFile := filepath.Join(dir, "vmu.bin")
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd, err := captureCommand(ctx, disc, logFile)
	if err != nil {
		return nil, err
	}
	cmd.Env = append(vmucapBaseEnv(),
		"FLYCAST_VMUCAP="+logFile,
		"SDL_MAC_BACKGROUND_APP=1", // macOS: do not take the focus from Card Manager
		fmt.Sprintf("FLYCAST_VMUCAP_SECS=%g", p.secs),
		"FLYCAST_VMUCAP_START="+p.start,
		"FLYCAST_VMUCAP_A="+p.a)
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	hideWindow(cmd)
	runErr := cmd.Run()
	b, _ := os.ReadFile(logFile)
	if runErr != nil && len(b) == 0 {
		if ctx.Err() != nil {
			return nil, errors.New("the emulator took too long")
		}
		return nil, fmt.Errorf("the emulator could not run this game (%v)", runErr)
	}
	return parseCapture(b, p.secs), nil
}

// ---------- capture by playing ----------

// manualCommand opens the emulator in a normal window; the person plays until the VMU picture shows.
var manualCommand = func(disc string) (*exec.Cmd, error) {
	exe, err := vmucapExe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "-config", "config:UseReios=yes", "-config", "config:rend.FloatVMUs=yes", disc)
	cmd.Dir = filepath.Dir(exe)
	old, _ := filepath.Glob(filepath.Join(vmucapDataDir(exe), "vmu_save_*"))
	for _, f := range old {
		os.Remove(f)
	}
	return cmd, nil
}

// StartManualCapture boots one game in a visible window. Every VMU picture it draws while the person
// plays is kept; closing the window ends the capture.
func StartManualCapture(root, folder string) error {
	c, err := ScanCard(root)
	if err != nil {
		return err
	}
	g := findGame(c, folder)
	if g == nil {
		return errors.New("that game is no longer on the card")
	}
	disc := findDisc(filepath.Join(root, g.Folder))
	if disc == "" {
		return errors.New("no .gdi or .cdi found for this game")
	}
	game := *g
	return runJob("Capture by playing: "+game.Name, "", func() error {
		dir, err := os.MkdirTemp("", "swirl_vmucap_")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		logFile := filepath.Join(dir, "vmu.bin")
		cmd, err := manualCommand(disc)
		if err != nil {
			return err
		}
		cmd.Env = append(vmucapBaseEnv(), "FLYCAST_VMUCAP="+logFile, "FLYCAST_VMUCAP_MANUAL=1")
		jobLog("Flycast is open. Play until the game's picture shows on the VMU (it floats in the corner of the Flycast window), then close the window.")
		jobLog("Keyboard: arrows move, X is A, C is B, S is X, D is Y, Enter is Start. A controller plugged into the computer works too.")
		jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Waiting for you to close Flycast", 0.5 })
		if err := cmd.Start(); err == nil {
			bringToFront(cmd.Path)
			err = cmd.Wait()
			if err != nil {
				jobLog("Flycast closed (%v)", err)
			}
		} else {
			jobLog("Flycast could not start (%v)", err)
		}
		b, _ := os.ReadFile(logFile)
		total := 0.0
		if len(b) >= capRecBytes {
			total = float64(binary.LittleEndian.Uint64(b[len(b)-capRecBytes:]))/sh4Clock + 5
		}
		shots := parseCapture(b, total)
		if len(shots) == 0 {
			return errors.New("the game did not draw anything on the VMU while it was open")
		}
		list := loadCaptures(root)
		r := VMUCapture{Folder: game.Folder, Name: game.Name, Product: game.Product, Shots: shots, Chosen: 0}
		replaced := false
		for i := range list {
			if list[i].Folder == game.Folder {
				list[i], replaced = r, true
			}
		}
		if !replaced {
			list = append(list, r)
		}
		if err := saveVMUChoice(root, game, shots[0].Bits); err != nil {
			return err
		}
		saveCaptures(root, list)
		jobLog("Captured %d pictures from %s. Pick the one you want in the review window, then click Update SWIRL.", len(shots), game.Name)
		return nil
	})
}

// ---------- storage of results for review ----------

func capturePath(root string) string { return filepath.Join(root, editsDir, "vmucap.json") }

func loadCaptures(root string) []VMUCapture {
	var out []VMUCapture
	if b, err := os.ReadFile(capturePath(root)); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func saveCaptures(root string, list []VMUCapture) error {
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	b, _ := json.Marshal(list)
	return os.WriteFile(capturePath(root), b, 0o644)
}

// StartVMUCapture captures every game (or only those without a VMU screen) and saves the best picture.
func StartVMUCapture(root string, onlyMissing bool) error {
	c, err := ScanCard(root)
	if err != nil {
		return err
	}
	if _, err := vmucapExe(); err != nil {
		return err
	}
	return runJob("Capturing VMU screens", "", func() error {
		var todo []Game
		seen := map[string]bool{}
		for _, g := range c.Games {
			if onlyMissing && g.HasVMU {
				continue
			}
			if g.Product != "" && seen[g.Product] {
				continue // other discs of a multi-disc game share the picture
			}
			seen[g.Product] = true
			todo = append(todo, g)
		}
		if len(todo) == 0 {
			jobLog("Every game already has a VMU screen.")
			return nil
		}
		jobLog("Booting %d games in a hidden emulator for %d seconds of game time each", len(todo), int(capSeconds))
		var results []VMUCapture
		got := 0
		for i, g := range todo {
			jobUpdate(func(j *jobState) {
				j.Stage = fmt.Sprintf("%s (%d of %d)", g.Name, i+1, len(todo))
				j.Pct = 0.95 * float64(i) / float64(len(todo))
			})
			r := VMUCapture{Folder: g.Folder, Name: g.Name, Product: g.Product, Chosen: -1}
			disc := findDisc(filepath.Join(root, g.Folder))
			if disc == "" {
				r.Error = "no .gdi or .cdi found"
			} else if shots, err := captureGame(disc); err != nil {
				r.Error = err.Error()
			} else if len(shots) == 0 {
				r.Error = "the game did not draw on the VMU"
			} else {
				r.Shots, r.Chosen = shots, 0
				if err := saveVMUChoice(root, g, shots[0].Bits); err != nil {
					return err
				}
				got++
			}
			if r.Error != "" {
				jobLog("%s: %s", g.Name, r.Error)
			} else {
				jobLog("%s: captured (%d pictures seen)", g.Name, len(r.Shots))
			}
			results = append(results, r)
			saveCaptures(root, results)
		}
		jobLog("Captured VMU screens for %d of %d games. Review them, then click Update SWIRL.", got, len(todo))
		return nil
	})
}

func saveVMUChoice(root string, g Game, bits []byte) error {
	os.MkdirAll(filepath.Join(root, editsDir, "art"), 0o755)
	if err := os.WriteFile(artPath(root, g.Folder, "vmu"), bits, 0o644); err != nil {
		return err
	}
	edits := loadEdits(root)
	if e := edits.Games[g.Folder]; e == nil || e.Product != g.Product {
		edits.Games[g.Folder] = &GameEdit{Product: g.Product}
		return edits.save()
	}
	return nil
}

// ChooseVMU sets which captured picture a game uses (-1 removes the captured picture).
func ChooseVMU(root, folder string, idx int) error {
	list := loadCaptures(root)
	c, err := ScanCard(root)
	if err != nil {
		return err
	}
	g := findGame(c, folder)
	if g == nil {
		return errors.New("that game is no longer on the card")
	}
	for i := range list {
		if list[i].Folder != folder {
			continue
		}
		if idx < -1 || idx >= len(list[i].Shots) {
			return errors.New("no such picture")
		}
		list[i].Chosen = idx
		if idx == -1 {
			os.Remove(artPath(root, folder, "vmu"))
		} else if err := saveVMUChoice(root, *g, list[i].Shots[idx].Bits); err != nil {
			return err
		}
		return saveCaptures(root, list)
	}
	return errors.New("no capture for that game")
}

// capturesForUI adds PNG previews.
func capturesForUI(root string) []map[string]any {
	var out []map[string]any
	for _, c := range loadCaptures(root) {
		var shots []map[string]any
		for _, s := range c.Shots {
			shots = append(shots, map[string]any{"png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(vmuPNG(s.Bits)), "seconds": s.Seconds})
		}
		out = append(out, map[string]any{"folder": c.Folder, "name": c.Name, "error": c.Error, "chosen": c.Chosen, "shots": shots})
	}
	return out
}

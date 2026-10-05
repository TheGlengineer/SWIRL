// SWIRL Card Manager: installs the SWIRL dashboard into folder 01 of a GDEMU SD card.
// Double-click to open the app in your browser. Command line: -root E:\ -install [-dats folder] | -scan
package main

import (
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

const version = "2.17.0-beta.1"

var (
	mu       sync.Mutex
	lastPing = time.Now()
	token    string
)

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusBadRequest)
	writeJSON(w, map[string]string{"error": err.Error()})
}

func guard(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// images loaded by <img> tags cannot send headers, so they carry the token in the query string
		if r.Header.Get("X-Swirl-Token") != token && r.URL.Query().Get("t") != token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		lastPing = time.Now()
		h(w, r)
	}
}

// lockOrFail takes the card's lock for a request that writes to it. On a busy card it answers the request
// with the reason and returns nil.
func lockOrFail(w http.ResponseWriter, root, op string) func() {
	unlock, err := lockCard(root, op)
	if err != nil {
		fail(w, err)
		return nil
	}
	return unlock
}

type logBuf struct{ lines []string }

func (l *logBuf) log(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func serve() {
	b := make([]byte, 16)
	rand.Read(b)
	token = hex.EncodeToString(b)

	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFS, "web")
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.FileServer(http.FS(sub)).ServeHTTP(w, r)
			return
		}
		lastPing = time.Now()
		page, _ := fs.ReadFile(sub, "index.html")
		html := strings.Replace(string(page), "__TOKEN__", token, 1)
		html = strings.ReplaceAll(html, "__VERSION__", version)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(html))
	})
	mux.HandleFunc("/api/drives", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, listDrives())
	}))
	mux.HandleFunc("/api/scan", guard(func(w http.ResponseWriter, r *http.Request) {
		c, err := ScanCard(r.URL.Query().Get("root"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"card": c, "swirlVersion": swirlHash(swirlBinary)})
	}))
	mux.HandleFunc("/api/install", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root  string            `json:"root"`
			Dats  string            `json:"dats"`
			Names map[string]string `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Update SWIRL")
		if unlock == nil {
			return
		}
		defer unlock()
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{lines: []string{}}
		if len(req.Names) > 0 {
			if err := SaveNames(req.Root, req.Names, l.log); err != nil {
				fail(w, err)
				return
			}
		}
		if err := InstallSwirl(req.Root, req.Dats, l.log); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "log": l.lines})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "log": l.lines})
	}))
	mux.HandleFunc("/api/restore", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root, Backup string
			Force        bool
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Restore")
		if unlock == nil {
			return
		}
		defer unlock()
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{lines: []string{}}
		if err := RestoreBackupForce(req.Root, req.Backup, req.Force, l.log); err != nil {
			var mm *restoreMismatch
			if errors.As(err, &mm) {
				writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "log": l.lines, "mismatches": mm.Lines})
				return
			}
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "log": l.lines})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "log": l.lines})
	}))
	mux.HandleFunc("/api/game", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req SaveGameRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			unlock := lockOrFail(w, req.Root, "Save game")
			if unlock == nil {
				return
			}
			defer unlock()
			mu.Lock()
			err := SaveGame(req)
			mu.Unlock()
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		d, err := GetGameDetail(r.URL.Query().Get("root"), r.URL.Query().Get("folder"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, d)
	}))
	mux.HandleFunc("/api/art", func(w http.ResponseWriter, r *http.Request) {
		// images are loaded by <img> tags, so the token comes in the query string
		q := r.URL.Query()
		if q.Get("t") != token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		b, err := GameArt(q.Get("root"), q.Get("folder"), q.Get("kind"))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	mux.HandleFunc("/api/thumb", guard(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		b, err := GameThumb(q.Get("root"), q.Get("folder"), q.Get("product"), q.Get("size") == "256")
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=86400") // the page adds a version to the address
		w.Write(b)
	}))
	mux.HandleFunc("/api/uiprefs", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var p UIPrefs
			if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
				fail(w, err)
				return
			}
			SaveUIPrefs(p)
		}
		writeJSON(w, LoadUIPrefs())
	}))
	mux.HandleFunc("/api/vga/patch", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root, Folder string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !folderRe.MatchString(req.Folder) {
			fail(w, errors.New("bad request"))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		rep, err := applyCatalogVGAPatch(req.Root, filepath.Join(req.Root, req.Folder), false)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, rep)
	}))
	mux.HandleFunc("/api/vga/unpatch", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root, Folder string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !folderRe.MatchString(req.Folder) {
			fail(w, errors.New("bad request"))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		undo := vgaUndoFile(req.Root, filepath.Join(req.Root, req.Folder))
		if undo == "" {
			fail(w, errors.New("no patch is applied to this game"))
			return
		}
		rep, err := undoDCP(req.Root, filepath.Join(req.Root, req.Folder), undo)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, rep)
	}))
	mux.HandleFunc("/api/vga/fix", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		res, err := vgaFixCard(req.Root)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, res)
	}))
	mux.HandleFunc("/api/fillart", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Fill art")
		if unlock == nil {
			return
		}
		defer unlock()
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{lines: []string{}}
		n, err := FillFromDiscs(req.Root, l.log)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "count": n, "log": l.lines})
	}))
	mux.HandleFunc("/api/dedupe", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Remove duplicates")
		if unlock == nil {
			return
		}
		defer unlock()
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{lines: []string{}}
		if err := RemoveDuplicates(req.Root, l.log); err != nil {
			writeJSON(w, map[string]any{"ok": false, "error": err.Error(), "log": l.lines})
			return
		}
		writeJSON(w, map[string]any{"ok": true, "log": l.lines})
	}))
	mux.HandleFunc("/api/deleteremoved", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root, Name string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Delete removed games")
		if unlock == nil {
			return
		}
		defer unlock()
		mu.Lock()
		defer mu.Unlock()
		if err := DeleteRemoved(req.Root, req.Name); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/diskinfo", guard(func(w http.ResponseWriter, r *http.Request) {
		info, err := diskInfo(r.URL.Query().Get("root"))
		if err != nil {
			fail(w, err)
			return
		}
		diskWarning(info)
		writeJSON(w, info)
	}))
	mux.HandleFunc("/api/checksource", guard(func(w http.ResponseWriter, r *http.Request) {
		p, err := planCopy(strings.TrimSpace(r.URL.Query().Get("src")))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"games": p.Games, "bytes": p.Total, "hasMenu": p.HasMenu, "hasIni": p.HasINI})
	}))
	mux.HandleFunc("/api/newcard", guard(func(w http.ResponseWriter, r *http.Request) {
		var req NewCardRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		if err := startNewCard(req); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/newcard/status", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jobSnapshot())
	}))
	mux.HandleFunc("/api/backup", guard(func(w http.ResponseWriter, r *http.Request) {
		var req BackupRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		if err := StartBackup(req); err != nil {
			fail(w, err)
			return
		}
		p := LoadUIPrefs()
		p.BackupDir, p.BackupOld = req.Dest, req.IncludeOld
		SaveUIPrefs(p)
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/backups/list", guard(func(w http.ResponseWriter, r *http.Request) {
		list := ListBackups(r.URL.Query().Get("dest"))
		if root := r.URL.Query().Get("root"); root != "" {
			if id := existingCardID(root); id != "" {
				for i := range list {
					list[i].ThisCard = list[i].Info.CardID == id
				}
			}
		}
		if list == nil {
			list = []BackupEntry{}
		}
		writeJSON(w, list)
	}))
	mux.HandleFunc("/api/job/cancel", guard(func(w http.ResponseWriter, r *http.Request) {
		jobMu.Lock()
		ok := job.Running && job.Cancellable
		jobMu.Unlock()
		if ok {
			jobCancel.Store(true)
			jobLog("Stopping after the current file...")
		}
		writeJSON(w, map[string]any{"ok": ok})
	}))
	mux.HandleFunc("/api/job", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jobSnapshot())
	}))
	mux.HandleFunc("/api/dbstatus", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, GetDBStatus(r.URL.Query().Get("root")))
	}))
	mux.HandleFunc("/api/dbdownload", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root    string `json:"root"`
			Rebuild bool   `json:"rebuild"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		lockRoot := "" // the download alone does not touch the card
		if req.Rebuild {
			lockRoot = req.Root
		}
		err := runJob(lockRoot, "Downloading box art and info", "", func() error {
			if err := DownloadDB(jobLog, func(f float64) { setPct(0.8 * f) }); err != nil {
				return err
			}
			if req.Rebuild && req.Root != "" {
				jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Adding it to your card", 0.85 })
				if err := installSwirl(req.Root, "", true, jobLog); err != nil {
					return err
				}
				jobLog("Done. Put the card back in your GDEMU.")
			}
			return nil
		})
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/gdemu", guard(func(w http.ResponseWriter, r *http.Request) {
		root := r.URL.Query().Get("root")
		if r.Method == http.MethodPost {
			var req struct {
				Root     string        `json:"root"`
				Settings GDEMUSettings `json:"settings"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			unlock := lockOrFail(w, req.Root, "GDEMU settings")
			if unlock == nil {
				return
			}
			err := SaveGDEMU(req.Root, req.Settings)
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
			root = req.Root
		}
		s, err := ReadGDEMU(root)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, s)
	}))
	mux.HandleFunc("/api/browse", guard(func(w http.ResponseWriter, r *http.Request) {
		res, err := Browse(r.URL.Query().Get("path"), r.URL.Query().Get("kind"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, res)
	}))
	mux.HandleFunc("/api/games/add", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root    string   `json:"root"`
			Sources []string `json:"sources"`
			Dats    string   `json:"dats"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		if err := StartAddGames(req.Root, req.Sources, req.Dats); err != nil {
			fail(w, err)
			return
		}
		rememberFolders("games", req.Sources)
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/names", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root  string            `json:"root"`
			Names map[string]string `json:"names"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		for f, n := range req.Names {
			req.Names[f] = asciiOnly(n)
		}
		unlock := lockOrFail(w, req.Root, "Names")
		if unlock == nil {
			return
		}
		defer unlock()
		l := &logBuf{lines: []string{}}
		if err := SaveNames(req.Root, req.Names, l.log); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "log": l.lines})
	}))
	mux.HandleFunc("/api/peek", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, Peek(r.URL.Query().Get("path"), r.URL.Query().Get("root")))
	}))
	mux.HandleFunc("/api/remember", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Kind  string   `json:"kind"`
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		rememberFolders(req.Kind, req.Paths)
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/games/remove", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root    string   `json:"root"`
			Folders []string `json:"folders"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		if err := StartRemoveGames(req.Root, req.Folders); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/games/reorder", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root  string   `json:"root"`
			Order []string `json:"order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		if err := StartReorder(req.Root, req.Order); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/vmucap", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req struct {
				Root        string `json:"root"`
				OnlyMissing bool   `json:"onlyMissing"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			if err := StartVMUCapture(req.Root, req.OnlyMissing); err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		writeJSON(w, capturesForUI(r.URL.Query().Get("root")))
	}))
	mux.HandleFunc("/api/vmucap/manual", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root, Folder string }
		json.NewDecoder(r.Body).Decode(&req)
		if err := StartManualCapture(req.Root, req.Folder); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	mux.HandleFunc("/api/vmucap/choose", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root   string `json:"root"`
			Folder string `json:"folder"`
			Index  int    `json:"index"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "VMU screen")
		if unlock == nil {
			return
		}
		defer unlock()
		if err := ChooseVMU(req.Root, req.Folder, req.Index); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	}))
	post := func(fn func(map[string]any) (any, error)) http.HandlerFunc {
		return guard(func(w http.ResponseWriter, r *http.Request) {
			req := map[string]any{}
			json.NewDecoder(r.Body).Decode(&req)
			res, err := fn(req)
			if err != nil {
				fail(w, err)
				return
			}
			if res == nil {
				res = map[string]any{"ok": true}
			}
			writeJSON(w, res)
		})
	}
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	mux.HandleFunc("/api/collections", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var req struct {
				Root        string       `json:"root"`
				Collections []Collection `json:"collections"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			unlock := lockOrFail(w, req.Root, "Collections")
			if unlock == nil {
				return
			}
			err := SaveCollections(req.Root, req.Collections)
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
			writeJSON(w, LoadCollections(req.Root))
			return
		}
		cols := LoadCollections(r.URL.Query().Get("root"))
		if cols == nil {
			cols = []Collection{}
		}
		writeJSON(w, cols)
	}))
	mux.HandleFunc("/api/music", guard(func(w http.ResponseWriter, r *http.Request) {
		root := r.URL.Query().Get("root")
		if r.Method == http.MethodPost {
			var req struct {
				Root, File string
				Remove     bool
				Track      int // with Remove: this track only (0 removes them all)
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			root = req.Root
			unlock := lockOrFail(w, root, "Menu music")
			if unlock == nil {
				return
			}
			l := &logBuf{lines: []string{}}
			var err error
			if req.Remove && req.Track > 0 {
				err = RemoveTrack(root, req.Track)
			} else if req.Remove {
				err = RemoveMusic(root)
			} else {
				_, err = SetMusic(root, req.File, l.log)
			}
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
		}
		writeJSON(w, GetMusicInfo(root))
	}))
	mux.HandleFunc("/api/codebreaker", guard(func(w http.ResponseWriter, r *http.Request) {
		root := r.URL.Query().Get("root")
		if r.Method == http.MethodPost {
			var req struct {
				Root, File string
				Remove     bool
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			root = req.Root
			unlock := lockOrFail(w, root, "CodeBreaker")
			if unlock == nil {
				return
			}
			l := &logBuf{lines: []string{}}
			var err error
			if req.Remove {
				err = RemoveCodeBreaker(root)
			} else {
				err = SetCodeBreaker(root, req.File, l.log)
			}
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
		}
		writeJSON(w, GetCodeBreakerInfo(root))
	}))
	mux.HandleFunc("/api/shots/game", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Root, Product string
			Names         []string
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		unlock := lockOrFail(w, req.Root, "Screenshots")
		if unlock == nil {
			return
		}
		defer unlock()
		res, err := FetchGameShots(req.Root, req.Product, req.Names)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, res)
	}))
	mux.HandleFunc("/api/shots/online", post(func(m map[string]any) (any, error) { return nil, StartShotDownload(str(m, "root")) }))
	mux.HandleFunc("/api/shots", guard(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method == http.MethodPost {
			var req struct {
				Root, Product string
				Slot          int
				Image         string // data URL, empty removes
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			if req.Slot < 1 || req.Slot > 2 || req.Product == "" {
				fail(w, errors.New("bad screenshot slot"))
				return
			}
			unlock := lockOrFail(w, req.Root, "Screenshots")
			if unlock == nil {
				return
			}
			defer unlock()

			if req.Image == "" {
				os.Remove(shotPath(req.Root, req.Product, req.Slot))
			} else {
				img, err := decodeDataURL(req.Image)
				if err != nil {
					fail(w, err)
					return
				}
				if err := saveShot(req.Root, req.Product, req.Slot, img); err != nil {
					fail(w, err)
					return
				}
			}
			writeJSON(w, map[string]any{"ok": true})
			return
		}
		// GET: the PNG for one slot (token in the query so an <img> can load it)
		if q.Get("t") != token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		n := 1
		if q.Get("slot") == "2" {
			n = 2
		}
		b, err := os.ReadFile(shotPath(q.Get("root"), q.Get("product"), n))
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(b)
	}))
	mux.HandleFunc("/api/health", guard(func(w http.ResponseWriter, r *http.Request) {
		rep, err := CheckCard(r.URL.Query().Get("root"))
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, rep)
	}))
	mux.HandleFunc("/api/health/junk", post(func(m map[string]any) (any, error) {
		unlock, err := lockCard(str(m, "root"), "Remove junk")
		if err != nil {
			return nil, err
		}
		defer unlock()
		n, err := RemoveJunk(str(m, "root"))
		return map[string]any{"moved": n}, err
	}))
	mux.HandleFunc("/api/health/gaps", post(func(m map[string]any) (any, error) { return nil, StartCloseGaps(str(m, "root")) }))
	mux.HandleFunc("/api/games/arrange", post(func(m map[string]any) (any, error) { return nil, StartArrangeDiscs(str(m, "root")) }))
	mux.HandleFunc("/api/preview", post(func(m map[string]any) (any, error) { return nil, StartPreview(str(m, "root"), str(m, "dats")) }))
	mux.HandleFunc("/api/bg", guard(func(w http.ResponseWriter, r *http.Request) {
		root := r.URL.Query().Get("root")
		if r.Method == http.MethodPost {
			var req struct {
				Root   string `json:"root"`
				Image  string `json:"image"` // a processed 4:3 picture as a data URL (added or replaced)
				Name   string `json:"name"`
				Dim    int    `json:"dim"`
				N      int    `json:"n"`      // with Image: replace this slot (0 adds); with Rename: the slot
				Remove int    `json:"remove"` // drop this slot
				Rename int    `json:"rename"` // change the name and dim of this slot
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			root = req.Root
			unlock := lockOrFail(w, root, "Backdrop pictures")
			if unlock == nil {
				return
			}
			var err error
			switch {
			case req.Remove > 0:
				err = RemoveBg(root, req.Remove)
			case req.Rename > 0:
				err = RenameBg(root, req.Rename, req.Name, req.Dim)
			default:
				var img image.Image
				img, err = decodeDataURL(req.Image)
				if err == nil {
					_, err = AddBg(root, img, req.Name, req.Dim, req.N)
				}
			}
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
		}
		writeJSON(w, map[string]any{"pictures": ListBg(root)})
	}))
	mux.HandleFunc("/api/bg/thumb", func(w http.ResponseWriter, r *http.Request) {
		// loaded by <img> tags, so the token comes in the query string, as for /api/art
		q := r.URL.Query()
		if q.Get("t") != token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		n, _ := strconv.Atoi(q.Get("n"))
		b, err := os.ReadFile(bgPNG(q.Get("root"), n))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	mux.HandleFunc("/api/vmulogo", guard(func(w http.ResponseWriter, r *http.Request) {
		root := r.URL.Query().Get("root")
		if r.Method == http.MethodPost {
			var req struct {
				Root  string `json:"root"`
				Bits  []byte `json:"bits"`
				Reset bool   `json:"reset"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			root = req.Root
			unlock := lockOrFail(w, root, "VMU logo")
			if unlock == nil {
				return
			}
			var bits []byte
			if !req.Reset {
				bits = req.Bits
			}
			err := SaveLogo(root, bits)
			unlock()
			if err != nil {
				fail(w, err)
				return
			}
		}
		bits, custom := GetLogo(root)
		writeJSON(w, map[string]any{"custom": custom, "png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(vmuPNG(bits))})
	}))
	mux.HandleFunc("/api/vmulogo/preview", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Image string      `json:"image"`
			Opts  LogoOptions `json:"opts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		img, err := decodeDataURL(req.Image)
		if err != nil {
			fail(w, err)
			return
		}
		bits := makeLogo(img, req.Opts)
		writeJSON(w, map[string]any{"bits": bits, "png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(vmuPNG(bits))})
	}))
	// the Report page: the scanned QR text in, the decoded report and the GitHub issue text out. Nothing
	// leaves this PC; the user copies the text into an issue.
	mux.HandleFunc("/api/report", post(func(m map[string]any) (any, error) {
		rep, err := ParseReport(str(m, "text"))
		if err != nil {
			return nil, err
		}
		return map[string]any{"report": rep, "issue": IssueBody(rep, version, str(m, "card"))}, nil
	}))
	mux.HandleFunc("/api/report/symbols", guard(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
		if err != nil {
			fail(w, err)
			return
		}
		p, err := saveSymbols(r.URL.Query().Get("name"), data)
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, map[string]any{"path": p})
	}))
	mux.HandleFunc("/api/ping", guard(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, "ok") }))
	mux.HandleFunc("/api/quit", guard(func(w http.ResponseWriter, r *http.Request) {
		if busy() {
			w.WriteHeader(http.StatusConflict)
			writeJSON(w, map[string]any{"error": "still writing to the SD card"})
			return
		}
		writeJSON(w, "bye")
		go func() { time.Sleep(300 * time.Millisecond); quitNow() }()
	}))
	// sent by the page as it closes (navigator.sendBeacon, so the token comes in the query string)
	mux.HandleFunc("/api/bye", guard(func(w http.ResponseWriter, r *http.Request) {
		pageClosed()
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.HandleFunc("/api/app", guard(func(w http.ResponseWriter, r *http.Request) {
		a := getAppInfo()
		a.DataDir = appDataDir()
		writeJSON(w, a)
	}))
	mux.HandleFunc("/api/app/install", post(func(m map[string]any) (any, error) {
		desktop, _ := m["desktop"].(bool)
		if err := InstallApp(desktop); err != nil {
			return nil, err
		}
		return getAppInfo(), nil
	}))
	mux.HandleFunc("/api/app/switch", post(func(m map[string]any) (any, error) { return nil, SwitchToInstalled() }))
	mux.HandleFunc("/api/update/check", guard(func(w http.ResponseWriter, r *http.Request) {
		force := r.URL.Query().Get("force") == "1"
		if !force && !LoadUIPrefs().UpdateAuto() {
			writeJSON(w, &UpdateInfo{Current: version, Repo: updateRepo})
			return
		}
		writeJSON(w, CheckUpdate(force))
	}))
	mux.HandleFunc("/api/update/apply", post(func(m map[string]any) (any, error) {
		kind, _ := m["kind"].(string)
		return nil, StartUpdate(kind)
	}))
	mux.HandleFunc("/api/app/uninstall", post(func(m map[string]any) (any, error) { return nil, startUninstall() }))

	// one copy at a time: a second start of the same program just opens another window onto the first. A
	// newer version, or a different build of the same version (a test build), takes over so the card gets it
	if ri := otherInstance(); ri != nil {
		takeOver := versionNewer(version, ri.Version) || (ri.Version == version && !ri.sameBuild())
		if !takeOver || !askToQuit(ri) {
			openWindow(fmt.Sprintf("http://127.0.0.1:%d/", ri.Port))
			return
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	writeRunning(port)
	addr := fmt.Sprintf("http://127.0.0.1:%d/", port)
	fmt.Println("SWIRL Card Manager running at", addr)
	lastPing = time.Now()
	if os.Getenv("SWIRL_NO_WINDOW") == "" {
		openWindow(addr)
	}
	go idleWatch()
	go cleanUpdates()
	go defaultMusic() // convert the SWIRL theme once, ahead of the first install
	http.Serve(ln, hostCheck(port, mux))
}

// hostCheck answers only requests addressed to this app's own address (CM-20). A web page that resolves
// its own name to 127.0.0.1 (DNS rebinding) reaches the port with that name in the Host header; without
// this check it could read the page, take the token from it and use the whole API. The browser's Origin
// header, when present, must be this address too.
func hostCheck(port int, next http.Handler) http.Handler {
	ok := map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok[strings.ToLower(r.Host)] {
			http.Error(w, "SWIRL Card Manager answers only its own window", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "null" {
			if u, err := url.Parse(o); err != nil || !ok[strings.ToLower(u.Host)] {
				http.Error(w, "SWIRL Card Manager answers only its own window", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	root := flag.String("root", "", "SD card root (for command line use)")
	install := flag.Bool("install", false, "install or update SWIRL in folder 01")
	scan := flag.Bool("scan", false, "print what is on the card")
	dats := flag.String("dats", "", "folder with BOX.DAT / ICON.DAT / META.DAT to import")
	restore := flag.String("restore", "", "backup name to restore into 01")
	buildMenu := flag.String("build-menu", "", "build a menu disc from a folder of menu files (for testing): -build-menu <data dir> -out <dir>")
	outDir := flag.String("out", "", "output folder for -build-menu")
	formatImage := flag.String("format-image", "", "write a fresh FAT32 card layout into an image file (for testing)")
	imageMB := flag.Int64("size-mb", 0, "image size for -format-image")
	formatDisk := flag.String("format-disk", "", "internal: format the card at this drive (runs as administrator)")
	diskNum := flag.Int("disk", -1, "internal: disk number that -format-disk must match")
	statusFile := flag.String("status", "", "internal: progress file for -format-disk")
	formatUID := flag.Int("format-uid", -1, "internal: uid that should own the freshly formatted card")
	formatGID := flag.Int("format-gid", -1, "internal: gid that should own the freshly formatted card")
	formatUser := flag.String("format-user", "", "internal: user to mount the freshly formatted card under")
	captureVMU := flag.String("capture-vmu", "", "boot one disc (.gdi or .cdi) and print the VMU pictures it draws")
	uninstall := flag.Bool("uninstall", false, "remove SWIRL Card Manager from this PC (used by Settings > Apps)")
	quiet := flag.Bool("quiet", false, "with -uninstall: no questions, remove everything")
	waitPID := flag.Int("wait-pid", 0, "internal: wait for this process to exit before starting")
	applyUpd := flag.Bool("apply-update", false, "internal: install this downloaded update and start it")
	patch := flag.String("patch", "", "apply a .dcp patch (VGA patches and the like) in place: -root <card> -folder <NN> -patch <file.dcp> [-dry-run]")
	folder := flag.String("folder", "", "game folder on the card, for -patch and -unpatch")
	unpatch := flag.String("unpatch", "", "take a patch off again from its undo file: -root <card> -folder <NN> -unpatch <file.undo>")
	dryRun := flag.Bool("dry-run", false, "with -patch or -vga-patch: only report what would change")
	vgaPatch := flag.Bool("vga-patch", false, "apply the catalog's VGA patch to a game that has one: -root <card> -folder <NN> -vga-patch [-dry-run]")
	vgaFix := flag.Bool("vga-fix-all", false, "apply the catalog's VGA patch to every game on the card that has one: -root <card> -vga-fix-all [-dry-run]")
	vgaStatus := flag.Bool("vga-status", false, "list every game's VGA support and whether a patch is available or applied: -root <card> -vga-status")
	flag.Parse()
	if *waitPID > 0 {
		waitForPID(*waitPID)
	}
	if *applyUpd {
		os.Exit(applyUpdate())
	}
	if *uninstall {
		os.Exit(Uninstall(*quiet))
	}
	logf := func(format string, args ...any) { fmt.Printf(format+"\n", args...) }
	switch {
	case *captureVMU != "":
		shots, err := captureGame(*captureVMU)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		for i, s := range shots {
			p := fmt.Sprintf("vmu_%d.png", i)
			os.WriteFile(p, vmuPNG(s.Bits), 0o644)
			fmt.Printf("%s: shown %.1fs, first at %.1fs\n", p, s.Seconds, s.First)
		}
	case *formatDisk != "":
		os.Exit(runFormatHelper(*formatDisk, *diskNum, *statusFile, *formatUID, *formatGID, *formatUser))
	case *formatImage != "":
		f, err := os.OpenFile(*formatImage, os.O_RDWR|os.O_CREATE, 0o644)
		if err == nil {
			err = f.Truncate(*imageMB << 20)
		}
		var l *fat32Layout
		if err == nil {
			l, err = planFAT32(uint64(*imageMB<<20)/secSize, "SWIRL")
		}
		if err == nil {
			err = writeFreshCard(f, l, nil)
		}
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("partition at %d, %d sectors, %d KB clusters, %d clusters, reserved %d, FAT %d sectors\n", l.PartStart, l.PartSectors, l.ClusterBytes()/1024, l.Clusters, l.Reserved, l.FATSectors)
	case *buildMenu != "":
		if err := buildTestMenu(*buildMenu, *outDir); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		fmt.Println("Built", *outDir)
	case *root != "" && *scan:
		c, err := ScanCard(*root)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(c, "", "  ")
		fmt.Println(string(out))
	case *root != "" && *install:
		if err := InstallSwirl(*root, *dats, logf); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	case *root != "" && *restore != "":
		if err := RestoreBackup(*root, *restore, logf); err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
	case *root != "" && *folder != "" && *patch != "":
		rep, err := applyDCP(*root, filepath.Join(*root, *folder), *patch, *dryRun)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	case *root != "" && *folder != "" && *vgaPatch:
		rep, err := applyCatalogVGAPatch(*root, filepath.Join(*root, *folder), *dryRun)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	case *root != "" && *vgaFix:
		rows, err := vgaFixAll(*root, *dryRun)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		for _, r := range rows {
			fmt.Println(r)
		}
	case *root != "" && *vgaStatus:
		rows, err := vgaStatusReport(*root)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		for _, r := range rows {
			fmt.Println(r)
		}
	case *root != "" && *folder != "" && *unpatch != "":
		rep, err := undoDCP(*root, filepath.Join(*root, *folder), *unpatch)
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		out, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(out))
	default:
		serve()
	}
}

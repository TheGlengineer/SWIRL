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
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

//go:embed web
var webFS embed.FS

const version = "2.12.1"

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
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{}
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
		var req struct{ Root, Backup string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{}
		if err := RestoreBackup(req.Root, req.Backup, l.log); err != nil {
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
	mux.HandleFunc("/api/fillart", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Root string }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			fail(w, err)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{}
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
		mu.Lock()
		defer mu.Unlock()
		l := &logBuf{}
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
		err := runJob("Downloading box art and info", "", func() error {
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
			if err := SaveGDEMU(req.Root, req.Settings); err != nil {
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
		l := &logBuf{}
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
			if err := SaveCollections(req.Root, req.Collections); err != nil {
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
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				fail(w, err)
				return
			}
			root = req.Root
			l := &logBuf{}
			var err error
			if req.Remove {
				err = RemoveMusic(root)
			} else {
				_, err = SetMusic(root, req.File, l.log)
			}
			if err != nil {
				fail(w, err)
				return
			}
		}
		writeJSON(w, GetMusicInfo(root))
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
		n, err := RemoveJunk(str(m, "root"))
		return map[string]any{"moved": n}, err
	}))
	mux.HandleFunc("/api/games/arrange", post(func(m map[string]any) (any, error) { return nil, StartArrangeDiscs(str(m, "root")) }))
	mux.HandleFunc("/api/preview", post(func(m map[string]any) (any, error) { return nil, StartPreview(str(m, "root"), str(m, "dats")) }))
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
			var bits []byte
			if !req.Reset {
				bits = req.Bits
			}
			if err := SaveLogo(root, bits); err != nil {
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
	mux.HandleFunc("/api/update/apply", post(func(m map[string]any) (any, error) { return nil, StartUpdate() }))
	mux.HandleFunc("/api/app/uninstall", post(func(m map[string]any) (any, error) { return nil, startUninstall() }))

	// one copy at a time: a second start just opens another window onto the first, unless it is a newer
	// version, which takes over (so running a downloaded update replaces the old one)
	if ri := otherInstance(); ri != nil {
		if !versionNewer(version, ri.Version) || !askToQuit(ri) {
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
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	fmt.Println("SWIRL Card Manager running at", url)
	lastPing = time.Now()
	if os.Getenv("SWIRL_NO_WINDOW") == "" {
		openWindow(url)
	}
	go idleWatch()
	go cleanUpdates()
	go defaultMusic() // convert the SWIRL theme once, ahead of the first install
	http.Serve(ln, mux)
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
	captureVMU := flag.String("capture-vmu", "", "boot one disc (.gdi or .cdi) and print the VMU pictures it draws")
	uninstall := flag.Bool("uninstall", false, "remove SWIRL Card Manager from this PC (used by Settings > Apps)")
	quiet := flag.Bool("quiet", false, "with -uninstall: no questions, remove everything")
	waitPID := flag.Int("wait-pid", 0, "internal: wait for this process to exit before starting")
	applyUpd := flag.Bool("apply-update", false, "internal: install this downloaded update and start it")
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
		os.Exit(runFormatHelper(*formatDisk, *diskNum, *statusFile))
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
	default:
		serve()
	}
}

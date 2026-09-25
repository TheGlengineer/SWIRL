package main

import (
	"encoding/json"
	"os"
)

// fmtStatus is how the formatting helper (running with administrator rights) reports progress to the app.
type fmtStatus struct {
	Pct   float64 `json:"pct"`
	Msg   string  `json:"msg"`
	Done  bool    `json:"done"`
	Error string  `json:"error"`
	Root  string  `json:"root"`
}

func writeStatus(path string, s fmtStatus) {
	if path == "" {
		return
	}
	b, _ := json.Marshal(s)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, path)
	}
}

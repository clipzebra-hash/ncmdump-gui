package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Error history is local; never write logs onto the user's source or SMB share.
func (a *App) recordFailure(file string, err error) {
	if a.ctx == nil {
		return
	}
	dir, e := os.UserCacheDir()
	if e != nil {
		return
	}
	dir = filepath.Join(dir, "ncmdump-gui")
	if os.MkdirAll(dir, 0700) != nil {
		return
	}
	f, e := os.OpenFile(filepath.Join(dir, "conversion-errors.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return
	}
	defer f.Close()
	json.NewEncoder(f).Encode(map[string]string{"time": time.Now().Format(time.RFC3339), "file": file, "error": err.Error()})
}

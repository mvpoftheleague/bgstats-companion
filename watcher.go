package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type watcher struct {
	cfg          *Config
	up           *uploader
	setStatus    func(string)
	lastModTimes map[string]int64 // path → last seen mod time
	uploaded     *uploadedStore
}

func newWatcher(cfg *Config, setStatus func(string)) *watcher {
	return &watcher{
		cfg:          cfg,
		up:           newUploader(cfg),
		setStatus:    setStatus,
		lastModTimes: make(map[string]int64),
		uploaded:     loadUploadedStore(),
	}
}

// run starts the polling loop in a background goroutine.
func (w *watcher) run() {
	interval := time.Duration(w.cfg.PollIntervalSecs) * time.Second
	log.Printf("[watcher] starting — poll interval: %ds, WoW dir: %s", w.cfg.PollIntervalSecs, w.cfg.WoWClassicDir)
	ticker := time.NewTicker(interval)
	go func() {
		w.check() // immediate first check
		for range ticker.C {
			w.check()
		}
	}()
}

func (w *watcher) check() {
	paths := findAllSavedVarsPaths(w.cfg.WoWClassicDir)
	if len(paths) == 0 {
		log.Printf("[watcher] no SavedVariables found under %s — waiting for WoW login", w.cfg.WoWClassicDir)
		w.setStatus("Waiting for WoW to create SavedVariables...")
		return
	}

	totalMatches, totalUploaded := 0, 0

	for _, path := range paths {
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			log.Printf("[watcher] stat error %s: %v", path, err)
			continue
		}

		modTime := info.ModTime().UnixMilli()
		if modTime <= w.lastModTimes[path] {
			continue // no change for this account
		}

		account := accountName(path)
		log.Printf("[watcher] change detected for account %q — processing", account)
		w.lastModTimes[path] = modTime

		n, u, err := w.processFile(path)
		if err != nil {
			log.Printf("[watcher] error processing %s: %v", account, err)
		}
		totalMatches += n
		totalUploaded += u
	}

	if totalUploaded > 0 {
		w.setStatus(fmt.Sprintf("Uploaded %d match(es) — %d account(s) monitored", totalUploaded, len(paths)))
	} else {
		w.setStatus(fmt.Sprintf("Watching %d account(s)...", len(paths)))
	}
}

// processFile parses one BgStats.lua, uploads any pending matches, and marks
// them uploaded. Returns (total matches, newly uploaded count, error).
func (w *watcher) processFile(path string) (int, int, error) {
	account := accountName(path)

	content, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	matches, uploaded, err := ParseSavedVariables(string(content))
	if err != nil {
		return 0, 0, fmt.Errorf("parse: %w", err)
	}

	log.Printf("[watcher] %s: %d match(es) total, %d already uploaded", account, len(matches), len(uploaded))

	var pending []Match
	for _, m := range matches {
		if !uploaded[m.UploadKey()] && !w.uploaded.has(m.UploadKey()) {
			pending = append(pending, m)
		}
	}

	if len(pending) == 0 {
		log.Printf("[watcher] %s: nothing to upload", account)
		return len(matches), 0, nil
	}

	log.Printf("[watcher] %s: uploading %d pending match(es)", account, len(pending))

	fileContent := string(content)
	uploadedCount := 0
	for _, m := range pending {
		log.Printf("[watcher] %s: uploading %s (%s, %d players)", account, m.UploadKey(), m.Battleground, len(m.Scores))
		logActivity(fmt.Sprintf("Uploading %s for %s (%d players)...", m.Battleground, account, len(m.Scores)))
		if err := w.up.upload(m); err != nil {
			log.Printf("[watcher] %s: upload FAILED for %s: %v", account, m.UploadKey(), err)
			logActivity(fmt.Sprintf("✗ %s failed: %v", m.Battleground, err))
			continue
		}
		log.Printf("[watcher] %s: upload OK for %s", account, m.UploadKey())
		logActivity(fmt.Sprintf("✓ %s uploaded successfully", m.Battleground))
		w.uploaded.add(m.UploadKey())
		fileContent = MarkUploaded(fileContent, m.UploadKey())
		uploadedCount++
	}

	if uploadedCount > 0 {
		if err := os.WriteFile(path, []byte(fileContent), 0644); err != nil {
			log.Printf("[watcher] %s: write-back failed: %v", account, err)
		} else {
			log.Printf("[watcher] %s: marked %d match(es) as uploaded in SavedVariables", account, uploadedCount)
		}
	}

	return len(matches), uploadedCount, nil
}

// accountName extracts the WoW account name from the SavedVariables path.
// e.g. .../WTF/Account/MYACCOUNT/SavedVariables/BgStats.lua → "MYACCOUNT"
func accountName(path string) string {
	// Walk up: BgStats.lua → SavedVariables → ACCOUNT
	return filepath.Base(filepath.Dir(filepath.Dir(path)))
}

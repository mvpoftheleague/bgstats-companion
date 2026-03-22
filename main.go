package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	// Walk requires the main goroutine to be locked to an OS thread.
	runtime.LockOSThread()

	// If another instance is running, signal it to open its settings window and exit.
	if !acquireSingleInstance() {
		return
	}

	setupLogging()

	cfg, isFirstRun, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	// On first run, copy self to AppData so the autostart entry
	// points to a stable location (not the user's Downloads folder).
	if isFirstRun {
		installedPath, err := installSelf()
		if err != nil {
			log.Printf("self-install failed (continuing in place): %v", err)
			installedPath = currentExePath()
		}
		if !strings.EqualFold(installedPath, currentExePath()) {
			log.Printf("relaunching from %s", installedPath)
			relaunchFrom(installedPath)
			return
		}
		if err := setupAutostart(installedPath); err != nil {
			log.Printf("autostart setup failed: %v", err)
		}
	}

	// runApp creates the tray icon, builds the walk message loop, and blocks.
	if err := runApp(cfg, func(setStatus func(string)) {
		// This runs in a background goroutine after the walk loop starts.

		// Always open settings on startup.
		openSettings(cfg)

		// Auto-register with the backend if we don't have an API key yet.
		// This is transparent — no character info or user action required.
		if cfg.APIKey == "" {
			up := newUploader(cfg)
			if key, err := up.registerCompanion(); err == nil && key != "" {
				cfg.APIKey = key
				cfg.save()
				log.Printf("companion auto-registered, API key obtained")
			} else if err != nil {
				log.Printf("companion auto-register failed: %v", err)
			}
		}

		if cfg.WoWClassicDir != "" {
			// Silently reinstall addon on each launch to keep it up-to-date.
			if err := installAddon(cfg.WoWClassicDir); err != nil {
				log.Printf("addon update failed: %v", err)
			}
		}

		if cfg.isReady() {
			w := newWatcher(cfg, setStatus)
			w.run()
			setStatus("Watching for new matches...")
		} else {
			setStatus("Setup required — click to configure")
		}
	}); err != nil {
		log.Fatalf("app error: %v", err)
	}
}

func setupLogging() {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return
	}
	logPath := filepath.Join(configDir(), "companion.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
}

func currentExePath() string {
	exe, _ := os.Executable()
	abs, _ := filepath.Abs(exe)
	return abs
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

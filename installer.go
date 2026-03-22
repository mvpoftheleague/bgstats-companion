package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	// Registry key to search for WoW install path
	wowRegistryKey  = `SOFTWARE\WOW6432Node\Blizzard Entertainment\World of Warcraft`
	wowRegistryKey2 = `SOFTWARE\Blizzard Entertainment\World of Warcraft`

	autostartRegKey  = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	autostartAppName = "BgStatsCompanion"
)

// detectWoWClassicDir attempts to find the WoW Classic Era directory.
// It checks the Windows registry first, then common install paths.
// Returns the _classic_era_ directory path, or "" if not found.
func detectWoWClassicDir() string {
	// 1. Try Windows registry
	for _, keyPath := range []string{wowRegistryKey, wowRegistryKey2} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, keyPath, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		installPath, _, err := k.GetStringValue("InstallPath")
		k.Close()
		if err != nil {
			continue
		}
		candidate := filepath.Join(installPath, "_classic_era_")
		if dirExists(candidate) {
			return candidate
		}
	}

	// 2. Try common install locations
	candidates := []string{
		`C:\Program Files (x86)\World of Warcraft\_classic_era_`,
		`C:\Program Files\World of Warcraft\_classic_era_`,
		`D:\World of Warcraft\_classic_era_`,
		`D:\Games\World of Warcraft\_classic_era_`,
	}
	for _, c := range candidates {
		if dirExists(c) {
			return c
		}
	}
	return ""
}

// findAllSavedVarsPaths returns every BgStats.lua found across all WoW accounts.
func findAllSavedVarsPaths(wowClassicDir string) []string {
	pattern := filepath.Join(wowClassicDir, "WTF", "Account", "*", "SavedVariables", "BgStats.lua")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}
	return matches
}

// installAddon copies the embedded addon files to the WoW AddOns directory.
func installAddon(wowClassicDir string) error {
	destDir := filepath.Join(wowClassicDir, "Interface", "AddOns", "BgStats")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("create AddOns dir: %w", err)
	}

	// Walk the embedded addon assets and copy each file
	return fs.WalkDir(addonFiles, "assets", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, "assets/")
		dest := filepath.Join(destDir, rel)

		src, err := addonFiles.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()

		dst, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		return err
	})
}

// setupAutostart adds the app to Windows startup via the registry.
func setupAutostart(exePath string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRegKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open autostart registry key: %w", err)
	}
	defer k.Close()
	return k.SetStringValue(autostartAppName, `"`+exePath+`"`)
}

// removeAutostart removes the autostart registry entry.
func removeAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRegKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.DeleteValue(autostartAppName)
}

// installSelf copies this executable to AppData and returns the new path.
// If already running from AppData, returns current path unchanged.
func installSelf() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	exePath, err = filepath.Abs(exePath)
	if err != nil {
		return "", err
	}

	destDir := configDir()
	destPath := filepath.Join(destDir, "BgStatsCompanion.exe")

	// Already in the right place
	if strings.EqualFold(exePath, destPath) {
		return exePath, nil
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create app dir: %w", err)
	}

	src, err := os.Open(exePath)
	if err != nil {
		return "", err
	}
	defer src.Close()

	dst, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("copy exe: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return destPath, nil
}

// relaunchFrom starts the exe at the given path and exits the current process.
func relaunchFrom(exePath string) {
	cmd := exec.Command(exePath)
	cmd.Start()
	os.Exit(0)
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

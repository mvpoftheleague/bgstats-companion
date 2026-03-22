package main

import (
	"log"
	"os"
	"os/exec"

	"github.com/lxn/walk"
)

// runApp creates the hidden main window, tray icon, and menu, then blocks on the walk
// message loop until the user exits. Must be called on the main OS thread.
func runApp(cfg *Config, onReady func(setStatus func(string))) error {
	// Hidden main window — required as parent for NotifyIcon and any dialogs.
	mw, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	mw.SetVisible(false)

	// Load tray icon
	icon, err := loadWalkIcon()
	if err != nil {
		log.Printf("icon load failed: %v", err)
	}

	// Create NotifyIcon (system tray)
	ni, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return err
	}
	defer ni.Dispose()

	if icon != nil {
		ni.SetIcon(icon)
	}
	ni.SetToolTip(appName)
	ni.SetVisible(true)

	// Left-click opens settings
	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			openSettings(cfg)
		}
	})

	// Build context menu using NotifyIcon's built-in menu
	menu := ni.ContextMenu()

	// Status item (disabled, shows current state)
	statusAction := walk.NewAction()
	statusAction.SetText("Starting...")
	statusAction.SetEnabled(false)
	menu.Actions().Add(statusAction)

	menu.Actions().Add(walk.NewSeparatorAction())

	addAction(menu, "Settings", func() { openSettings(cfg) })
	menu.Actions().Add(walk.NewSeparatorAction())
	addAction(menu, "Open Config Folder", func() { openFolder(configDir()) })
	addAction(menu, "Open bgstats.gg", func() { openURL("https://bgstats.gg") })
	menu.Actions().Add(walk.NewSeparatorAction())
	addAction(menu, "Exit", func() {
		ni.SetVisible(false)
		os.Exit(0)
	})

	// When a second instance is launched, open settings on the walk thread.
	startIPCListener(func() {
		mw.Synchronize(func() { openSettings(cfg) })
	})

	// Kick off background work after the message loop starts
	go onReady(func(msg string) {
		// Walk UI updates must happen on the walk thread
		mw.Synchronize(func() {
			statusAction.SetText(msg)
			ni.SetToolTip(appName + ": " + msg)
		})
	})

	// Block until walk exits
	mw.Run()
	return nil
}

func addAction(menu *walk.Menu, text string, fn func()) {
	a := walk.NewAction()
	a.SetText(text)
	a.Triggered().Attach(fn)
	menu.Actions().Add(a)
}

func openFolder(path string) {
	exec.Command("explorer", path).Start()
}

func openURL(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

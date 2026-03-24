package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
)

var settingsIsOpen atomic.Bool

// openSettings opens the settings window non-blocking.
// Only one instance can be open at a time.
func openSettings(cfg *Config) {
	if !settingsIsOpen.CompareAndSwap(false, true) {
		return
	}
	go func() {
		runtime.LockOSThread()
		defer settingsIsOpen.Store(false)
		if err := runSettingsWindow(cfg); err != nil {
			log.Printf("settings window: %v", err)
		}
	}()
}

func runSettingsWindow(cfg *Config) error {
	var mw *walk.Dialog
	var wowDirEdit *walk.LineEdit
	var statusLabel *walk.Label
	var logEdit *walk.TextEdit

	refreshStatus := func() {
		if statusLabel == nil {
			return
		}
		switch {
		case cfg.WoWClassicDir == "":
			statusLabel.SetText("⚠  WoW directory not configured")
		default:
			paths := findAllSavedVarsPaths(cfg.WoWClassicDir)
			if len(paths) == 0 {
				statusLabel.SetText("⚠  No SavedVariables found yet — log into WoW at least once")
			} else {
				statusLabel.SetText(fmt.Sprintf("✓  Active — monitoring %d account(s)", len(paths)))
			}
		}
	}

	err := Dialog{
		AssignTo: &mw,
		Title:    "BgStats Companion — Settings",
		MinSize:  Size{Width: 530, Height: 420},
		Layout:   VBox{Margins: Margins{Top: 16, Left: 20, Right: 20, Bottom: 16}, Spacing: 10},
		Children: []Widget{

			// Status bar
			GroupBox{
				Title:  "Status",
				Layout: VBox{Margins: Margins{Top: 4, Left: 8, Right: 8, Bottom: 8}},
				Children: []Widget{
					Label{
						AssignTo: &statusLabel,
						Text:     "Initializing...",
					},
				},
			},

			// WoW directory
			GroupBox{
				Title:  "World of Warcraft",
				Layout: VBox{Margins: Margins{Top: 4, Left: 8, Right: 8, Bottom: 8}, Spacing: 6},
				Children: []Widget{
					Label{Text: "Classic Era directory (_classic_era_ folder):"},
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 4},
						Children: []Widget{
							LineEdit{
								AssignTo: &wowDirEdit,
								Text:     cfg.WoWClassicDir,
							},
							PushButton{
								Text:    "Browse...",
								MaxSize: Size{Width: 80},
								OnClicked: func() {
									if path, ok := browseFolder("Select World of Warcraft _classic_era_ folder"); ok {
										wowDirEdit.SetText(path)
									}
								},
							},
							PushButton{
								Text:    "Auto-detect",
								MaxSize: Size{Width: 90},
								OnClicked: func() {
									if found := detectWoWClassicDir(); found != "" {
										wowDirEdit.SetText(found)
									} else {
										walk.MsgBox(mw, appName,
											"WoW Classic Era was not found automatically.\nPlease click Browse and select the _classic_era_ folder.",
											walk.MsgBoxIconInformation)
									}
								},
							},
						},
					},
				},
			},

			// Recent activity log
			GroupBox{
				Title:  "Recent Activity",
				Layout: VBox{Margins: Margins{Top: 4, Left: 8, Right: 8, Bottom: 8}},
				Children: []Widget{
					TextEdit{
						AssignTo: &logEdit,
						ReadOnly: true,
						VScroll:  true,
						MinSize:  Size{Height: 120},
					},
				},
			},

			// Action buttons
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 6},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text:    "Save",
						MinSize: Size{Width: 90},
						OnClicked: func() {
							cfg.WoWClassicDir = wowDirEdit.Text()

							// Install / update addon
							if cfg.WoWClassicDir != "" && dirExists(cfg.WoWClassicDir) {
								if err := installAddon(cfg.WoWClassicDir); err != nil {
									walk.MsgBox(mw, appName,
										"Failed to install the BgStats addon:\n\n"+err.Error()+
											"\n\nMake sure WoW is closed and try again.",
										walk.MsgBoxIconWarning)
								} else {
									cfg.AddonInstalled = true
								}
							}

							if err := cfg.save(); err != nil {
								walk.MsgBox(mw, appName,
									"Failed to save settings:\n\n"+err.Error(),
									walk.MsgBoxIconWarning)
								return
							}

							refreshStatus()
							walk.MsgBox(mw, appName, "Settings saved!", walk.MsgBoxIconInformation)
							mw.Close(0)
						},
					},
					PushButton{
						Text:    "Cancel",
						MinSize: Size{Width: 90},
						OnClicked: func() {
							mw.Close(0)
						},
					},
				},
			},
		},
	}.Create(nil)
	if err != nil {
		return err
	}

	// Title-bar / taskbar icon
	if icon, err := loadWalkIcon(); err == nil {
		mw.SetIcon(icon)
	}

	// Populate log with existing entries and subscribe to new ones.
	scrollLogToBottom := func() {
		l := uintptr(len(logEdit.Text()))
		win.SendMessage(logEdit.Handle(), 0x00B1 /*EM_SETSEL*/, l, l)
		win.SendMessage(logEdit.Handle(), 0x00B7 /*EM_SCROLLCARET*/, 0, 0)
	}
	if entries := getActivityLog(); len(entries) > 0 {
		logEdit.SetText(strings.Join(entries, "\r\n"))
		scrollLogToBottom()
	}
	activityMu.Lock()
	onActivityEntry = func(entry string) {
		mw.Synchronize(func() {
			if logEdit == nil {
				return
			}
			cur := logEdit.Text()
			if cur == "" {
				logEdit.SetText(entry)
			} else {
				logEdit.SetText(cur + "\r\n" + entry)
			}
			scrollLogToBottom()
		})
	}
	activityMu.Unlock()
	mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		activityMu.Lock()
		onActivityEntry = nil
		activityMu.Unlock()
	})

	// Minimizing closes the window instead of shrinking to the taskbar.
	mw.SizeChanged().Attach(func() {
		if win.IsIconic(mw.Handle()) {
			mw.Close(0)
		}
	})

	// Fix window to exact client size (530x420 logical pixels) and remove resize handles.
	// SetClientSize is DPI-aware so this works correctly on high-DPI displays.
	const clientW, clientH = 530, 420
	mw.SetClientSize(walk.Size{Width: clientW, Height: clientH})

	hwnd := mw.Handle()
	style := win.GetWindowLong(hwnd, win.GWL_STYLE)
	style &^= win.WS_THICKFRAME | win.WS_MAXIMIZEBOX
	win.SetWindowLong(hwnd, win.GWL_STYLE, style)
	win.SetWindowPos(hwnd, 0, 0, 0, 0, 0,
		win.SWP_NOMOVE|win.SWP_NOSIZE|win.SWP_NOZORDER|win.SWP_FRAMECHANGED)

	// Center on screen.
	var rect win.RECT
	win.GetWindowRect(hwnd, &rect)
	w := rect.Right - rect.Left
	h := rect.Bottom - rect.Top
	screenW := win.GetSystemMetrics(win.SM_CXSCREEN)
	screenH := win.GetSystemMetrics(win.SM_CYSCREEN)
	x := (screenW - w) / 2
	y := (screenH - h) / 2
	win.SetWindowPos(hwnd, 0, x, y, 0, 0, win.SWP_NOSIZE|win.SWP_NOZORDER)

	refreshStatus()
	mw.Run()
	return nil
}

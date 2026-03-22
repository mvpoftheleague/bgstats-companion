package main

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	ole32   = windows.NewLazySystemDLL("ole32.dll")

	procSHBrowseForFolder   = shell32.NewProc("SHBrowseForFolderW")
	procSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
	procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
	procCoInitializeEx      = ole32.NewProc("CoInitializeEx")
)

const (
	bifNewDialogStyle   = 0x0040
	bifReturnOnlyFsDirs = 0x0001
)

type browseInfo struct {
	HwndOwner      windows.HWND
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	LpfnCallback   uintptr
	LParam         uintptr
	IImage         int32
}

// browseFolder shows the Windows "Browse for Folder" dialog.
// Returns the selected path and true, or "", false if cancelled.
func browseFolder(title string) (string, bool) {
	procCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	displayBuf := make([]uint16, windows.MAX_PATH)

	bi := browseInfo{
		PszDisplayName: &displayBuf[0],
		LpszTitle:      titlePtr,
		UlFlags:        bifNewDialogStyle | bifReturnOnlyFsDirs,
	}

	pidl, _, _ := procSHBrowseForFolder.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return "", false
	}
	defer procCoTaskMemFree.Call(pidl)

	pathBuf := make([]uint16, windows.MAX_PATH)
	ret, _, _ := procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&pathBuf[0])))
	if ret == 0 {
		return "", false
	}
	return syscall.UTF16ToString(pathBuf), true
}

package main

// Single-instance enforcement using a named Win32 mutex + named event.
//
//   First instance  → holds the mutex, creates the open-settings event, listens.
//   Second instance → detects the mutex already exists, fires the event, and exits.

import (
	"golang.org/x/sys/windows"
)

const (
	mutexName = "BgStatsCompanion_Mutex"
	eventName = "BgStatsCompanion_OpenSettings"
)

// globalMutex keeps the handle alive for the lifetime of the process.
var globalMutex windows.Handle

// acquireSingleInstance returns true when this is the first running instance.
// If another instance is already running it signals that instance to open its
// settings window and returns false (the caller should then exit immediately).
func acquireSingleInstance() bool {
	namePtr, _ := windows.UTF16PtrFromString(mutexName)
	h, err := windows.CreateMutex(nil, true, namePtr)
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(h)
		signalOpenSettings()
		return false
	}
	globalMutex = h // keep alive
	return true
}

// signalOpenSettings opens the named event created by the first instance and
// sets it, waking the listener goroutine started by startIPCListener.
func signalOpenSettings() {
	evPtr, _ := windows.UTF16PtrFromString(eventName)
	const EVENT_MODIFY_STATE = 0x0002
	h, err := windows.OpenEvent(EVENT_MODIFY_STATE, false, evPtr)
	if err != nil {
		return
	}
	windows.SetEvent(h)
	windows.CloseHandle(h)
}

// startIPCListener creates the named event and waits in a background goroutine.
// onSignal is called each time the event is fired (i.e. every time a second
// instance of the exe is launched).
func startIPCListener(onSignal func()) {
	evPtr, _ := windows.UTF16PtrFromString(eventName)
	h, err := windows.CreateEvent(nil, 0 /*auto-reset*/, 0, evPtr)
	if err != nil {
		return
	}
	go func() {
		for {
			windows.WaitForSingleObject(h, windows.INFINITE)
			onSignal()
		}
	}()
}

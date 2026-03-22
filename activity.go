package main

import (
	"fmt"
	"sync"
	"time"
)

var (
	activityMu      sync.Mutex
	activityEntries []string
	onActivityEntry func(string) // called from background goroutine; must marshal to UI thread
)

const maxActivityEntries = 100

func logActivity(msg string) {
	entry := fmt.Sprintf("[%s] %s", time.Now().Format("15:04:05"), msg)
	activityMu.Lock()
	activityEntries = append(activityEntries, entry)
	if len(activityEntries) > maxActivityEntries {
		activityEntries = activityEntries[len(activityEntries)-maxActivityEntries:]
	}
	cb := onActivityEntry
	activityMu.Unlock()
	if cb != nil {
		cb(entry)
	}
}

func getActivityLog() []string {
	activityMu.Lock()
	defer activityMu.Unlock()
	out := make([]string, len(activityEntries))
	copy(out, activityEntries)
	return out
}

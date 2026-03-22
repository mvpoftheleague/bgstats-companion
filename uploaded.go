package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// uploadedStore is a persistent set of upload keys the companion has
// successfully sent to the backend. It lives in the AppData config directory
// so it survives WoW UI reloads that would otherwise overwrite the markers
// written into SavedVariables.
type uploadedStore struct {
	mu   sync.Mutex
	keys map[string]bool
	path string
}

func loadUploadedStore() *uploadedStore {
	p := filepath.Join(configDir(), "uploaded.json")
	s := &uploadedStore{keys: make(map[string]bool), path: p}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return s
	}
	if err != nil {
		log.Printf("[uploaded] failed to read store: %v", err)
		return s
	}
	var keys []string
	if err := json.Unmarshal(data, &keys); err != nil {
		log.Printf("[uploaded] failed to parse store: %v", err)
		return s
	}
	for _, k := range keys {
		s.keys[k] = true
	}
	log.Printf("[uploaded] loaded %d previously uploaded keys", len(s.keys))
	return s
}

func (s *uploadedStore) has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.keys[key]
}

func (s *uploadedStore) add(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[key] = true
	keys := make([]string, 0, len(s.keys))
	for k := range s.keys {
		keys = append(keys, k)
	}
	data, err := json.Marshal(keys)
	if err != nil {
		log.Printf("[uploaded] failed to marshal store: %v", err)
		return
	}
	if err := os.WriteFile(s.path, data, 0644); err != nil {
		log.Printf("[uploaded] failed to save store: %v", err)
	}
}

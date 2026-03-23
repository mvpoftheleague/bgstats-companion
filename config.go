package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const (
	defaultBackendURL    = "https://bgstats.cc"
	defaultPollInterval  = 30
	appName              = "BgStats Companion"
	configFileName       = "config.json"
	appDataDirName       = "BgStats Companion"
)

// Config holds all persistent settings for the companion app.
type Config struct {
	APIKey           string `json:"apiKey"`
	WoWClassicDir    string `json:"wowClassicDir"` // e.g. C:/.../World of Warcraft/_classic_era_
	BackendURL       string `json:"backendUrl"`
	PollIntervalSecs int    `json:"pollIntervalSeconds"`
	AddonInstalled   bool   `json:"addonInstalled"`
}

func configDir() string {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		home, _ := os.UserHomeDir()
		appData = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appData, appDataDirName)
}

func configPath() string {
	return filepath.Join(configDir(), configFileName)
}

func loadConfig() (*Config, bool, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{
			BackendURL:       defaultBackendURL,
			PollIntervalSecs: defaultPollInterval,
		}
		return cfg, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, false, err
	}
	if cfg.BackendURL == "" {
		cfg.BackendURL = defaultBackendURL
	}
	if cfg.PollIntervalSecs == 0 {
		cfg.PollIntervalSecs = defaultPollInterval
	}
	return &cfg, false, nil
}

func (c *Config) save() error {
	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), data, 0644)
}

func (c *Config) isReady() bool {
	return c.WoWClassicDir != ""
}

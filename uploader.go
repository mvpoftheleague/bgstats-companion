package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type uploader struct {
	cfg    *Config
	client *http.Client
}

func newUploader(cfg *Config) *uploader {
	return &uploader{
		cfg: cfg,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// registerCompanion calls /api/companions/register and returns a fresh API key.
// No character info is required; the companion calls this once on first launch.
func (u *uploader) registerCompanion() (string, error) {
	resp, err := u.client.Post(u.cfg.BackendURL+"/api/companions/register", "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("companion register http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("companion register: server returned %d", resp.StatusCode)
	}
	var result struct {
		ApiKey string `json:"apiKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("companion register decode: %w", err)
	}
	return result.ApiKey, nil
}

func (u *uploader) upload(m Match) error {
	var (
		payload  []byte
		endpoint string
		err      error
	)
	if m.EncodedData != "" {
		payload, err = json.Marshal(map[string]string{"encodedMatch": m.EncodedData})
		endpoint = u.cfg.BackendURL + "/api/matches/encoded"
	} else {
		payload, err = buildPayload(m)
		endpoint = u.cfg.BackendURL + "/api/matches"
	}
	if err != nil {
		return fmt.Errorf("build payload: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", u.cfg.APIKey)

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

func buildPayload(m Match) ([]byte, error) {
	type scorePayload struct {
		CharacterName       string `json:"characterName"`
		Realm               string `json:"realm"`
		Faction             string `json:"faction"`
		CharacterClass      string `json:"characterClass,omitempty"`
		TeamWon             bool   `json:"teamWon"`
		KillingBlows        int    `json:"killingBlows"`
		HonorableKills      int    `json:"honorableKills"`
		Deaths              int    `json:"deaths"`
		BonusHonor          int    `json:"bonusHonor"`
		FlagCaptures        *int   `json:"flagCaptures,omitempty"`
		FlagReturns         *int   `json:"flagReturns,omitempty"`
		BasesAssaulted      *int   `json:"basesAssaulted,omitempty"`
		BasesDefended       *int   `json:"basesDefended,omitempty"`
		GraveyardsAssaulted *int   `json:"graveyardsAssaulted,omitempty"`
		GraveyardsDefended  *int   `json:"graveyardsDefended,omitempty"`
		TowersAssaulted     *int   `json:"towersAssaulted,omitempty"`
		TowersDefended      *int   `json:"towersDefended,omitempty"`
	}

	type matchPayload struct {
		Battleground   string         `json:"battleground"`
		InstanceID     int64          `json:"instanceId"`
		MatchStart     string         `json:"matchStart"`
		MatchEnd       string         `json:"matchEnd"`
		WinningFaction string         `json:"winningFaction"`
		AddonVersion   string         `json:"addonVersion"`
		Scores         []scorePayload `json:"scores"`
	}

	scores := make([]scorePayload, len(m.Scores))
	for i, s := range m.Scores {
		scores[i] = scorePayload{
			CharacterName:       s.CharacterName,
			Realm:               s.Realm,
			Faction:             s.Faction,
			CharacterClass:      s.CharacterClass,
			TeamWon:             s.TeamWon,
			KillingBlows:        s.KillingBlows,
			HonorableKills:      s.HonorableKills,
			Deaths:              s.Deaths,
			BonusHonor:          s.BonusHonor,
			FlagCaptures:        s.FlagCaptures,
			FlagReturns:         s.FlagReturns,
			BasesAssaulted:      s.BasesAssaulted,
			BasesDefended:       s.BasesDefended,
			GraveyardsAssaulted: s.GraveyardsAssaulted,
			GraveyardsDefended:  s.GraveyardsDefended,
			TowersAssaulted:     s.TowersAssaulted,
			TowersDefended:      s.TowersDefended,
		}
	}

	addonVersion := m.AddonVersion
	if addonVersion == "" {
		addonVersion = "unknown"
	}

	p := matchPayload{
		Battleground:   m.Battleground,
		InstanceID:     m.InstanceID,
		MatchStart:     m.MatchStart.UTC().Format(time.RFC3339),
		MatchEnd:       m.MatchEnd.UTC().Format(time.RFC3339),
		WinningFaction: m.WinningFaction,
		AddonVersion:   addonVersion,
		Scores:         scores,
	}
	return json.Marshal(p)
}

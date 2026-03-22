package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Match represents a parsed battleground match from BgStats.lua.
// For addon v1.1+ the match data is opaque (base64-encoded) and decoded by the backend;
// the companion only needs EncodedData and RawKey to forward and track uploads.
type Match struct {
	// Legacy plain-table fields (addon < v1.1)
	Battleground   string
	InstanceID     int64
	MatchStart     time.Time
	MatchEnd       time.Time
	WinningFaction string
	AddonVersion   string
	Scores         []PlayerScore

	// Encoded format (addon v1.1+) — forwarded opaquely to the backend.
	EncodedData string
	RawKey      string // pre-computed key stored alongside the encoded blob
}

// UploadKey returns the unique key used to track uploads (must match the addon's format).
func (m Match) UploadKey() string {
	if m.RawKey != "" {
		return m.RawKey
	}
	return fmt.Sprintf("%s:%d:%d", m.Battleground, m.InstanceID, m.MatchStart.Unix())
}

// PlayerScore is a single player's scoreboard entry.
type PlayerScore struct {
	CharacterName string
	Realm         string
	Faction       string
	CharacterClass string
	TeamWon        bool
	KillingBlows   int
	HonorableKills int
	Deaths         int
	BonusHonor     int

	// WSG
	FlagCaptures *int
	FlagReturns  *int

	// AB
	BasesAssaulted *int
	BasesDefended  *int

	// AV
	GraveyardsAssaulted *int
	GraveyardsDefended  *int
	TowersAssaulted     *int
	TowersDefended      *int
}

// ParseSavedVariables parses BgStats.lua content and returns all matches
// along with the set of already-uploaded match keys.
func ParseSavedVariables(content string) ([]Match, map[string]bool, error) {
	p := &luaParser{src: content}

	dbStart := strings.Index(content, "BgStatsDB = {")
	if dbStart < 0 {
		return nil, nil, nil // file exists but no data yet
	}
	p.pos = dbStart + len("BgStatsDB = ")

	db, err := p.parseTable()
	if err != nil {
		return nil, nil, fmt.Errorf("parse BgStatsDB: %w", err)
	}

	// Parse uploaded map
	uploaded := map[string]bool{}
	if rawUploaded, ok := db["uploaded"]; ok {
		if uploadedMap, ok := rawUploaded.(luaTable); ok {
			for k := range uploadedMap {
				uploaded[k] = true
			}
		}
	}

	// Parse matches — supports both the new encoded format ({ key, data }) and
	// the legacy plain-table format for backwards compatibility.
	var matches []Match
	if rawMatches, ok := db["matches"]; ok {
		if matchesMap, ok := rawMatches.(luaTable); ok {
			for _, v := range matchesMap {
				m, ok := v.(luaTable)
				if !ok {
					continue
				}
				var match Match
				if encodedData, isEncoded := m["data"].(string); isEncoded {
					// Encoded format (addon v1.1+): forward the blob opaquely to the backend.
					rawKey, _ := m["key"].(string)
					if rawKey == "" {
						log.Printf("[lua] skipping encoded match: missing key field")
						continue
					}
					match = Match{EncodedData: encodedData, RawKey: rawKey}
				} else {
					// Legacy plain-table format
					match = Match{
						Battleground:   tableStr(m, "battleground"),
						InstanceID:     tableInt(m, "instanceId"),
						MatchStart:     time.Unix(tableInt(m, "matchStart"), 0),
						MatchEnd:       time.Unix(tableInt(m, "matchEnd"), 0),
						WinningFaction: tableStr(m, "winningFaction"),
						AddonVersion:   tableStr(m, "addonVersion"),
					}
					if rawScores, ok := m["scores"]; ok {
						if scoresMap, ok := rawScores.(luaTable); ok {
							match.Scores = parseScores(scoresMap)
						}
					}
				}
				matches = append(matches, match)
			}
		}
	}
	return matches, uploaded, nil
}

// MarkUploaded appends a new key to the ["uploaded"] table in the Lua file content.
// Returns the updated content.
func MarkUploaded(content string, key string) string {
	marker := fmt.Sprintf(`["%s"] = true,`, key)
	// If already marked, skip
	if strings.Contains(content, marker) {
		return content
	}
	// Append inside the ["uploaded"] = { ... } block
	uploadedMarker := `["uploaded"] = {`
	idx := strings.Index(content, uploadedMarker)
	if idx < 0 {
		return content
	}
	insertAt := idx + len(uploadedMarker)
	return content[:insertAt] + "\n\t\t[\"" + key + `"] = true,` + content[insertAt:]
}

// --- Lua table types ---

type luaTable map[string]any

// --- Recursive descent Lua parser ---

type luaParser struct {
	src string
	pos int
}

func (p *luaParser) parseTable() (luaTable, error) {
	p.skipWS()
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	table := luaTable{}
	arrayIdx := 1

	for p.pos < len(p.src) {
		p.skipWS()
		if p.pos >= len(p.src) || p.src[p.pos] == '}' {
			break
		}

		var key string
		if p.src[p.pos] == '[' {
			p.pos++ // consume '['
			if p.pos < len(p.src) && p.src[p.pos] == '"' {
				var err error
				key, err = p.parseString()
				if err != nil {
					return nil, err
				}
			} else {
				n, err := p.parseNumber()
				if err != nil {
					return nil, err
				}
				key = strconv.FormatInt(n, 10)
			}
			if err := p.expect(']'); err != nil {
				return nil, err
			}
			p.skipWS()
			if err := p.expect('='); err != nil {
				return nil, err
			}
			p.skipWS()
		} else {
			key = strconv.Itoa(arrayIdx)
			arrayIdx++
		}

		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		table[key] = val

		p.skipWS()
		if p.pos < len(p.src) && p.src[p.pos] == ',' {
			p.pos++
		}
	}
	p.skipWS()
	if err := p.expect('}'); err != nil {
		return nil, err
	}
	return table, nil
}

func (p *luaParser) parseValue() (any, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("unexpected EOF")
	}
	c := p.src[p.pos]
	switch {
	case c == '{':
		return p.parseTable()
	case c == '"':
		return p.parseString()
	case c == 't':
		return p.parseBool(true)
	case c == 'f':
		return p.parseBool(false)
	case c == 'n':
		return p.parseNil()
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	default:
		return nil, fmt.Errorf("unexpected char %q at pos %d", c, p.pos)
	}
}

func (p *luaParser) parseString() (string, error) {
	if err := p.expect('"'); err != nil {
		return "", err
	}
	var sb strings.Builder
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		p.pos++
		if c == '"' {
			break
		}
		if c == '\\' && p.pos < len(p.src) {
			sb.WriteByte(p.src[p.pos])
			p.pos++
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String(), nil
}

func (p *luaParser) parseNumber() (int64, error) {
	start := p.pos
	if p.pos < len(p.src) && p.src[p.pos] == '-' {
		p.pos++
	}
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	return strconv.ParseInt(p.src[start:p.pos], 10, 64)
}

func (p *luaParser) parseBool(expected bool) (bool, error) {
	word := "true"
	if !expected {
		word = "false"
	}
	if strings.HasPrefix(p.src[p.pos:], word) {
		p.pos += len(word)
		return expected, nil
	}
	return false, fmt.Errorf("expected %q at pos %d", word, p.pos)
}

func (p *luaParser) parseNil() (any, error) {
	if strings.HasPrefix(p.src[p.pos:], "nil") {
		p.pos += 3
		return nil, nil
	}
	return nil, fmt.Errorf("expected 'nil' at pos %d", p.pos)
}

func (p *luaParser) expect(c byte) error {
	if p.pos >= len(p.src) || p.src[p.pos] != c {
		got := "EOF"
		if p.pos < len(p.src) {
			got = string(p.src[p.pos])
		}
		return fmt.Errorf("expected %q at pos %d, got %q", c, p.pos, got)
	}
	p.pos++
	return nil
}

func (p *luaParser) skipWS() {
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			p.pos++
		} else {
			break
		}
	}
}

// --- Helpers ---

func tableStr(t luaTable, key string) string {
	if v, ok := t[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func tableInt(t luaTable, key string) int64 {
	if v, ok := t[key]; ok {
		if n, ok := v.(int64); ok {
			return n
		}
	}
	return 0
}

func tableBool(t luaTable, key string) bool {
	if v, ok := t[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func optInt(t luaTable, key string) *int {
	if v, ok := t[key]; ok {
		if n, ok := v.(int64); ok {
			i := int(n)
			return &i
		}
	}
	return nil
}

func parseScores(scoresMap luaTable) []PlayerScore {
	var scores []PlayerScore
	for _, v := range scoresMap {
		s, ok := v.(luaTable)
		if !ok {
			continue
		}
		score := PlayerScore{
			CharacterName:  tableStr(s, "characterName"),
			Realm:          tableStr(s, "realm"),
			Faction:        tableStr(s, "faction"),
			CharacterClass: tableStr(s, "characterClass"),
			TeamWon:        tableBool(s, "teamWon"),
			KillingBlows:   int(tableInt(s, "killingBlows")),
			HonorableKills: int(tableInt(s, "honorableKills")),
			Deaths:         int(tableInt(s, "deaths")),
			BonusHonor:     int(tableInt(s, "bonusHonor")),
			FlagCaptures:        optInt(s, "flagCaptures"),
			FlagReturns:         optInt(s, "flagReturns"),
			BasesAssaulted:      optInt(s, "basesAssaulted"),
			BasesDefended:       optInt(s, "basesDefended"),
			GraveyardsAssaulted: optInt(s, "graveyardsAssaulted"),
			GraveyardsDefended:  optInt(s, "graveyardsDefended"),
			TowersAssaulted:     optInt(s, "towersAssaulted"),
			TowersDefended:      optInt(s, "towersDefended"),
		}
		scores = append(scores, score)
	}
	return scores
}

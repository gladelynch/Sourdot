package store

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings is the singleton app config, persisted as human-editable JSON
// (unlike versions/projects, there's only ever one record, so a DB bucket
// buys nothing here).
type Settings struct {
	// Theme is reserved for a future light/dark/auto toggle; v1 ships a
	// single dark theme but the field is stored from day one.
	Theme string `json:"theme"`

	// DefaultVersionID is used to resolve unpinned projects. Empty means
	// "no default set yet".
	DefaultVersionID string `json:"defaultVersionId"`

	// GitHubToken is optional; unauthenticated GitHub API access is capped
	// at 60 req/hr, which aggressive caching should mostly absorb (see
	// plan's open risks) but a PAT can be set to raise the limit.
	GitHubToken string `json:"githubToken,omitempty"`

	WindowWidth  int `json:"windowWidth,omitempty"`
	WindowHeight int `json:"windowHeight,omitempty"`
}

// DefaultSettings returns the settings a fresh install starts with.
func DefaultSettings() Settings {
	return Settings{Theme: "dark"}
}

// LoadSettings reads settings.json from dir, returning DefaultSettings if
// the file doesn't exist yet.
func LoadSettings(dir string) (Settings, error) {
	path := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return DefaultSettings(), nil
	}
	if err != nil {
		return Settings{}, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings writes settings.json to dir, replacing it atomically (write
// to a temp file, then rename) so a crash mid-write can't corrupt it.
func SaveSettings(dir string, s Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := filepath.Join(dir, "settings.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

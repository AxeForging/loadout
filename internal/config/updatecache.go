package config

import (
	"os"
	"path/filepath"
	"time"

	"github.com/goccy/go-yaml"
)

// UpdateCache is the cached result of the last source-update check, written
// by `loadout check` and the opt-in hint pass, and read by the CLI hint and
// the TUI dashboard. Machine-local, best-effort, self-healing: a missing or
// corrupt file just reads as "no cached result".
type UpdateCache struct {
	CheckedAt time.Time `yaml:"checkedAt"`
	Updates   int       `yaml:"updates"`
}

func UpdateCachePath() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "update-cache.yaml"), nil
}

func LoadUpdateCache() (UpdateCache, bool) {
	var c UpdateCache
	path, err := UpdateCachePath()
	if err != nil {
		return c, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, false
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return c, false
	}
	return c, true
}

// SaveUpdateCache is best-effort by design — a hint that can't be cached
// must never break the command that triggered it.
func SaveUpdateCache(updates int) {
	path, err := UpdateCachePath()
	if err != nil {
		return
	}
	raw, err := yaml.Marshal(UpdateCache{CheckedAt: time.Now(), Updates: updates})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644)
}

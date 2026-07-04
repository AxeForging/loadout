// Package config loads loadout's own configuration and resolves platform
// directories. Config lives at os.UserConfigDir()/loadout/config.yaml; a
// missing file yields defaults. Machine-local state (never shared) lives in
// the platform data dir.
package config

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/goccy/go-yaml"
)

// Config is loadout's own configuration. Everything is optional.
type Config struct {
	// StorePath overrides the default canonical store location.
	StorePath string `yaml:"storePath,omitempty"`
	// SkillsStrategy is "per-agent" (default) or "shared-agents-dir".
	SkillsStrategy string `yaml:"skillsStrategy,omitempty"`
	Updates        struct {
		CheckOnRun    bool `yaml:"checkOnRun,omitempty"`
		IntervalHours int  `yaml:"intervalHours,omitempty"`
	} `yaml:"updates,omitempty"`
}

// Dir returns loadout's config directory.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "loadout"), nil
}

// Load reads config.yaml; a missing file returns the zero Config.
func Load() (Config, error) {
	var c Config
	dir, err := Dir()
	if err != nil {
		return c, err
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

// DataDir returns loadout's machine-local data directory (state, caches).
func DataDir() (string, error) {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "loadout"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "loadout"), nil
	case "windows":
		if appData := os.Getenv("AppData"); appData != "" {
			return filepath.Join(appData, "loadout"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", "loadout"), nil
	default:
		return filepath.Join(home, ".local", "share", "loadout"), nil
	}
}

// DefaultStorePath is where the canonical store lives unless overridden.
func DefaultStorePath() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "store"), nil
}

// StatePath is the machine-local install-state file.
func StatePath() (string, error) {
	data, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "state.yaml"), nil
}

// ResolveStore picks the store path: flag > config > default.
func (c Config) ResolveStore(flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}
	if c.StorePath != "" {
		return c.StorePath, nil
	}
	return DefaultStorePath()
}

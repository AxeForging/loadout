package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// profilesDir is the store-relative directory for user-saved wizard
// profiles — answer sets a team can commit and share, distinct from the
// embedded presets (go-cli, web-app) that ship with the binary.
const profilesDir = "profiles"

// SaveProfile writes answers as a named profile in the store, so it's
// versioned and shareable the same way as everything else in the store
// (git, or a bundle).
func SaveProfile(s *store.Store, name string, a Answers) error {
	if err := store.ValidateName(name); err != nil {
		return err
	}
	dir := filepath.Join(s.Root, profilesDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+".yaml"), raw, 0o644)
}

// LoadProfile reads a saved profile's answers from the store.
func LoadProfile(s *store.Store, name string) (Answers, error) {
	if err := store.ValidateName(name); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, profilesDir, name+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			names, _ := ListProfiles(s)
			return nil, fmt.Errorf("unknown profile %q (available: %s)", name, strings.Join(names, ", "))
		}
		return nil, err
	}
	var a Answers
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, fmt.Errorf("parse profile %q: %w", name, err)
	}
	return a.validate()
}

// ListProfiles returns the names of profiles saved in the store, sorted.
func ListProfiles(s *store.Store) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, profilesDir))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	sort.Strings(out)
	return out, nil
}

// RemoveProfile deletes a saved profile from the store.
func RemoveProfile(s *store.Store, name string) error {
	if err := store.ValidateName(name); err != nil {
		return err
	}
	path := filepath.Join(s.Root, profilesDir, name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("unknown profile %q", name)
	}
	return os.Remove(path)
}

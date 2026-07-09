package loadouts

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/config"
)

// activeFile is the machine-local marker recording which loadout was last
// switched to, for display only (loadouts list/show annotate the active
// one). It is NEVER used to compute what a switch removes — that is always
// derived from real state.yaml install records (Engine.InstalledArtifactIDs)
// — so a stale or missing marker can never cause an incorrect diff.
const activeFile = "active-loadout"

// SetActive records name as the last-switched-to loadout.
func SetActive(name string) error {
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, activeFile), []byte(name+"\n"), 0o644)
}

// GetActive returns the last-switched-to loadout name, or "" if never set.
func GetActive() (string, error) {
	dataDir, err := config.DataDir()
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, activeFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

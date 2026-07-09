// Package loadouts manages named, switchable snapshots of the active
// artifact set. A loadout is a saved list of artifact IDs a user wants
// installed; switching to one installs its artifacts and removes whatever
// was previously installed but isn't in it. Distinct from wizard profiles
// (internal/wizard), which save wizard *answers*, not artifact sets.
package loadouts

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// dir is the store-relative directory for saved loadouts.
const dir = "loadouts"

// DefaultName is the reserved, protected loadout auto-created the first time
// this feature is used. It can never be saved over or removed.
const DefaultName = "default"

// ArtifactRef is one artifact a loadout wants active. Targets is reserved
// for a future release; v1 always resolves targets via the store manifest's
// own Manifest.Targeted() and ignores this field.
type ArtifactRef struct {
	ID      string   `yaml:"id"`
	Targets []string `yaml:"targets,omitempty"`
}

// Loadout is a named, saved artifact set.
type Loadout struct {
	Name      string        `yaml:"name"`
	Protected bool          `yaml:"protected,omitempty"`
	CreatedAt time.Time     `yaml:"createdAt"`
	Artifacts []ArtifactRef `yaml:"artifacts"`
}

// Summary renders a loadout for human display (loadouts show).
func (l Loadout) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "name: %s\n", l.Name)
	if l.Protected {
		b.WriteString("protected: true (cannot be saved over or removed)\n")
	}
	fmt.Fprintf(&b, "created: %s\n", l.CreatedAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "artifacts (%d):\n", len(l.Artifacts))
	for _, a := range l.Artifacts {
		fmt.Fprintf(&b, "  - %s\n", a.ID)
	}
	return b.String()
}

// SaveLoadout writes a named loadout to the store, replacing any existing
// loadout of the same name. Rejects the reserved DefaultName.
func SaveLoadout(s *store.Store, name string, artifactIDs []string) (Loadout, error) {
	if name == DefaultName {
		return Loadout{}, fmt.Errorf("%q is managed automatically and can't be saved over — use 'loadout loadouts switch default' to see it", DefaultName)
	}
	if err := store.ValidateName(name); err != nil {
		return Loadout{}, err
	}
	l := Loadout{Name: name, CreatedAt: time.Now().UTC(), Artifacts: refsFor(artifactIDs)}
	if err := writeLoadout(s, l); err != nil {
		return Loadout{}, err
	}
	return l, nil
}

// LoadLoadout reads a saved loadout from the store.
func LoadLoadout(s *store.Store, name string) (Loadout, error) {
	if err := store.ValidateName(name); err != nil {
		return Loadout{}, err
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, dir, name+".yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			names, _ := ListLoadouts(s)
			return Loadout{}, fmt.Errorf("unknown loadout %q (available: %s)", name, strings.Join(names, ", "))
		}
		return Loadout{}, err
	}
	var l Loadout
	if err := yaml.Unmarshal(raw, &l); err != nil {
		return Loadout{}, fmt.Errorf("parse loadout %q: %w", name, err)
	}
	return l, nil
}

// ListLoadouts returns the names of loadouts saved in the store, sorted.
func ListLoadouts(s *store.Store) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, dir))
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

// RemoveLoadout deletes a saved loadout's definition from the store. This
// only removes the definition — it never touches installed artifacts or
// state.yaml; real removal only ever happens through a switch. Rejects the
// reserved DefaultName.
func RemoveLoadout(s *store.Store, name string) error {
	if name == DefaultName {
		return fmt.Errorf("%q is managed automatically and can't be removed", DefaultName)
	}
	if err := store.ValidateName(name); err != nil {
		return err
	}
	path := filepath.Join(s.Root, dir, name+".yaml")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("unknown loadout %q", name)
	}
	return os.Remove(path)
}

// EnsureDefault lazily creates the reserved "default" loadout the first
// time it's called, capturing every artifact ID currently in the store
// manifest as a frozen snapshot — a safety-net rollback point. It is a
// no-op if "default" already exists, even if the manifest has since
// changed: the baseline never retroactively picks up new artifacts.
func EnsureDefault(s *store.Store) error {
	path := filepath.Join(s.Root, dir, DefaultName+".yaml")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	ids := make([]string, 0, len(s.Manifest.Artifacts))
	for _, a := range s.Manifest.Artifacts {
		ids = append(ids, a.ID)
	}
	l := Loadout{Name: DefaultName, Protected: true, CreatedAt: time.Now().UTC(), Artifacts: refsFor(ids)}
	return writeLoadout(s, l)
}

func refsFor(ids []string) []ArtifactRef {
	refs := make([]ArtifactRef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, ArtifactRef{ID: id})
	}
	return refs
}

func writeLoadout(s *store.Store, l Loadout) error {
	d := filepath.Join(s.Root, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(l)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, l.Name+".yaml"), raw, 0o644)
}

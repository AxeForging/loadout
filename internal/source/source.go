// Package source tracks external git repositories that provide artifacts.
// Pins live in the store's sources.lock.yaml (shareable, reproducible);
// clones are cached in the machine-local data dir. All git operations shell
// out to the system git so the user's existing credentials (SSH agent,
// credential helpers) work for private repositories — loadout never stores
// secrets. Updates are never applied without an explicit confirmation.
package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// LockFile is the pin list's file name inside the store root.
const LockFile = "sources.lock.yaml"

type Manager struct {
	Store *store.Store
	// CacheDir holds one cached clone per source (data dir, machine-local).
	CacheDir string
}

type lockFile struct {
	Sources []domain.SourceRef `yaml:"sources"`
}

// Lock reads the pin list; a missing file returns an empty list.
func (m *Manager) Lock() ([]domain.SourceRef, error) {
	raw, err := os.ReadFile(filepath.Join(m.Store.Root, LockFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lf lockFile
	if err := yaml.Unmarshal(raw, &lf); err != nil {
		return nil, fmt.Errorf("parse %s: %w", LockFile, err)
	}
	return lf.Sources, nil
}

func (m *Manager) saveLock(refs []domain.SourceRef) error {
	sort.Slice(refs, func(i, j int) bool { return refs[i].Name < refs[j].Name })
	raw, err := yaml.Marshal(lockFile{Sources: refs})
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(filepath.Join(m.Store.Root, LockFile), raw, 0o644)
}

func (m *Manager) find(name string) (domain.SourceRef, []domain.SourceRef, error) {
	refs, err := m.Lock()
	if err != nil {
		return domain.SourceRef{}, nil, err
	}
	for _, r := range refs {
		if r.Name == name {
			return r, refs, nil
		}
	}
	return domain.SourceRef{}, refs, fmt.Errorf("unknown source %q (see 'loadout source list')", name)
}

// AddResult summarizes a source registration.
type AddResult struct {
	Ref     domain.SourceRef `json:"ref"`
	Added   []string         `json:"added"`
	Skipped []string         `json:"skipped"` // artifact IDs already present in the store
}

// Add clones the repository, discovers skills, imports them into the store
// with origin source:<name>, and pins the commit in the lock file. Aliases
// maps an upstream skill name to the local name it should be imported under
// (spec 008 R15) — the artifact keeps tracking the real upstream skill.
func (m *Manager) Add(ctx context.Context, name, url, ref, subdir string, aliases map[string]string) (AddResult, error) {
	if err := store.ValidateName(name); err != nil {
		return AddResult{}, err
	}
	if refs, err := m.Lock(); err != nil {
		return AddResult{}, err
	} else {
		for _, r := range refs {
			if r.Name == name {
				return AddResult{}, fmt.Errorf("source %q already registered", name)
			}
		}
	}

	dir := filepath.Join(m.CacheDir, name)
	commit, err := cloneFresh(ctx, url, ref, dir)
	if err != nil {
		return AddResult{}, err
	}

	skills, err := DiscoverSkills(dir, subdir)
	if err != nil {
		return AddResult{}, err
	}
	if len(skills) == 0 {
		return AddResult{}, fmt.Errorf("no skills found in %s (expected directories containing SKILL.md)", url)
	}
	known := map[string]bool{}
	for _, sk := range skills {
		known[sk.Name] = true
	}
	for upstream := range aliases {
		if !known[upstream] {
			return AddResult{}, fmt.Errorf("--alias %s=%s: source has no skill %q", upstream, aliases[upstream], upstream)
		}
	}

	result := AddResult{Ref: domain.SourceRef{Name: name, URL: url, Ref: ref, Commit: commit, Subdir: subdir}}
	for _, sk := range skills {
		localName := sk.Name
		if alias, ok := aliases[sk.Name]; ok {
			localName = alias
		}
		if _, exists := m.Store.Artifact(localName); exists {
			result.Skipped = append(result.Skipped, sk.Name)
			continue
		}
		if _, err := m.Store.AdoptAs(domain.KindSkill, localName, sk.Path, domain.OriginSource(name), sk.Name); err != nil {
			return AddResult{}, fmt.Errorf("import skill %s: %w", sk.Name, err)
		}
		result.Added = append(result.Added, localName)
	}

	refs, err := m.Lock()
	if err != nil {
		return AddResult{}, err
	}
	if err := m.saveLock(append(refs, result.Ref)); err != nil {
		return AddResult{}, err
	}
	return result, nil
}

// ImportSkill imports one skill from an already-registered source under a
// local name — the escape hatch for skills skipped at add time because the
// name was taken. The artifact records the real upstream name so updates and
// upstream diffs keep tracking the original skill.
func (m *Manager) ImportSkill(ctx context.Context, sourceName, upstreamName, localName string) (domain.Artifact, error) {
	ref, _, err := m.find(sourceName)
	if err != nil {
		return domain.Artifact{}, err
	}
	if localName == "" {
		localName = upstreamName
	}
	if _, exists := m.Store.Artifact(localName); exists {
		return domain.Artifact{}, fmt.Errorf("artifact %q already exists — pick another local name with --as", localName)
	}
	dir := filepath.Join(m.CacheDir, sourceName)
	if _, err := cloneFull(ctx, ref.URL, ref.Ref, dir); err != nil {
		return domain.Artifact{}, err
	}
	skills, err := DiscoverSkills(dir, ref.Subdir)
	if err != nil {
		return domain.Artifact{}, err
	}
	for _, sk := range skills {
		if sk.Name == upstreamName {
			return m.Store.AdoptAs(domain.KindSkill, localName, sk.Path, domain.OriginSource(sourceName), upstreamName)
		}
	}
	return domain.Artifact{}, fmt.Errorf("source %q has no skill %q (see 'loadout update' output or the repo)", sourceName, upstreamName)
}

// artifactsByUpstream maps each of a source's artifacts by its upstream
// skill name (alias-aware), for matching discovered skills to the store.
func (m *Manager) artifactsByUpstream(origin string) map[string]domain.Artifact {
	out := map[string]domain.Artifact{}
	for _, a := range m.Store.Manifest.Artifacts {
		if a.Origin == origin {
			out[a.UpstreamName()] = a
		}
	}
	return out
}

// Remove drops a source from the lock file. Imported artifacts stay unless
// purge is true, in which case artifacts with this source's origin are
// removed from the store as well.
func (m *Manager) Remove(name string, purge bool) ([]string, error) {
	_, refs, err := m.find(name)
	if err != nil {
		return nil, err
	}
	kept := refs[:0]
	for _, r := range refs {
		if r.Name != name {
			kept = append(kept, r)
		}
	}
	if err := m.saveLock(kept); err != nil {
		return nil, err
	}

	var removed []string
	if purge {
		origin := domain.OriginSource(name)
		var keptArts []domain.Artifact
		for _, a := range m.Store.Manifest.Artifacts {
			if a.Origin != origin {
				keptArts = append(keptArts, a)
				continue
			}
			if err := os.RemoveAll(filepath.Join(m.Store.Root, a.Path)); err != nil {
				return removed, err
			}
			removed = append(removed, a.ID)
		}
		m.Store.Manifest.Artifacts = keptArts
		if err := m.Store.SaveManifest(); err != nil {
			return removed, err
		}
	}
	// Cache cleanup is best-effort; a stale clone is harmless.
	_ = os.RemoveAll(filepath.Join(m.CacheDir, name))
	return removed, nil
}

// SkillState is the offline half of the three-way classification: how a
// source-owned artifact's store content relates to its recorded import
// baseline. Computed without any network access.
type SkillState string

const (
	SkillOK     SkillState = "ok"     // store content matches the import baseline
	SkillEdited SkillState = "edited" // user changed it in the store since import
	SkillLegacy SkillState = "legacy" // no baseline recorded (pre-008 import)
)

// SkillStates maps each artifact owned by the named source to its offline
// state, for `loadout source list`.
func (m *Manager) SkillStates(name string) (map[string]SkillState, error) {
	origin := domain.OriginSource(name)
	out := map[string]SkillState{}
	for _, a := range m.Store.Manifest.Artifacts {
		if a.Origin != origin {
			continue
		}
		if a.SourceHash == "" {
			out[a.ID] = SkillLegacy
			continue
		}
		c, err := m.Store.Content(a)
		if err != nil {
			return nil, err
		}
		if store.Hash(c) == a.SourceHash {
			out[a.ID] = SkillOK
		} else {
			out[a.ID] = SkillEdited
		}
	}
	return out, nil
}

// SkillDir is a discovered skill inside a source clone. Name is the
// upstream name; LocalID is the store artifact it maps to once planning has
// resolved aliases (empty until then, or for skills not yet imported).
type SkillDir struct {
	Name    string
	Path    string
	LocalID string
}

// Label renders a skill for plan output: the upstream name, plus the local
// alias when they differ.
func (sk SkillDir) Label() string {
	if sk.LocalID != "" && sk.LocalID != sk.Name {
		return sk.Name + " (as " + sk.LocalID + ")"
	}
	return sk.Name
}

// DiscoverSkills finds directories containing a SKILL.md beneath root/subdir.
// Nested skills inside a found skill are not descended into.
func DiscoverSkills(root, subdir string) ([]SkillDir, error) {
	base := filepath.Join(root, filepath.FromSlash(subdir))
	var out []SkillDir
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != base && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			out = append(out, SkillDir{Name: d.Name(), Path: path})
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

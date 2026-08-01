package source

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

// CheckResult is the outcome of a non-intrusive remote check for one source.
type CheckResult struct {
	Name      string
	Pinned    string
	Remote    string
	HasUpdate bool
	Err       error
}

// Check resolves each source's remote hash via ls-remote — no clone, no
// writes, failures reported per source.
func (m *Manager) Check(ctx context.Context) ([]CheckResult, error) {
	refs, err := m.Lock()
	if err != nil {
		return nil, err
	}
	out := make([]CheckResult, 0, len(refs))
	for _, r := range refs {
		res := CheckResult{Name: r.Name, Pinned: r.Commit}
		res.Remote, res.Err = lsRemote(ctx, r.URL, r.Ref)
		res.HasUpdate = res.Err == nil && res.Remote != r.Commit
		out = append(out, res)
	}
	return out, nil
}

// UpdatePlan describes what applying a source update would change in the
// STORE (agent homes are only ever touched by 'loadout sync' afterwards).
// Each source-owned skill is classified three-way — baseline (content as
// last imported), store (content now) and upstream — so a user's local edits
// are never mistaken for upstream changes.
type UpdatePlan struct {
	Ref       domain.SourceRef
	NewCommit string
	Log       []string
	Added     []SkillDir
	// Changed skills are safe to write: the user never touched them and
	// upstream moved (or no baseline is known — see LegacyBaseline).
	Changed []SkillDir
	// LocallyEdited skills differ from their import baseline while upstream
	// did not move them — they are reported and never written.
	LocallyEdited []SkillDir
	// Conflicts moved both locally and upstream — written only when the
	// caller explicitly opts into discarding the local edits.
	Conflicts []SkillDir
	// OwnedElsewhere are upstream skill names that exist locally with a
	// different origin (the user's own or another source's) — never touched,
	// listed so the collision is visible.
	OwnedElsewhere []string
	// LegacyBaseline names skills with no recorded import baseline whose
	// pinned content could not be reconstructed (pre-008 store with an
	// unreachable pin): for these, "changed" falls back to the legacy
	// two-way comparison and local-edit detection was impossible.
	LegacyBaseline []string
	// RemovedIDs are store artifacts from this source that vanished
	// upstream; they are kept and relabeled origin "local", never deleted.
	RemovedIDs []string
	// UpToDate means the pin already points at the remote head. Unresolved
	// conflicts and local edits are still classified and reported so a
	// previously skipped discard can be forced without a new upstream commit.
	UpToDate bool

	// rebaseline holds silent manifest-only baseline stamps for skills whose
	// store content already matches upstream but whose recorded baseline is
	// missing or stale (e.g. first update of a pre-008 store).
	rebaseline map[string]string

	cloneDir string
}

// Changes reports how many artifacts applying the plan would write without
// force-discard. Skipped conflicts are intentionally not counted.
func (p *UpdatePlan) Changes() int {
	return len(p.Added) + len(p.Changed) + len(p.RemovedIDs)
}

// PlanUpdate fetches the source and computes the artifact-level three-way
// diff against the store. It performs no store writes. Even when the pin is
// up-to-date the classification runs, so conflicts skipped by an earlier
// update remain visible and forceable.
func (m *Manager) PlanUpdate(ctx context.Context, name string) (*UpdatePlan, error) {
	ref, _, err := m.find(name)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(m.CacheDir, name)
	commit, err := cloneFull(ctx, ref.URL, ref.Ref, dir)
	if err != nil {
		return nil, err
	}
	plan := &UpdatePlan{Ref: ref, NewCommit: commit, cloneDir: dir, rebaseline: map[string]string{}}
	if commit == ref.Commit {
		plan.UpToDate = true
	} else {
		plan.Log = changelog(ctx, dir, ref.Commit, commit)
	}

	skills, err := DiscoverSkills(dir, ref.Subdir)
	if err != nil {
		return nil, err
	}
	origin := domain.OriginSource(name)
	upstream := map[string]SkillDir{}
	for _, sk := range skills {
		upstream[sk.Name] = sk
	}

	for _, sk := range skills {
		art, exists := m.Store.Artifact(sk.Name)
		switch {
		case !exists:
			if !plan.UpToDate {
				plan.Added = append(plan.Added, sk)
			}
		case art.Origin != origin:
			plan.OwnedElsewhere = append(plan.OwnedElsewhere, sk.Name)
		default:
			if err := m.classifySkill(ctx, plan, art, sk); err != nil {
				return nil, err
			}
		}
	}
	if !plan.UpToDate {
		for _, art := range m.Store.Manifest.Artifacts {
			if art.Origin == origin {
				if _, still := upstream[art.ID]; !still {
					plan.RemovedIDs = append(plan.RemovedIDs, art.ID)
				}
			}
		}
	}
	return plan, nil
}

// classifySkill routes one source-owned skill into the plan's buckets using
// the three-way hash comparison, reconstructing a missing baseline from the
// pinned commit when possible.
func (m *Manager) classifySkill(ctx context.Context, plan *UpdatePlan, art domain.Artifact, sk SkillDir) error {
	current, err := m.Store.Content(art)
	if err != nil {
		return err
	}
	storeHash := store.Hash(current)
	upstreamHash, err := hashDir(sk.Path)
	if err != nil {
		return err
	}

	baseline := art.SourceHash
	backfilled := false
	if baseline == "" && storeHash != upstreamHash {
		if h, ok := m.hashAtCommit(ctx, plan.cloneDir, plan.Ref.Commit, sk); ok {
			baseline = h
			backfilled = true
		}
	}

	switch classifyUpdate(baseline, storeHash, upstreamHash) {
	case stateIdentical:
		if art.SourceHash != storeHash {
			plan.rebaseline[art.ID] = storeHash
		}
	case stateUpstreamChanged:
		plan.Changed = append(plan.Changed, sk)
	case stateLocallyEdited:
		plan.LocallyEdited = append(plan.LocallyEdited, sk)
		if backfilled {
			// The reconstructed baseline is worth persisting so the next
			// plan detects this without git archaeology.
			plan.rebaseline[art.ID] = baseline
		}
	case stateConflict:
		plan.Conflicts = append(plan.Conflicts, sk)
	case stateLegacy:
		plan.LegacyBaseline = append(plan.LegacyBaseline, sk.Name)
		plan.Changed = append(plan.Changed, sk)
	}
	return nil
}

// ApplyUpdate writes the planned changes into the store and moves the pin.
// The caller is responsible for having confirmed the plan with the user.
// Conflicts are written only when forceDiscard is true; locally edited
// skills are never written.
func (m *Manager) ApplyUpdate(p *UpdatePlan, forceDiscard bool) error {
	origin := domain.OriginSource(p.Ref.Name)
	dirty := false

	write := append([]SkillDir{}, p.Changed...)
	if forceDiscard {
		write = append(write, p.Conflicts...)
	}
	for _, sk := range write {
		art, ok := m.Store.Artifact(sk.Name)
		if !ok {
			continue
		}
		dst := filepath.Join(m.Store.Root, art.Path)
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := store.CopyTree(sk.Path, dst); err != nil {
			return err
		}
		h, err := hashDir(sk.Path)
		if err != nil {
			return err
		}
		m.setSourceHashInMemory(sk.Name, h)
		dirty = true
	}
	for _, sk := range p.Added {
		if _, err := m.Store.AdoptFrom(domain.KindSkill, sk.Name, sk.Path, origin); err != nil {
			return fmt.Errorf("import skill %s: %w", sk.Name, err)
		}
	}
	for _, id := range p.RemovedIDs {
		for i, art := range m.Store.Manifest.Artifacts {
			if art.ID == id {
				m.Store.Manifest.Artifacts[i].Origin = "local"
				m.Store.Manifest.Artifacts[i].SourceHash = ""
				dirty = true
			}
		}
	}
	for id, h := range p.rebaseline {
		m.setSourceHashInMemory(id, h)
		dirty = true
	}
	if dirty {
		if err := m.Store.SaveManifest(); err != nil {
			return err
		}
	}

	if p.NewCommit == p.Ref.Commit {
		return nil
	}
	refs, err := m.Lock()
	if err != nil {
		return err
	}
	for i := range refs {
		if refs[i].Name == p.Ref.Name {
			refs[i].Commit = p.NewCommit
		}
	}
	return m.saveLock(refs)
}

// UpdateWritePaths returns the store-relative paths ApplyUpdate would
// overwrite with the given force setting — the set the orchestrating layer
// (CLI/web) snapshots before applying, fail closed. Empty when the apply
// would write nothing.
func (m *Manager) UpdateWritePaths(p *UpdatePlan, forceDiscard bool) []string {
	var out []string
	add := func(sks []SkillDir) {
		for _, sk := range sks {
			if art, ok := m.Store.Artifact(sk.Name); ok {
				out = append(out, art.Path)
			}
		}
	}
	add(p.Changed)
	if forceDiscard {
		add(p.Conflicts)
	}
	mutates := len(out) > 0 || len(p.Added) > 0 || len(p.RemovedIDs) > 0 ||
		len(p.rebaseline) > 0 || p.NewCommit != p.Ref.Commit
	if mutates {
		out = append(out, store.ManifestFile, LockFile)
	}
	return out
}

// setSourceHashInMemory mutates the manifest entry without saving — callers
// batch mutations and save once.
func (m *Manager) setSourceHashInMemory(id, hash string) {
	for i := range m.Store.Manifest.Artifacts {
		if m.Store.Manifest.Artifacts[i].ID == id {
			m.Store.Manifest.Artifacts[i].SourceHash = hash
			return
		}
	}
}

// dirContent reads a directory's file set keyed the same way the store keys
// artifact content, so clone-side and store-side hashes are comparable.
func dirContent(dir string) (domain.Content, error) {
	c := domain.Content{Files: map[string][]byte{}}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		c.Files[filepath.ToSlash(rel)] = raw
		return nil
	})
	return c, err
}

// hashDir hashes a directory via dirContent.
func hashDir(dir string) (string, error) {
	c, err := dirContent(dir)
	if err != nil {
		return "", err
	}
	return store.Hash(c), nil
}

// hashAtCommit reconstructs a skill's content hash at a commit from the full
// clone (used to backfill baselines for stores predating sourceHash). A
// missing path or unreachable commit reports ok=false, never an error — the
// caller degrades to legacy behavior.
func (m *Manager) hashAtCommit(ctx context.Context, cloneDir, commit string, sk SkillDir) (string, bool) {
	rel, err := filepath.Rel(cloneDir, sk.Path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	list, err := git(ctx, cloneDir, "ls-tree", "-r", "--name-only", commit, "--", rel)
	if err != nil || list == "" {
		return "", false
	}
	c := domain.Content{Files: map[string][]byte{}}
	for _, p := range strings.Split(list, "\n") {
		raw, err := gitRaw(ctx, cloneDir, "show", commit+":"+p)
		if err != nil {
			return "", false
		}
		c.Files[strings.TrimPrefix(p, rel+"/")] = raw
	}
	return store.Hash(c), true
}

package source

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
type UpdatePlan struct {
	Ref       domain.SourceRef
	NewCommit string
	Log       []string
	Added     []SkillDir
	Changed   []SkillDir
	// RemovedIDs are store artifacts from this source that vanished
	// upstream; they are kept and relabeled origin "local", never deleted.
	RemovedIDs []string
	UpToDate   bool

	cloneDir string
}

// Changes reports whether applying the plan would modify the store.
func (p *UpdatePlan) Changes() int {
	return len(p.Added) + len(p.Changed) + len(p.RemovedIDs)
}

// PlanUpdate fetches the source and computes the artifact-level diff against
// the store. It performs no store writes.
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
	plan := &UpdatePlan{Ref: ref, NewCommit: commit, cloneDir: dir}
	if commit == ref.Commit {
		plan.UpToDate = true
		return plan, nil
	}
	plan.Log = changelog(ctx, dir, ref.Commit, commit)

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
			plan.Added = append(plan.Added, sk)
		case art.Origin != origin:
			// Owned by the user or another source — never overwrite.
		default:
			same, err := m.sameContent(art, sk.Path)
			if err != nil {
				return nil, err
			}
			if !same {
				plan.Changed = append(plan.Changed, sk)
			}
		}
	}
	for _, art := range m.Store.Manifest.Artifacts {
		if art.Origin == origin {
			if _, still := upstream[art.ID]; !still {
				plan.RemovedIDs = append(plan.RemovedIDs, art.ID)
			}
		}
	}
	return plan, nil
}

// ApplyUpdate writes the planned changes into the store and moves the pin.
// The caller is responsible for having confirmed the plan with the user.
func (m *Manager) ApplyUpdate(p *UpdatePlan) error {
	if p.UpToDate {
		return nil
	}
	origin := domain.OriginSource(p.Ref.Name)
	for _, sk := range p.Changed {
		dst := ""
		if art, ok := m.Store.Artifact(sk.Name); ok {
			dst = filepath.Join(m.Store.Root, art.Path)
		}
		if dst == "" {
			continue
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := store.CopyTree(sk.Path, dst); err != nil {
			return err
		}
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
			}
		}
	}
	if len(p.RemovedIDs) > 0 {
		if err := m.Store.SaveManifest(); err != nil {
			return err
		}
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

// sameContent compares a store artifact's content hash with a directory in
// the source clone.
func (m *Manager) sameContent(art domain.Artifact, dir string) (bool, error) {
	current, err := m.Store.Content(art)
	if err != nil {
		return false, err
	}
	incoming := domain.Content{Files: map[string][]byte{}}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
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
		incoming.Files[filepath.ToSlash(rel)] = raw
		return nil
	})
	if err != nil {
		return false, err
	}
	return store.Hash(current) == store.Hash(incoming), nil
}

// Package bundle exports the canonical store as one shareable artifact ("a
// loadout") and imports bundles from teammates. A bundle is the store tree
// plus a bundle.yaml metadata file — either a plain directory (git-friendly)
// or a single .loadout.tar.gz archive. Importing never merges silently: the
// caller shows the plan and confirms before Apply.
package bundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// APIVersion is the bundle schema this build reads and writes.
const APIVersion = "loadout-bundle/v1"

// MetaFile is the metadata file name inside a bundle.
const MetaFile = "bundle.yaml"

// Meta describes a bundle.
type Meta struct {
	APIVersion string    `yaml:"apiVersion"`
	Name       string    `yaml:"name"`
	CreatedAt  time.Time `yaml:"createdAt"`
	CreatedBy  string    `yaml:"createdBy"`
	Artifacts  int       `yaml:"artifacts"`
}

// Export writes the store as a bundle directory at dest (which must not
// exist yet). The store's .git directory is not included.
func Export(s *store.Store, dest, createdBy string) (Meta, error) {
	if _, err := os.Stat(dest); err == nil {
		return Meta{}, fmt.Errorf("destination %s already exists", dest)
	}
	if err := copyStoreTree(s.Root, dest); err != nil {
		return Meta{}, err
	}
	meta := Meta{
		APIVersion: APIVersion,
		Name:       s.Manifest.Name,
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  createdBy,
		Artifacts:  len(s.Manifest.Artifacts),
	}
	raw, err := yaml.Marshal(meta)
	if err != nil {
		return Meta{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, MetaFile), raw, 0o644); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// ExportArchive writes the store as a single .loadout.tar.gz file.
func ExportArchive(s *store.Store, destFile, createdBy string) (Meta, error) {
	staging, err := os.MkdirTemp("", "loadout-bundle-*")
	if err != nil {
		return Meta{}, err
	}
	defer os.RemoveAll(staging)

	dir := filepath.Join(staging, "bundle")
	meta, err := Export(s, dir, createdBy)
	if err != nil {
		return Meta{}, err
	}
	if err := tarGz(dir, destFile); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// copyStoreTree copies the store excluding its .git directory.
func copyStoreTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, raw, info.Mode().Perm())
	})
}

// ImportPlan is the computed merge of a bundle into the local store.
type ImportPlan struct {
	Meta      Meta
	New       []domain.Artifact
	Conflicts []domain.Artifact // same ID exists locally with different content
	Identical []string
	Sources   []domain.SourceRef // bundle sources not registered locally

	bundleRoot string
	cleanup    func()
}

// Close releases any temp extraction directory behind the plan.
func (p *ImportPlan) Close() {
	if p.cleanup != nil {
		p.cleanup()
	}
}

// Changes reports how many artifacts Apply would write with the given
// overwrite policy.
func (p *ImportPlan) Changes(overwrite bool) int {
	n := len(p.New) + len(p.Sources)
	if overwrite {
		n += len(p.Conflicts)
	}
	return n
}

// PlanImport validates a bundle (directory or .tar.gz) and diffs it against
// the local store. It performs no writes to the store.
func PlanImport(s *store.Store, src string) (*ImportPlan, error) {
	root, cleanup, err := openBundle(src)
	if err != nil {
		return nil, err
	}
	plan := &ImportPlan{bundleRoot: root, cleanup: cleanup}

	rawMeta, err := os.ReadFile(filepath.Join(root, MetaFile))
	if err != nil {
		plan.Close()
		return nil, fmt.Errorf("not a loadout bundle (missing %s): %w", MetaFile, err)
	}
	if err := yaml.Unmarshal(rawMeta, &plan.Meta); err != nil {
		plan.Close()
		return nil, err
	}
	if plan.Meta.APIVersion != APIVersion {
		plan.Close()
		return nil, fmt.Errorf("unsupported bundle apiVersion %q (want %s)", plan.Meta.APIVersion, APIVersion)
	}

	bundleStore, err := store.Open(root)
	if err != nil {
		plan.Close()
		return nil, err
	}

	for _, art := range bundleStore.Manifest.Artifacts {
		local, exists := s.Artifact(art.ID)
		if !exists {
			plan.New = append(plan.New, art)
			continue
		}
		incoming, err := bundleStore.Content(art)
		if err != nil {
			plan.Close()
			return nil, err
		}
		current, err := s.Content(local)
		if err != nil {
			plan.Close()
			return nil, err
		}
		if store.Hash(incoming) == store.Hash(current) {
			plan.Identical = append(plan.Identical, art.ID)
		} else {
			plan.Conflicts = append(plan.Conflicts, art)
		}
	}

	bundleSources, err := (&sourceLockReader{root: root}).read()
	if err != nil {
		plan.Close()
		return nil, err
	}
	localSources, err := (&sourceLockReader{root: s.Root}).read()
	if err != nil {
		plan.Close()
		return nil, err
	}
	known := map[string]bool{}
	for _, r := range localSources {
		known[r.Name] = true
	}
	for _, r := range bundleSources {
		if !known[r.Name] {
			plan.Sources = append(plan.Sources, r)
		}
	}

	sort.Slice(plan.New, func(i, j int) bool { return plan.New[i].ID < plan.New[j].ID })
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].ID < plan.Conflicts[j].ID })
	return plan, nil
}

// Apply merges the planned bundle into the store. Conflicting artifacts are
// skipped unless overwrite is true. The caller must have confirmed the plan.
func Apply(s *store.Store, p *ImportPlan, overwrite bool) ([]string, error) {
	var applied []string
	copyArtifact := func(art domain.Artifact) error {
		src := filepath.Join(p.bundleRoot, filepath.FromSlash(art.Path))
		dst := filepath.Join(s.Root, filepath.FromSlash(art.Path))
		info, err := os.Stat(src)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if info.IsDir() {
			return store.CopyTree(src, dst)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		raw, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, raw, info.Mode().Perm())
	}

	for _, art := range p.New {
		if err := copyArtifact(art); err != nil {
			return applied, err
		}
		if err := s.AddArtifact(art); err != nil {
			return applied, err
		}
		applied = append(applied, art.ID)
	}
	if overwrite {
		for _, art := range p.Conflicts {
			if err := copyArtifact(art); err != nil {
				return applied, err
			}
			for i, local := range s.Manifest.Artifacts {
				if local.ID == art.ID {
					s.Manifest.Artifacts[i] = art
				}
			}
			applied = append(applied, art.ID)
		}
		if len(p.Conflicts) > 0 {
			if err := s.SaveManifest(); err != nil {
				return applied, err
			}
		}
	}

	if len(p.Sources) > 0 {
		localSources, err := (&sourceLockReader{root: s.Root}).read()
		if err != nil {
			return applied, err
		}
		merged := append(localSources, p.Sources...)
		sort.Slice(merged, func(i, j int) bool { return merged[i].Name < merged[j].Name })
		raw, err := yaml.Marshal(map[string][]domain.SourceRef{"sources": merged})
		if err != nil {
			return applied, err
		}
		if err := os.WriteFile(filepath.Join(s.Root, "sources.lock.yaml"), raw, 0o644); err != nil {
			return applied, err
		}
	}
	return applied, nil
}

// sourceLockReader reads sources.lock.yaml without importing the source
// package (bundle and source stay siblings).
type sourceLockReader struct{ root string }

func (r *sourceLockReader) read() ([]domain.SourceRef, error) {
	raw, err := os.ReadFile(filepath.Join(r.root, "sources.lock.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lf struct {
		Sources []domain.SourceRef `yaml:"sources"`
	}
	if err := yaml.Unmarshal(raw, &lf); err != nil {
		return nil, err
	}
	return lf.Sources, nil
}

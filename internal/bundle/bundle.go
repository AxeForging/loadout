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
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/goccy/go-yaml"
)

// APIVersion is the bundle schema this build reads and writes.
const APIVersion = "loadout-bundle/v1"

// MetaFile is the metadata file name inside a bundle.
const MetaFile = "bundle.yaml"

// Meta describes a bundle.
type Meta struct {
	APIVersion string    `yaml:"apiVersion" json:"apiVersion"`
	Name       string    `yaml:"name" json:"name"`
	CreatedAt  time.Time `yaml:"createdAt" json:"createdAt"`
	CreatedBy  string    `yaml:"createdBy" json:"createdBy"`
	Artifacts  int       `yaml:"artifacts" json:"artifacts"`
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

// TempArchive is an archive exported to a temp location (e.g. for an HTTP
// download); call Cleanup when done.
type TempArchive struct {
	Path string
	dir  string
}

func (t TempArchive) Cleanup() { _ = os.RemoveAll(t.dir) }

// ExportArchiveTemp exports the store archive into a fresh temp dir.
func ExportArchiveTemp(s *store.Store, createdBy string) (TempArchive, error) {
	dir, err := os.MkdirTemp("", "loadout-export-*")
	if err != nil {
		return TempArchive{}, err
	}
	path := filepath.Join(dir, "bundle.loadout.tar.gz")
	if _, err := ExportArchive(s, path, createdBy); err != nil {
		_ = os.RemoveAll(dir)
		return TempArchive{}, err
	}
	return TempArchive{Path: path, dir: dir}, nil
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

	// NewProfiles and ConflictProfiles are saved wizard profiles
	// (profiles/<name>.yaml) found in the bundle — diffed by filename and
	// raw content hash, the same way artifacts are diffed by ID and hash.
	NewProfiles      []string
	ConflictProfiles []string // same name exists locally with different content
	// NewLibraryEntries are custom-instruction library entries the bundle
	// has that the local store doesn't. Library entries are
	// content-addressed (see wizard.AddToLibrary), so merging them is
	// always safe — there is no conflict concept for the library.
	NewLibraryEntries []wizard.LibraryEntry
	// NewGroups and ConflictGroups are library groups (library/groups.yaml)
	// diffed by name — same shape as profiles: a name the local store
	// doesn't have is New, the same name with different member entries is
	// a Conflict.
	NewGroups      []string
	ConflictGroups []string

	// Warnings flag incoming artifacts whose name would shadow an agent
	// built-in slash command (e.g. a bundle's "plan" command). Advisory only —
	// import is never blocked by them.
	Warnings []string

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
	n := len(p.New) + len(p.Sources) + len(p.NewProfiles) + len(p.NewLibraryEntries) + len(p.NewGroups)
	if overwrite {
		n += len(p.Conflicts) + len(p.ConflictProfiles) + len(p.ConflictGroups)
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

	bundleProfileNames, err := wizard.ListProfiles(bundleStore)
	if err != nil {
		plan.Close()
		return nil, err
	}
	for _, name := range bundleProfileNames {
		bundleRaw, err := os.ReadFile(filepath.Join(root, "profiles", name+".yaml"))
		if err != nil {
			plan.Close()
			return nil, err
		}
		localRaw, err := os.ReadFile(filepath.Join(s.Root, "profiles", name+".yaml"))
		switch {
		case os.IsNotExist(err):
			plan.NewProfiles = append(plan.NewProfiles, name)
		case err != nil:
			plan.Close()
			return nil, err
		case store.HashBytes(localRaw) != store.HashBytes(bundleRaw):
			plan.ConflictProfiles = append(plan.ConflictProfiles, name)
		}
	}

	localLibrary, err := wizard.ListLibrary(s)
	if err != nil {
		plan.Close()
		return nil, err
	}
	bundleLibrary, err := wizard.ListLibrary(bundleStore)
	if err != nil {
		plan.Close()
		return nil, err
	}
	haveEntry := map[string]bool{}
	for _, e := range localLibrary {
		haveEntry[e.ID] = true
	}
	for _, e := range bundleLibrary {
		if !haveEntry[e.ID] {
			plan.NewLibraryEntries = append(plan.NewLibraryEntries, e)
		}
	}

	localGroups, err := wizard.ListGroups(s)
	if err != nil {
		plan.Close()
		return nil, err
	}
	bundleGroups, err := wizard.ListGroups(bundleStore)
	if err != nil {
		plan.Close()
		return nil, err
	}
	localGroupByName := make(map[string]wizard.LibraryGroup, len(localGroups))
	for _, g := range localGroups {
		localGroupByName[g.Name] = g
	}
	for _, g := range bundleGroups {
		local, exists := localGroupByName[g.Name]
		switch {
		case !exists:
			plan.NewGroups = append(plan.NewGroups, g.Name)
		case !slices.Equal(local.EntryIDs, g.EntryIDs):
			plan.ConflictGroups = append(plan.ConflictGroups, g.Name)
		}
	}

	sort.Slice(plan.New, func(i, j int) bool { return plan.New[i].ID < plan.New[j].ID })
	sort.Slice(plan.Conflicts, func(i, j int) bool { return plan.Conflicts[i].ID < plan.Conflicts[j].ID })
	sort.Strings(plan.NewProfiles)
	sort.Strings(plan.ConflictProfiles)

	// Flag incoming artifacts that would shadow an agent built-in — advisory
	// only, computed over everything the import could write.
	for _, art := range append(append([]domain.Artifact{}, plan.New...), plan.Conflicts...) {
		if w := domain.ReservedNameWarning(art.Kind, art.ID); w != "" {
			plan.Warnings = append(plan.Warnings, w)
		}
	}
	return plan, nil
}

// copyArtifactFiles copies one artifact's file(s) (single file or whole
// directory) from srcRoot to destRoot, replacing whatever's already at the
// destination. Shared by Apply (import) and ExportSelected (export).
func copyArtifactFiles(srcRoot, destRoot string, art domain.Artifact) error {
	src := filepath.Join(srcRoot, filepath.FromSlash(art.Path))
	dst := filepath.Join(destRoot, filepath.FromSlash(art.Path))
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

// Apply merges the planned bundle into the store. Conflicting artifacts are
// skipped unless overwrite is true. The caller must have confirmed the plan.
func Apply(s *store.Store, p *ImportPlan, overwrite bool) ([]string, error) {
	var applied []string
	copyArtifact := func(art domain.Artifact) error {
		return copyArtifactFiles(p.bundleRoot, s.Root, art)
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

	if len(p.NewProfiles) > 0 || (overwrite && len(p.ConflictProfiles) > 0) {
		localDir := filepath.Join(s.Root, "profiles")
		if err := os.MkdirAll(localDir, 0o755); err != nil {
			return applied, err
		}
		copyProfile := func(name string) error {
			raw, err := os.ReadFile(filepath.Join(p.bundleRoot, "profiles", name+".yaml"))
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(localDir, name+".yaml"), raw, 0o644)
		}
		for _, name := range p.NewProfiles {
			if err := copyProfile(name); err != nil {
				return applied, err
			}
		}
		if overwrite {
			for _, name := range p.ConflictProfiles {
				if err := copyProfile(name); err != nil {
					return applied, err
				}
			}
		}
	}

	for _, entry := range p.NewLibraryEntries {
		if _, err := wizard.AddToLibrary(s, entry.Text); err != nil {
			return applied, err
		}
	}

	if len(p.NewGroups) > 0 || (overwrite && len(p.ConflictGroups) > 0) {
		bundleStore, err := store.Open(p.bundleRoot)
		if err != nil {
			return applied, err
		}
		bundleGroups, err := wizard.ListGroups(bundleStore)
		if err != nil {
			return applied, err
		}
		byName := make(map[string]wizard.LibraryGroup, len(bundleGroups))
		for _, g := range bundleGroups {
			byName[g.Name] = g
		}
		applyGroup := func(name string) error {
			g, ok := byName[name]
			if !ok {
				return fmt.Errorf("group %q not found in bundle", name)
			}
			_, err := wizard.SaveGroup(s, g.Name, g.EntryIDs)
			return err
		}
		for _, name := range p.NewGroups {
			if err := applyGroup(name); err != nil {
				return applied, err
			}
		}
		if overwrite {
			for _, name := range p.ConflictGroups {
				if err := applyGroup(name); err != nil {
					return applied, err
				}
			}
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

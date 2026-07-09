package bundle

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/goccy/go-yaml"
)

// Selection is an explicit include-list per category for a filtered
// export. Callers (CLI/web) are responsible for resolving "everything"
// into concrete IDs before calling — ExportSelected never guesses, so
// there's no nil-vs-empty ambiguity: an empty slice always means "include
// none of this category," not "include all." Export/ExportArchive (the
// unfiltered functions) are unchanged and remain the way to export
// everything.
type Selection struct {
	ArtifactIDs     []string
	ProfileNames    []string
	LibraryEntryIDs []string
	Sources         bool
}

// ExportSelected writes a filtered bundle directory at dest (which must
// not exist yet) containing only what sel lists.
func ExportSelected(s *store.Store, dest, createdBy string, sel Selection) (Meta, error) {
	if _, err := os.Stat(dest); err == nil {
		return Meta{}, fmt.Errorf("destination %s already exists", dest)
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return Meta{}, err
	}

	artifacts := make([]domain.Artifact, 0, len(sel.ArtifactIDs))
	for _, id := range sel.ArtifactIDs {
		art, ok := s.Artifact(id)
		if !ok {
			return Meta{}, fmt.Errorf("unknown artifact %q", id)
		}
		artifacts = append(artifacts, art)
		if err := copyArtifactFiles(s.Root, dest, art); err != nil {
			return Meta{}, err
		}
	}
	manifest := domain.Manifest{
		APIVersion:  s.Manifest.APIVersion,
		Name:        s.Manifest.Name,
		Description: s.Manifest.Description,
		Defaults:    s.Manifest.Defaults,
		Artifacts:   artifacts,
	}
	manifestRaw, err := yaml.Marshal(manifest)
	if err != nil {
		return Meta{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, "loadout.yaml"), manifestRaw, 0o644); err != nil {
		return Meta{}, err
	}

	if len(sel.ProfileNames) > 0 {
		if err := os.MkdirAll(filepath.Join(dest, "profiles"), 0o755); err != nil {
			return Meta{}, err
		}
		for _, name := range sel.ProfileNames {
			raw, err := os.ReadFile(filepath.Join(s.Root, "profiles", name+".yaml"))
			if err != nil {
				return Meta{}, fmt.Errorf("unknown profile %q: %w", name, err)
			}
			if err := os.WriteFile(filepath.Join(dest, "profiles", name+".yaml"), raw, 0o644); err != nil {
				return Meta{}, err
			}
		}
	}

	if len(sel.LibraryEntryIDs) > 0 {
		all, err := wizard.ListLibrary(s)
		if err != nil {
			return Meta{}, err
		}
		byID := make(map[string]wizard.LibraryEntry, len(all))
		for _, e := range all {
			byID[e.ID] = e
		}
		selected := make([]wizard.LibraryEntry, 0, len(sel.LibraryEntryIDs))
		for _, id := range sel.LibraryEntryIDs {
			e, ok := byID[id]
			if !ok {
				return Meta{}, fmt.Errorf("unknown library entry %q", id)
			}
			selected = append(selected, e)
		}
		libRaw, err := yaml.Marshal(struct {
			Instructions []wizard.LibraryEntry `yaml:"instructions"`
		}{Instructions: selected})
		if err != nil {
			return Meta{}, err
		}
		if err := os.MkdirAll(filepath.Join(dest, "library"), 0o755); err != nil {
			return Meta{}, err
		}
		if err := os.WriteFile(filepath.Join(dest, "library", "instructions.yaml"), libRaw, 0o644); err != nil {
			return Meta{}, err
		}
	}

	if sel.Sources {
		raw, err := os.ReadFile(filepath.Join(s.Root, "sources.lock.yaml"))
		if err != nil && !os.IsNotExist(err) {
			return Meta{}, err
		}
		if err == nil {
			if err := os.WriteFile(filepath.Join(dest, "sources.lock.yaml"), raw, 0o644); err != nil {
				return Meta{}, err
			}
		}
	}

	meta := Meta{
		APIVersion: APIVersion,
		Name:       s.Manifest.Name,
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  createdBy,
		Artifacts:  len(artifacts),
	}
	metaRaw, err := yaml.Marshal(meta)
	if err != nil {
		return Meta{}, err
	}
	if err := os.WriteFile(filepath.Join(dest, MetaFile), metaRaw, 0o644); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// ExportSelectedArchive writes a filtered bundle as a single .loadout.tar.gz.
func ExportSelectedArchive(s *store.Store, destFile, createdBy string, sel Selection) (Meta, error) {
	staging, err := os.MkdirTemp("", "loadout-bundle-*")
	if err != nil {
		return Meta{}, err
	}
	defer os.RemoveAll(staging)

	dir := filepath.Join(staging, "bundle")
	meta, err := ExportSelected(s, dir, createdBy, sel)
	if err != nil {
		return Meta{}, err
	}
	if err := tarGz(dir, destFile); err != nil {
		return Meta{}, err
	}
	return meta, nil
}

// ExportSelectedArchiveTemp exports a filtered archive into a fresh temp
// dir (e.g. for an HTTP download); call Cleanup when done.
func ExportSelectedArchiveTemp(s *store.Store, createdBy string, sel Selection) (TempArchive, error) {
	dir, err := os.MkdirTemp("", "loadout-export-*")
	if err != nil {
		return TempArchive{}, err
	}
	path := filepath.Join(dir, "bundle.loadout.tar.gz")
	if _, err := ExportSelectedArchive(s, path, createdBy, sel); err != nil {
		_ = os.RemoveAll(dir)
		return TempArchive{}, err
	}
	return TempArchive{Path: path, dir: dir}, nil
}

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

// FlatAPIVersion is the schema for the single-file YAML export — a fully
// self-contained alternative to the directory/.tar.gz bundle formats,
// small enough to paste into a gist or a doc. It's the same bundle
// contents as the other formats, just a different transport encoding:
// file bytes are inlined as plain strings instead of living on disk
// inside an archive.
const FlatAPIVersion = "loadout-config/v1"

// FlatArtifact mirrors domain.Artifact but inlines its file content as
// strings (not []byte, which goccy/go-yaml would base64-encode) so the
// document stays human-readable.
type FlatArtifact struct {
	ID      string            `yaml:"id"`
	Kind    string            `yaml:"kind"`
	Path    string            `yaml:"path"`
	Targets []string          `yaml:"targets,omitempty"`
	Origin  string            `yaml:"origin,omitempty"`
	Tags    []string          `yaml:"tags,omitempty"`
	Files   map[string]string `yaml:"files"`
}

// FlatProfile pairs a saved profile's name with its answers (wizard's
// SaveProfile/LoadProfile keep these separate; the flat document needs
// them together).
type FlatProfile struct {
	Name    string         `yaml:"name"`
	Answers wizard.Answers `yaml:"answers"`
}

// FlatConfig is the full single-file document.
type FlatConfig struct {
	APIVersion string                `yaml:"apiVersion"`
	Name       string                `yaml:"name"`
	CreatedAt  time.Time             `yaml:"createdAt"`
	CreatedBy  string                `yaml:"createdBy"`
	Artifacts  []FlatArtifact        `yaml:"artifacts"`
	Profiles   []FlatProfile         `yaml:"profiles,omitempty"`
	Library    []wizard.LibraryEntry `yaml:"library,omitempty"`
	Groups     []wizard.LibraryGroup `yaml:"libraryGroups,omitempty"`
	Sources    []domain.SourceRef    `yaml:"sources,omitempty"`
}

// ExportFlat renders a Selection as one self-contained YAML document. It
// reuses ExportSelected's filtering by staging into a temp directory first
// and reading that back into the flat shape, rather than duplicating
// selection logic — the flat format is just an alternate encoding of the
// same bundle contents.
func ExportFlat(s *store.Store, createdBy string, sel Selection) ([]byte, error) {
	staging, err := os.MkdirTemp("", "loadout-flat-export-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	dir := filepath.Join(staging, "bundle")
	if _, err := ExportSelected(s, dir, createdBy, sel); err != nil {
		return nil, err
	}
	staged, err := store.Open(dir)
	if err != nil {
		return nil, err
	}

	artifacts := make([]FlatArtifact, 0, len(staged.Manifest.Artifacts))
	for _, art := range staged.Manifest.Artifacts {
		content, err := staged.Content(art)
		if err != nil {
			return nil, err
		}
		files := make(map[string]string, len(content.Files))
		for rel, raw := range content.Files {
			files[rel] = string(raw)
		}
		artifacts = append(artifacts, FlatArtifact{
			ID: art.ID, Kind: string(art.Kind), Path: art.Path,
			Targets: art.Targets, Origin: art.Origin, Tags: art.Tags, Files: files,
		})
	}

	var profiles []FlatProfile
	names, err := wizard.ListProfiles(staged)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		answers, err := wizard.LoadProfile(staged, name)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, FlatProfile{Name: name, Answers: answers})
	}

	library, err := wizard.ListLibrary(staged)
	if err != nil {
		return nil, err
	}

	// Groups aren't individually selectable (only library entries are); a
	// group is included only when every one of its member entries was also
	// selected, so the document never references an entry it doesn't carry.
	selectedIDs := make(map[string]bool, len(sel.LibraryEntryIDs))
	for _, id := range sel.LibraryEntryIDs {
		selectedIDs[id] = true
	}
	allGroups, err := wizard.ListGroups(s)
	if err != nil {
		return nil, err
	}
	var groups []wizard.LibraryGroup
	for _, g := range allGroups {
		complete := len(g.EntryIDs) > 0
		for _, id := range g.EntryIDs {
			if !selectedIDs[id] {
				complete = false
				break
			}
		}
		if complete {
			groups = append(groups, g)
		}
	}

	var sources []domain.SourceRef
	if sel.Sources {
		sources, err = (&sourceLockReader{root: dir}).read()
		if err != nil {
			return nil, err
		}
	}

	cfg := FlatConfig{
		APIVersion: FlatAPIVersion,
		Name:       s.Manifest.Name,
		CreatedAt:  time.Now().UTC(),
		CreatedBy:  createdBy,
		Artifacts:  artifacts,
		Profiles:   profiles,
		Library:    library,
		Groups:     groups,
		Sources:    sources,
	}
	return yaml.Marshal(cfg)
}

// unpackFlat writes a flat config document into a temp directory shaped
// like a normal bundle (bundle.yaml + loadout.yaml + skills/<id>/... from
// the inlined Files), so PlanImport can read it exactly like any other
// bundle — no separate diff/merge logic for the flat format.
func unpackFlat(raw []byte) (root string, cleanup func(), err error) {
	var cfg FlatConfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return "", nil, fmt.Errorf("parse flat config: %w", err)
	}
	if cfg.APIVersion != FlatAPIVersion {
		return "", nil, fmt.Errorf("not a loadout bundle (unrecognized apiVersion %q)", cfg.APIVersion)
	}

	tmp, err := os.MkdirTemp("", "loadout-flat-*")
	if err != nil {
		return "", nil, err
	}
	cleanupFn := func() { os.RemoveAll(tmp) }
	fail := func(err error) (string, func(), error) {
		cleanupFn()
		return "", nil, err
	}

	artifacts := make([]domain.Artifact, 0, len(cfg.Artifacts))
	for _, fa := range cfg.Artifacts {
		for rel, content := range fa.Files {
			dest := filepath.Join(tmp, filepath.FromSlash(fa.Path), filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return fail(err)
			}
			if err := os.WriteFile(dest, []byte(content), 0o644); err != nil {
				return fail(err)
			}
		}
		artifacts = append(artifacts, domain.Artifact{
			ID: fa.ID, Kind: domain.ArtifactKind(fa.Kind), Path: fa.Path,
			Targets: fa.Targets, Origin: fa.Origin, Tags: fa.Tags,
		})
	}
	manifest := domain.Manifest{APIVersion: domain.APIVersion, Name: cfg.Name, Artifacts: artifacts}
	manifestRaw, err := yaml.Marshal(manifest)
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "loadout.yaml"), manifestRaw, 0o644); err != nil {
		return fail(err)
	}

	if len(cfg.Profiles) > 0 {
		if err := os.MkdirAll(filepath.Join(tmp, "profiles"), 0o755); err != nil {
			return fail(err)
		}
		for _, p := range cfg.Profiles {
			raw, err := yaml.Marshal(p.Answers)
			if err != nil {
				return fail(err)
			}
			if err := os.WriteFile(filepath.Join(tmp, "profiles", p.Name+".yaml"), raw, 0o644); err != nil {
				return fail(err)
			}
		}
	}

	if len(cfg.Library) > 0 {
		if err := os.MkdirAll(filepath.Join(tmp, "library"), 0o755); err != nil {
			return fail(err)
		}
		raw, err := yaml.Marshal(struct {
			Instructions []wizard.LibraryEntry `yaml:"instructions"`
		}{Instructions: cfg.Library})
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "library", "instructions.yaml"), raw, 0o644); err != nil {
			return fail(err)
		}
	}

	if len(cfg.Groups) > 0 {
		if err := os.MkdirAll(filepath.Join(tmp, "library"), 0o755); err != nil {
			return fail(err)
		}
		raw, err := yaml.Marshal(struct {
			Groups []wizard.LibraryGroup `yaml:"groups"`
		}{Groups: cfg.Groups})
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "library", "groups.yaml"), raw, 0o644); err != nil {
			return fail(err)
		}
	}

	if len(cfg.Sources) > 0 {
		raw, err := yaml.Marshal(map[string][]domain.SourceRef{"sources": cfg.Sources})
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(filepath.Join(tmp, "sources.lock.yaml"), raw, 0o644); err != nil {
			return fail(err)
		}
	}

	meta := Meta{APIVersion: APIVersion, Name: cfg.Name, CreatedAt: cfg.CreatedAt, CreatedBy: cfg.CreatedBy, Artifacts: len(artifacts)}
	metaRaw, err := yaml.Marshal(meta)
	if err != nil {
		return fail(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, MetaFile), metaRaw, 0o644); err != nil {
		return fail(err)
	}

	return tmp, cleanupFn, nil
}

// ImportFlat validates and diffs a flat config document against the local
// store, exactly like PlanImport does for a directory or .tar.gz bundle —
// it just unpacks the flat document into a temp directory first.
func ImportFlat(s *store.Store, raw []byte) (*ImportPlan, error) {
	root, cleanup, err := unpackFlat(raw)
	if err != nil {
		return nil, err
	}
	plan, err := PlanImport(s, root)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return nil, err
	}
	// PlanImport's directory branch leaves cleanup nil (it doesn't own
	// directories it didn't create) — here we did create one, so wire our
	// own cleanup into the plan the caller already knows how to Close().
	plan.cleanup = cleanup
	return plan, nil
}

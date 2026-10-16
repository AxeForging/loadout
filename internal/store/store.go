// Package store manages the canonical loadout store on disk: a plain,
// git-friendly directory holding loadout.yaml plus one directory or file per
// artifact. The store never touches agent homes — that is the engine's job.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/goccy/go-yaml"
)

// ManifestFile is the manifest's file name inside the store root.
const ManifestFile = "loadout.yaml"

// kindDirs maps each artifact kind to its directory inside the store.
var kindDirs = map[domain.ArtifactKind]string{
	domain.KindSkill:       "skills",
	domain.KindInstruction: "instructions",
	domain.KindCommand:     "commands",
	domain.KindHook:        "hooks",
	domain.KindSettings:    "settings",
	domain.KindMCP:         "mcps",
}

// Store is an opened canonical store.
type Store struct {
	Root     string
	Manifest domain.Manifest
}

// Open reads an existing store.
func Open(root string) (*Store, error) {
	raw, err := os.ReadFile(filepath.Join(root, ManifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no store at %s (run 'loadout init' first)", root)
		}
		return nil, err
	}
	var m domain.Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	if m.APIVersion != domain.APIVersion {
		return nil, fmt.Errorf("unsupported manifest apiVersion %q (want %s)", m.APIVersion, domain.APIVersion)
	}
	return &Store{Root: root, Manifest: m}, nil
}

// Init creates a new store: directories, a starter manifest and (best-effort)
// a git repository. It refuses to overwrite an existing manifest.
func Init(root, name string) (*Store, error) {
	if _, err := os.Stat(filepath.Join(root, ManifestFile)); err == nil {
		return nil, fmt.Errorf("store already exists at %s", root)
	}
	for _, d := range kindDirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			return nil, err
		}
	}
	s := &Store{
		Root: root,
		Manifest: domain.Manifest{
			APIVersion: domain.APIVersion,
			Name:       name,
			Artifacts:  []domain.Artifact{},
		},
	}
	if err := s.SaveManifest(); err != nil {
		return nil, err
	}
	gitInit(root)
	return s, nil
}

// gitInit makes the store a git repo if git is available; failure is not fatal.
func gitInit(root string) {
	if _, err := os.Stat(filepath.Join(root, ".git")); err == nil {
		return
	}
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = root
	_ = cmd.Run()
}

// SaveManifest writes loadout.yaml.
func (s *Store) SaveManifest() error {
	raw, err := yaml.Marshal(s.Manifest)
	if err != nil {
		return err
	}
	return WriteFileAtomic(filepath.Join(s.Root, ManifestFile), raw, 0o644)
}

// Artifact looks up a manifest entry by ID.
func (s *Store) Artifact(id string) (domain.Artifact, bool) {
	for _, a := range s.Manifest.Artifacts {
		if a.ID == id {
			return a, true
		}
	}
	return domain.Artifact{}, false
}

// AddArtifact appends a manifest entry and persists it.
func (s *Store) AddArtifact(a domain.Artifact) error {
	if _, exists := s.Artifact(a.ID); exists {
		return fmt.Errorf("artifact %q already exists", a.ID)
	}
	s.Manifest.Artifacts = append(s.Manifest.Artifacts, a)
	return s.SaveManifest()
}

// RemoveArtifact drops an artifact's manifest entry and deletes its content
// files. Manifest first: a crash in between leaves orphaned content files on
// disk (harmless — the manifest is the source of truth), never a manifest
// entry pointing at nothing.
func (s *Store) RemoveArtifact(id string) error {
	idx := -1
	for i, a := range s.Manifest.Artifacts {
		if a.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("artifact %q not found", id)
	}
	a := s.Manifest.Artifacts[idx]
	s.Manifest.Artifacts = append(s.Manifest.Artifacts[:idx], s.Manifest.Artifacts[idx+1:]...)
	if err := s.SaveManifest(); err != nil {
		return err
	}
	abs := filepath.Join(s.Root, a.Path)
	if rel, err := filepath.Rel(s.Root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact %q: content path %q escapes the store — entry removed, files left untouched", id, a.Path)
	}
	return os.RemoveAll(abs)
}

// Content reads an artifact's file set. Directory artifacts return every
// regular file keyed by relative path; file artifacts return one entry keyed
// by base name.
func (s *Store) Content(a domain.Artifact) (domain.Content, error) {
	c := domain.Content{Files: map[string][]byte{}}
	abs := filepath.Join(s.Root, a.Path)
	info, err := os.Stat(abs)
	if err != nil {
		return c, fmt.Errorf("artifact %s: %w", a.ID, err)
	}
	if !info.IsDir() {
		raw, err := os.ReadFile(abs)
		if err != nil {
			return c, err
		}
		c.Files[filepath.Base(abs)] = raw
		return c, nil
	}
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(abs, path)
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

// ContentRelPath maps a content file key (as returned by Content) to its
// store-relative path.
func (s *Store) ContentRelPath(a domain.Artifact, key string) string {
	abs := filepath.Join(s.Root, a.Path)
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		return filepath.ToSlash(a.Path)
	}
	return path.Join(filepath.ToSlash(a.Path), key)
}

// WriteContent overwrites existing content files of an artifact, keyed as
// Content returns them. It refuses keys that are not already part of the
// artifact or that escape its directory — it updates, never adds.
func (s *Store) WriteContent(a domain.Artifact, files map[string][]byte) error {
	current, err := s.Content(a)
	if err != nil {
		return err
	}
	for key, raw := range files {
		if _, ok := current.Files[key]; !ok {
			return fmt.Errorf("artifact %s has no file %q", a.ID, key)
		}
		abs := filepath.Join(s.Root, filepath.FromSlash(s.ContentRelPath(a, key)))
		if rel, err := filepath.Rel(s.Root, abs); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("artifact %s: %q escapes the store", a.ID, key)
		}
		mode := fs.FileMode(0o644)
		if info, err := os.Stat(abs); err == nil {
			mode = info.Mode().Perm()
		}
		if err := WriteFileAtomic(abs, raw, mode); err != nil {
			return err
		}
	}
	return nil
}

// Hash returns a stable content hash over an artifact's file set.
func Hash(c domain.Content) string {
	paths := make([]string, 0, len(c.Files))
	for p := range c.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		fmt.Fprintf(h, "%s\x00%d\x00", p, len(c.Files[p]))
		h.Write(c.Files[p])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// HashBytes returns the hash of a single blob.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// ValidateName enforces kebab-case artifact names so IDs stay filesystem- and
// URL-safe everywhere.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			continue
		}
		return fmt.Errorf("invalid name %q: use lowercase letters, digits and dashes", name)
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		return fmt.Errorf("invalid name %q: must not start or end with a dash", name)
	}
	return nil
}

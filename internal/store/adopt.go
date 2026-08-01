package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
)

// Adopt copies an existing native artifact (e.g. a skill directory from
// ~/.claude/skills/) into the store and registers it in the manifest. The
// source is never modified.
func (s *Store) Adopt(kind domain.ArtifactKind, name, src string) (domain.Artifact, error) {
	return s.AdoptFrom(kind, name, src, "local")
}

// AdoptFrom is Adopt with an explicit origin (e.g. "source:<name>" for
// artifacts imported from an external repository).
func (s *Store) AdoptFrom(kind domain.ArtifactKind, name, src, origin string) (domain.Artifact, error) {
	return s.AdoptAs(kind, name, src, origin, "")
}

// AdoptAs is AdoptFrom for aliased imports: sourceName is the artifact's
// real upstream name when it differs from the local name (empty otherwise).
func (s *Store) AdoptAs(kind domain.ArtifactKind, name, src, origin, sourceName string) (domain.Artifact, error) {
	if err := ValidateName(name); err != nil {
		return domain.Artifact{}, err
	}
	dir, ok := kindDirs[kind]
	if !ok {
		return domain.Artifact{}, fmt.Errorf("unknown artifact kind %q", kind)
	}
	if _, exists := s.Artifact(name); exists {
		return domain.Artifact{}, fmt.Errorf("artifact %q already exists", name)
	}
	info, err := os.Stat(src)
	if err != nil {
		return domain.Artifact{}, err
	}

	var relPath string
	switch kind {
	case domain.KindSkill, domain.KindHook, domain.KindMCP:
		if !info.IsDir() {
			return domain.Artifact{}, fmt.Errorf("%s artifacts are directories, %s is a file", kind, src)
		}
		relPath = filepath.Join(dir, name)
		if err := copyDir(src, filepath.Join(s.Root, relPath)); err != nil {
			return domain.Artifact{}, err
		}
	case domain.KindInstruction, domain.KindCommand:
		if info.IsDir() {
			return domain.Artifact{}, fmt.Errorf("%s artifacts are single files, %s is a directory", kind, src)
		}
		relPath = filepath.Join(dir, name+".md")
		if err := copyFile(src, filepath.Join(s.Root, relPath), info.Mode()); err != nil {
			return domain.Artifact{}, err
		}
	default:
		return domain.Artifact{}, fmt.Errorf("adopting %s artifacts is not supported", kind)
	}

	if sourceName == name {
		sourceName = ""
	}
	a := domain.Artifact{
		ID:         name,
		Kind:       kind,
		Path:       filepath.ToSlash(relPath),
		Origin:     origin,
		SourceName: sourceName,
	}
	if strings.HasPrefix(origin, "source:") {
		content, err := s.Content(a)
		if err != nil {
			return domain.Artifact{}, err
		}
		a.SourceHash = Hash(content)
	}
	if err := s.AddArtifact(a); err != nil {
		return domain.Artifact{}, err
	}
	return a, nil
}

// SetSourceHash updates an artifact's import baseline and persists the
// manifest. Used by source updates after refreshing an artifact's content.
func (s *Store) SetSourceHash(id, hash string) error {
	for i := range s.Manifest.Artifacts {
		if s.Manifest.Artifacts[i].ID == id {
			s.Manifest.Artifacts[i].SourceHash = hash
			return s.SaveManifest()
		}
	}
	return fmt.Errorf("artifact %q not found", id)
}

// CopyTree copies a directory recursively, preserving file modes. Used by
// source updates to refresh imported artifacts in place.
func CopyTree(src, dst string) error {
	return copyDir(src, dst)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, raw, mode.Perm())
}

package store

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/AxeForging/loadout/internal/domain"
)

// Adopt copies an existing native artifact (e.g. a skill directory from
// ~/.claude/skills/) into the store and registers it in the manifest. The
// source is never modified.
func (s *Store) Adopt(kind domain.ArtifactKind, name, src string) (domain.Artifact, error) {
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
	case domain.KindSkill, domain.KindHook:
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

	a := domain.Artifact{
		ID:     name,
		Kind:   kind,
		Path:   filepath.ToSlash(relPath),
		Origin: "local",
	}
	if err := s.AddArtifact(a); err != nil {
		return domain.Artifact{}, err
	}
	return a, nil
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

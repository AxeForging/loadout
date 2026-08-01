package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func TestAdopt_SkillDirectoryCopiedRecursivelyWithModes(t *testing.T) {
	s := testStore(t)
	src := filepath.Join(t.TempDir(), "native-skill")
	if err := os.MkdirAll(filepath.Join(src, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: native-skill\n---\n"), 0o644)
	os.WriteFile(filepath.Join(src, "scripts", "go.sh"), []byte("#!/bin/sh\n"), 0o755)

	a, err := s.Adopt(domain.KindSkill, "native-skill", src)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(s.Root, a.Path, "scripts", "go.sh"))
	if err != nil {
		t.Fatalf("script not copied: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("script lost its executable bit")
	}
	if _, found := s.Artifact("native-skill"); !found {
		t.Error("artifact not registered")
	}
	// Source must be untouched.
	if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err != nil {
		t.Error("source was modified")
	}
}

func TestAdopt_CommandFileGetsCanonicalName(t *testing.T) {
	s := testStore(t)
	src := filepath.Join(t.TempDir(), "plan.md")
	os.WriteFile(src, []byte("do the plan\n"), 0o644)

	a, err := s.Adopt(domain.KindCommand, "plan", src)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, a.Path))
	if err != nil || !strings.Contains(string(raw), "do the plan") {
		t.Fatalf("adopted file wrong: %v %s", err, raw)
	}
}

func TestAdopt_RejectsShapeMismatch(t *testing.T) {
	s := testStore(t)
	file := filepath.Join(t.TempDir(), "x.md")
	os.WriteFile(file, []byte("x"), 0o644)

	if _, err := s.Adopt(domain.KindSkill, "x", file); err == nil {
		t.Error("adopting a file as a skill should fail")
	}
	if _, err := s.Adopt(domain.KindCommand, "dir", t.TempDir()); err == nil {
		t.Error("adopting a directory as a command should fail")
	}
}

func TestAdoptFrom_SourceOriginRecordsBaselineHash(t *testing.T) {
	s := testStore(t)
	src := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: x\n---\nbody\n"), 0o644)

	a, err := s.AdoptFrom(domain.KindSkill, "sourced", src, domain.OriginSource("team"))
	if err != nil {
		t.Fatalf("AdoptFrom() error = %v", err)
	}
	if a.SourceHash == "" {
		t.Fatal("source-origin adopt must record a baseline hash")
	}
	content, err := s.Content(a)
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceHash != Hash(content) {
		t.Errorf("baseline hash %q does not match adopted content hash %q", a.SourceHash, Hash(content))
	}
	// The persisted manifest carries the hash too.
	saved, _ := s.Artifact("sourced")
	if saved.SourceHash != a.SourceHash {
		t.Error("baseline hash not persisted in the manifest")
	}
}

func TestAdopt_LocalOriginRecordsNoBaselineHash(t *testing.T) {
	s := testStore(t)
	src := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("---\nname: y\n---\n"), 0o644)

	a, err := s.Adopt(domain.KindSkill, "local-skill", src)
	if err != nil {
		t.Fatalf("Adopt() error = %v", err)
	}
	if a.SourceHash != "" {
		t.Errorf("local adopt must not record a baseline hash, got %q", a.SourceHash)
	}
}

func TestSetSourceHash_PersistsAndErrorsOnUnknown(t *testing.T) {
	s := testStore(t)
	src := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("v1"), 0o644)
	if _, err := s.AdoptFrom(domain.KindSkill, "sk", src, domain.OriginSource("team")); err != nil {
		t.Fatal(err)
	}

	if err := s.SetSourceHash("sk", "newhash"); err != nil {
		t.Fatalf("SetSourceHash() error = %v", err)
	}
	reopened, err := Open(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := reopened.Artifact("sk")
	if a.SourceHash != "newhash" {
		t.Errorf("reopened store hash = %q, want %q", a.SourceHash, "newhash")
	}

	if err := s.SetSourceHash("missing", "h"); err == nil {
		t.Error("SetSourceHash on unknown artifact must error")
	}
}

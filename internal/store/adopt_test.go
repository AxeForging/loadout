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

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func TestInitAndOpen_RoundTripsManifest(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	s, err := Init(root, "team-kit")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if s.Manifest.APIVersion != domain.APIVersion {
		t.Errorf("APIVersion = %q, want %q", s.Manifest.APIVersion, domain.APIVersion)
	}

	reopened, err := Open(root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if reopened.Manifest.Name != "team-kit" {
		t.Errorf("Name = %q, want team-kit", reopened.Manifest.Name)
	}
}

func TestInit_RefusesToOverwriteExistingStore(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	if _, err := Init(root, "one"); err != nil {
		t.Fatalf("first Init() error = %v", err)
	}
	if _, err := Init(root, "two"); err == nil {
		t.Fatal("second Init() should fail, got nil")
	}
}

func TestOpen_FailsWithHintWhenStoreMissing(t *testing.T) {
	_, err := Open(filepath.Join(t.TempDir(), "nowhere"))
	if err == nil || !strings.Contains(err.Error(), "loadout init") {
		t.Fatalf("Open() error = %v, want hint to run loadout init", err)
	}
}

func TestNew_SkillScaffoldsSkillMDAndRegistersArtifact(t *testing.T) {
	s := testStore(t)
	a, err := s.New(domain.KindSkill, "code-review")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, "skills", "code-review", "SKILL.md"))
	if err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}
	if !strings.Contains(string(raw), "name: code-review") {
		t.Errorf("SKILL.md missing frontmatter name, got:\n%s", raw)
	}
	if _, found := s.Artifact(a.ID); !found {
		t.Error("artifact not registered in manifest")
	}

	reopened, err := Open(s.Root)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, found := reopened.Artifact("code-review"); !found {
		t.Error("manifest entry not persisted")
	}
}

func TestNew_HookScaffoldsSpecAndExecutableScript(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindHook, "commit-msg-lint"); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	info, err := os.Stat(filepath.Join(s.Root, "hooks", "commit-msg-lint", "run.sh"))
	if err != nil {
		t.Fatalf("run.sh not created: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Errorf("run.sh not executable, mode = %v", info.Mode())
	}
	if _, err := os.Stat(filepath.Join(s.Root, "hooks", "commit-msg-lint", "hook.yaml")); err != nil {
		t.Fatalf("hook.yaml not created: %v", err)
	}
}

func TestCreateMCP_WritesValidatedYAMLAndRegisters(t *testing.T) {
	s := testStore(t)
	yaml := "transport: stdio\ncommand: npx\nargs: [\"-y\", \"@x/y\"]\nscope: user\n"
	a, err := s.CreateMCP("context7", []byte(yaml))
	if err != nil {
		t.Fatalf("CreateMCP() error = %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(s.Root, "mcps", "context7", "mcp.yaml"))
	if err != nil {
		t.Fatalf("mcp.yaml not created: %v", err)
	}
	if string(raw) != yaml {
		t.Errorf("mcp.yaml content changed on write:\n%s", raw)
	}
	if a.Kind != domain.KindMCP {
		t.Errorf("kind = %q, want mcp", a.Kind)
	}
	if _, found := s.Artifact("context7"); !found {
		t.Error("artifact not registered in manifest")
	}
}

func TestCreateMCP_RejectsInvalidSpecBeforeTouchingDisk(t *testing.T) {
	s := testStore(t)
	// A literal secret must never land in a shareable store.
	bad := "transport: http\nurl: https://x\nheaders:\n  Authorization: Bearer sk-live-123\n"
	if _, err := s.CreateMCP("leaky", []byte(bad)); err == nil {
		t.Fatal("literal secret should fail, got nil")
	}
	if _, err := os.Stat(filepath.Join(s.Root, "mcps", "leaky")); !os.IsNotExist(err) {
		t.Error("rejected MCP left files on disk")
	}
	if _, err := s.CreateMCP("no-command", []byte("transport: stdio\n")); err == nil {
		t.Fatal("stdio without command should fail, got nil")
	}
}

func TestNew_RejectsDuplicateID(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindSkill, "dup"); err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := s.New(domain.KindCommand, "dup"); err == nil {
		t.Fatal("duplicate ID should fail, got nil")
	}
}

func TestContent_DirectoryArtifactReturnsAllFilesByRelPath(t *testing.T) {
	s := testStore(t)
	a, err := s.New(domain.KindSkill, "multi")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	scriptPath := filepath.Join(s.Root, a.Path, "scripts", "run.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	c, err := s.Content(a)
	if err != nil {
		t.Fatalf("Content() error = %v", err)
	}
	if _, ok := c.Files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md in content")
	}
	if _, ok := c.Files["scripts/run.sh"]; !ok {
		t.Errorf("missing scripts/run.sh in content, got keys %v", keys(c))
	}
}

func TestHash_ChangesWithContentButNotMapOrder(t *testing.T) {
	a := domain.Content{Files: map[string][]byte{"a.md": []byte("one"), "b.md": []byte("two")}}
	b := domain.Content{Files: map[string][]byte{"b.md": []byte("two"), "a.md": []byte("one")}}
	if Hash(a) != Hash(b) {
		t.Error("hash should be independent of map insertion order")
	}
	c := domain.Content{Files: map[string][]byte{"a.md": []byte("one!"), "b.md": []byte("two")}}
	if Hash(a) == Hash(c) {
		t.Error("hash should change when content changes")
	}
}

func TestValidateName_RejectsNonKebabCase(t *testing.T) {
	for _, bad := range []string{"", "Has Caps", "under_score", "-lead", "trail-", "dot.name"} {
		if err := ValidateName(bad); err == nil {
			t.Errorf("ValidateName(%q) should fail", bad)
		}
	}
	for _, good := range []string{"a", "code-review", "sdd-2"} {
		if err := ValidateName(good); err != nil {
			t.Errorf("ValidateName(%q) error = %v", good, err)
		}
	}
}

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Init(filepath.Join(t.TempDir(), "store"), "test")
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	return s
}

func keys(c domain.Content) []string {
	out := make([]string, 0, len(c.Files))
	for k := range c.Files {
		out = append(out, k)
	}
	return out
}

func TestRemoveArtifact_DropsManifestEntryAndContent(t *testing.T) {
	s, err := Init(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindSkill, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindCommand, "kept"); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveArtifact("gone"); err != nil {
		t.Fatalf("RemoveArtifact() error = %v", err)
	}
	if _, found := s.Artifact("gone"); found {
		t.Error("manifest entry should be gone")
	}
	if _, err := os.Stat(filepath.Join(s.Root, "skills", "gone")); !os.IsNotExist(err) {
		t.Error("content directory should be deleted")
	}

	// Persisted, not just in memory — and the other artifact survives intact.
	reopened, err := Open(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := reopened.Artifact("gone"); found {
		t.Error("removal not persisted to loadout.yaml")
	}
	if _, found := reopened.Artifact("kept"); !found {
		t.Error("unrelated artifact lost")
	}
}

func TestRemoveArtifact_UnknownIDErrors(t *testing.T) {
	s, err := Init(t.TempDir(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveArtifact("nope"); err == nil {
		t.Fatal("want an error for an unknown artifact")
	}
}

func TestRemoveArtifact_EscapingContentPathRefusesFileDeletion(t *testing.T) {
	dir := t.TempDir()
	s, err := Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(dir, "precious.txt")
	if err := os.WriteFile(outside, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Manifest.Artifacts = append(s.Manifest.Artifacts, domain.Artifact{
		ID: "evil", Kind: domain.KindCommand, Path: "../precious.txt",
	})
	if err := s.SaveManifest(); err != nil {
		t.Fatal(err)
	}

	if err := s.RemoveArtifact("evil"); err == nil {
		t.Fatal("want an error for a content path escaping the store")
	}
	if _, statErr := os.Stat(outside); statErr != nil {
		t.Error("file outside the store must never be deleted")
	}
	if _, found := s.Artifact("evil"); found {
		t.Error("manifest entry should still be removed")
	}
}

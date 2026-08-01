//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// makeUpstream2 builds a fake upstream repo with two skills so one can move
// upstream while the other is edited locally.
func makeUpstream2(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "team-skills")
	for name, body := range map[string]string{"alpha": "alpha v1", "beta": "beta v1"} {
		skill := filepath.Join(dir, name)
		if err := os.MkdirAll(skill, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "---\nname: " + name + "\ndescription: test\n---\n\n" + body + "\n"
		if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "config", "user.email", "t@example.com")
	gitIn(t, dir, "config", "user.name", "t")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "initial")
	return dir
}

func commitUpstreamSkill(t *testing.T, upstream, name, body, msg string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: test\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(upstream, name, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, upstream, "add", "-A")
	gitIn(t, upstream, "commit", "-m", msg)
}

func storeSkillPath(e *env, name string) string {
	return filepath.Join(e.home, ".local", "share", "loadout", "store", "skills", name, "SKILL.md")
}

func editStoreSkill(t *testing.T, e *env, name, body string) {
	t.Helper()
	content := "---\nname: " + name + "\ndescription: test\n---\n\n" + body + "\n"
	if err := os.WriteFile(storeSkillPath(e, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestUpdate_PreservesLocalEditsWhenUpstreamUnchanged is the regression test
// spec 008 exists for: a skill edited in the store must survive an update
// driven by an unrelated upstream commit, even with --yes.
func TestUpdate_PreservesLocalEditsWhenUpstreamUnchanged(t *testing.T) {
	e := newEnv(t)
	upstream := makeUpstream2(t)
	e.mustRun("init")
	e.mustRun("source", "add", upstream, "--name", "team-skills")

	// User customizes alpha in the store; upstream only touches beta.
	editStoreSkill(t, e, "alpha", "alpha CUSTOMIZED locally")
	commitUpstreamSkill(t, upstream, "beta", "beta v2", "feat: improve beta")

	out := e.mustRun("update", "--yes")
	if !strings.Contains(out, "locally edited") {
		t.Fatalf("plan should report alpha as locally edited:\n%s", out)
	}

	raw, _ := os.ReadFile(storeSkillPath(e, "alpha"))
	if !strings.Contains(string(raw), "alpha CUSTOMIZED locally") {
		t.Fatalf("local edit to alpha was discarded by update --yes:\n%s", raw)
	}
	raw, _ = os.ReadFile(storeSkillPath(e, "beta"))
	if !strings.Contains(string(raw), "beta v2") {
		t.Fatalf("beta should have been updated:\n%s", raw)
	}
}

// TestUpdate_ConflictNeedsForceDiscard: local edit + upstream change to the
// same skill is a conflict — skipped by default, discarded only with
// --force-discard.
func TestUpdate_ConflictNeedsForceDiscard(t *testing.T) {
	e := newEnv(t)
	upstream := makeUpstream2(t)
	e.mustRun("init")
	e.mustRun("source", "add", upstream, "--name", "team-skills")

	editStoreSkill(t, e, "alpha", "alpha CUSTOMIZED locally")
	commitUpstreamSkill(t, upstream, "alpha", "alpha v2 upstream", "feat: rework alpha")

	// Default: conflict is reported and skipped, even with --yes.
	out := e.mustRun("update", "--yes")
	if !strings.Contains(out, "conflict") {
		t.Fatalf("plan should report the alpha conflict:\n%s", out)
	}
	raw, _ := os.ReadFile(storeSkillPath(e, "alpha"))
	if !strings.Contains(string(raw), "alpha CUSTOMIZED locally") {
		t.Fatalf("conflicted skill was overwritten without --force-discard:\n%s", raw)
	}

	// The pin moved, so re-add an upstream commit to have an update to apply
	// again; then force the discard.
	commitUpstreamSkill(t, upstream, "beta", "beta v3", "chore: beta again")
	out = e.mustRun("update", "--yes", "--force-discard")
	if !strings.Contains(strings.ToLower(out), "discard") {
		t.Fatalf("forced plan should name what it discards:\n%s", out)
	}
	raw, _ = os.ReadFile(storeSkillPath(e, "alpha"))
	if !strings.Contains(string(raw), "alpha v2 upstream") {
		t.Fatalf("--force-discard should have applied upstream alpha:\n%s", raw)
	}
}

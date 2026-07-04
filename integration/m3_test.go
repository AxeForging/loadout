//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitIn runs git in dir for building a fake upstream skills repo.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func makeUpstream(t *testing.T, skillBody string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "team-skills")
	skill := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: review code\n---\n\n" + skillBody + "\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "config", "user.email", "t@example.com")
	gitIn(t, dir, "config", "user.name", "t")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "initial")
	return dir
}

func TestSourceFlow_AddCheckUpdateSync(t *testing.T) {
	e := newEnv(t)
	upstream := makeUpstream(t, "version one")
	e.mustRun("init")

	out := e.mustRun("source", "add", upstream, "--name", "team-skills")
	if !strings.Contains(out, "+ skill code-review") {
		t.Fatalf("source add output:\n%s", out)
	}

	out = e.mustRun("source", "list")
	if !strings.Contains(out, "team-skills") {
		t.Fatalf("source list output:\n%s", out)
	}

	out = e.mustRun("check")
	if !strings.Contains(out, "up-to-date") {
		t.Fatalf("check should be up-to-date:\n%s", out)
	}

	// Imported skill syncs into the agent home like any other artifact.
	e.mustRun("sync", "--yes")
	raw, err := os.ReadFile(e.claude("skills", "code-review", "SKILL.md"))
	if err != nil || !strings.Contains(string(raw), "version one") {
		t.Fatalf("imported skill not installed: %v\n%s", err, raw)
	}

	// Upstream moves.
	skillMD := filepath.Join(upstream, "code-review", "SKILL.md")
	if err := os.WriteFile(skillMD, []byte("---\nname: code-review\ndescription: review code\n---\n\nversion two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, upstream, "add", "-A")
	gitIn(t, upstream, "commit", "-m", "feat: improve review skill")

	out = e.mustRun("check")
	if !strings.Contains(out, "update available") {
		t.Fatalf("check should see the update:\n%s", out)
	}

	// Update without confirmation must not change the store.
	if out, err := e.run("", "update"); err != nil {
		t.Fatalf("update (declined) errored: %v\n%s", err, out)
	}
	storeSkill := filepath.Join(e.home, ".local", "share", "loadout", "store", "skills", "code-review", "SKILL.md")
	raw, _ = os.ReadFile(storeSkill)
	if strings.Contains(string(raw), "version two") {
		t.Fatal("update applied without confirmation")
	}

	out = e.mustRun("update", "--yes")
	if !strings.Contains(out, "applied") || !strings.Contains(out, "feat: improve review skill") {
		t.Fatalf("update output:\n%s", out)
	}
	raw, _ = os.ReadFile(storeSkill)
	if !strings.Contains(string(raw), "version two") {
		t.Fatal("store not refreshed after update --yes")
	}

	// Store drifted ahead of the agent home — status must say so, sync fixes it.
	out = e.mustRun("status", "code-review")
	if !strings.Contains(out, "drifted-store") {
		t.Fatalf("status after update:\n%s", out)
	}
	e.mustRun("sync", "--yes")
	raw, _ = os.ReadFile(e.claude("skills", "code-review", "SKILL.md"))
	if !strings.Contains(string(raw), "version two") {
		t.Fatal("agent home not refreshed after sync")
	}
}

func TestSourceRemove_PurgeCleansStore(t *testing.T) {
	e := newEnv(t)
	upstream := makeUpstream(t, "v1")
	e.mustRun("init")
	e.mustRun("source", "add", upstream, "--name", "team-skills")
	e.mustRun("source", "remove", "team-skills", "--purge")

	out := e.mustRun("source", "list")
	if !strings.Contains(out, "No sources registered") {
		t.Fatalf("source list after remove:\n%s", out)
	}
	out = e.mustRun("status")
	if strings.Contains(out, "code-review") {
		t.Fatalf("purged skill still in status:\n%s", out)
	}
}

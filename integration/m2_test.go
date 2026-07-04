//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newMultiAgentEnv fakes every supported agent home under one temp HOME.
func newMultiAgentEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t) // creates ~/.claude
	for _, dir := range []string{".codex", ".gemini/antigravity-cli", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(e.home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestSync_InstallsAcrossAllFiveAgents(t *testing.T) {
	e := newMultiAgentEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("new", "instruction", "style")
	e.mustRun("new", "command", "plan")
	e.mustRun("sync", "--yes")

	// Skills reach every agent in its native location.
	for _, path := range []string{
		filepath.Join(e.home, ".claude", "skills", "demo", "SKILL.md"),
		filepath.Join(e.home, ".codex", "skills", "demo", "SKILL.md"),
		filepath.Join(e.home, ".gemini", "skills", "demo", "SKILL.md"),
		filepath.Join(e.home, ".gemini", "antigravity-cli", "skills", "demo", "SKILL.md"),
		filepath.Join(e.home, ".cursor", "skills", "demo", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("skill missing at %s", path)
		}
	}

	// Instructions land as managed blocks in each ecosystem's file.
	for _, path := range []string{
		filepath.Join(e.home, ".claude", "CLAUDE.md"),
		filepath.Join(e.home, ".codex", "AGENTS.md"),
		filepath.Join(e.home, ".gemini", "GEMINI.md"),
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("instruction file missing at %s: %v", path, err)
			continue
		}
		if !strings.Contains(string(raw), "<!-- loadout:begin style -->") {
			t.Errorf("managed block missing in %s", path)
		}
	}

	// Commands: native file for claude, rendered as skills elsewhere.
	if _, err := os.Stat(filepath.Join(e.home, ".claude", "commands", "plan.md")); err != nil {
		t.Error("claude command missing")
	}
	for _, agentDir := range []string{".codex", ".gemini", ".cursor"} {
		path := filepath.Join(e.home, agentDir, "skills", "plan", "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("command-as-skill missing at %s", path)
		}
	}

	out := e.mustRun("status")
	if strings.Contains(out, "missing") || strings.Contains(out, "conflict") {
		t.Errorf("status not clean after full sync:\n%s", out)
	}
}

func TestAgents_ListsAllFiveWithDetection(t *testing.T) {
	e := newMultiAgentEnv(t)
	out := e.mustRun("agents")
	for _, id := range []string{"claude-code", "codex", "gemini-cli", "antigravity", "cursor"} {
		if !strings.Contains(out, id) {
			t.Errorf("agents output missing %s:\n%s", id, out)
		}
	}
	if strings.Contains(out, "no") {
		t.Errorf("all fake agents should detect as installed:\n%s", out)
	}
}

func TestAdd_AdoptsNativeSkillIntoStore(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	native := filepath.Join(e.home, ".claude", "skills", "imported")
	if err := os.MkdirAll(native, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(native, "SKILL.md"), []byte("---\nname: imported\ndescription: adopted skill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := e.mustRun("add", native)
	if !strings.Contains(out, `Adopted skill "imported"`) {
		t.Fatalf("add output:\n%s", out)
	}

	// The claude copy is already identical → in-sync for claude-code.
	out = e.mustRun("status", "imported", "--agent", "claude-code")
	if !strings.Contains(out, "in-sync") {
		t.Fatalf("adopted skill should be in-sync on its source agent:\n%s", out)
	}
}

func TestDoctor_HealthyThenDetectsBrokenStoreAndBlocks(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("new", "instruction", "style")
	e.mustRun("sync", "--yes")

	out := e.mustRun("doctor")
	if !strings.Contains(out, "All good.") {
		t.Fatalf("doctor should be clean:\n%s", out)
	}

	// Break a managed block: delete the end marker.
	claudeMD := filepath.Join(e.home, ".claude", "CLAUDE.md")
	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	broken := strings.ReplaceAll(string(raw), "<!-- loadout:end style -->", "")
	if err := os.WriteFile(claudeMD, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	// Break the store too: remove the skill's file.
	storeDir := filepath.Join(e.home, ".local", "share", "loadout", "store")
	if err := os.RemoveAll(filepath.Join(storeDir, "skills", "demo")); err != nil {
		t.Fatal(err)
	}

	out, err = e.run("", "doctor")
	if err == nil {
		t.Fatalf("doctor should exit non-zero on problems:\n%s", out)
	}
	if !strings.Contains(out, "no end marker") {
		t.Errorf("doctor missed the broken block:\n%s", out)
	}
	if !strings.Contains(out, "artifact demo") {
		t.Errorf("doctor missed the missing artifact path:\n%s", out)
	}
}

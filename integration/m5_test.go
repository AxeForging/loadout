//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWizard_PresetGeneratesAndSyncs(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	out := e.mustRun("wizard", "--preset", "go-cli", "--yes")
	if !strings.Contains(out, "+ conventions") || !strings.Contains(out, "+ commit-and-pr") {
		t.Fatalf("wizard output:\n%s", out)
	}

	out = e.mustRun("status")
	for _, id := range []string{"core", "conventions", "commit-and-pr", "linting", "ld-sdd-workflow", "ld-plan", "ld-spec"} {
		if !strings.Contains(out, id) {
			t.Errorf("status missing wizard artifact %q:\n%s", id, out)
		}
	}

	e.mustRun("sync", "--yes")

	// Conventions land in CLAUDE.md as a managed block.
	raw, err := os.ReadFile(e.claude("CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Conventional Commits") {
		t.Errorf("conventions not synced into CLAUDE.md:\n%s", raw)
	}
	// The /ld-plan command exists natively for claude (safe default prefix).
	if _, err := os.Stat(e.claude("commands", "ld-plan.md")); err != nil {
		t.Error("ld-plan command not installed")
	}
	// The commit-and-pr skill ships its lefthook reference.
	if _, err := os.Stat(e.claude("skills", "commit-and-pr", "references", "lefthook.yml")); err != nil {
		t.Error("lefthook reference not installed with the skill")
	}
}

func TestWizard_AnswersFileAndDeclineWritesNothing(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	answers := filepath.Join(e.home, "answers.yaml")
	if err := os.WriteFile(answers, []byte(
		"profile.languages: [python]\nconv.linters: [ruff]\nconv.branching: trunk\ntoolchain.docker: false\nsdd.enable: false\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	// Declined (EOF on stdin) → nothing written.
	if out, err := e.run("", "wizard", "--answers", answers); err != nil {
		t.Fatalf("declined wizard errored: %v\n%s", err, out)
	}
	storeDir := filepath.Join(e.home, ".local", "share", "loadout", "store")
	if _, err := os.Stat(filepath.Join(storeDir, "instructions", "conventions.md")); !os.IsNotExist(err) {
		t.Fatal("wizard wrote without confirmation")
	}

	out := e.mustRun("wizard", "--answers", answers, "--yes")
	if !strings.Contains(out, "Created") {
		t.Fatalf("wizard output:\n%s", out)
	}
	raw, err := os.ReadFile(filepath.Join(storeDir, "instructions", "conventions.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Trunk-based") || !strings.Contains(string(raw), "Ruff") {
		t.Errorf("answers not reflected in conventions:\n%s", raw)
	}
	// docker + sdd disabled → their artifacts must not exist.
	if _, err := os.Stat(filepath.Join(storeDir, "skills", "docker-workflows")); !os.IsNotExist(err) {
		t.Error("docker-workflows generated despite docker=false")
	}
	if _, err := os.Stat(filepath.Join(storeDir, "commands", "ld-plan.md")); !os.IsNotExist(err) {
		t.Error("sdd artifacts generated despite sdd.enable=false")
	}
}

func TestBareLoadout_ShowsOnboardingThenOverview(t *testing.T) {
	e := newEnv(t)

	out := e.mustRun()
	if !strings.Contains(out, "No store yet") || !strings.Contains(out, "loadout init") {
		t.Fatalf("bare loadout should onboard:\n%s", out)
	}

	e.mustRun("init")
	out = e.mustRun()
	if !strings.Contains(out, "The store is empty") {
		t.Fatalf("bare loadout should suggest filling the store:\n%s", out)
	}

	e.mustRun("new", "skill", "demo")
	out = e.mustRun()
	if !strings.Contains(out, "pending change(s)") || !strings.Contains(out, "loadout sync") {
		t.Fatalf("bare loadout should surface pending changes:\n%s", out)
	}
}

func TestWizard_RejectsInvalidAnswers(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	answers := filepath.Join(e.home, "bad.yaml")
	if err := os.WriteFile(answers, []byte("conv.branching: chaos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := e.run("", "wizard", "--answers", answers, "--yes")
	if err == nil {
		t.Fatalf("invalid answers should fail:\n%s", out)
	}
	if !strings.Contains(out, "chaos") {
		t.Errorf("error should name the bad value:\n%s", out)
	}
}

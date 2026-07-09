//go:build integration

package integration

import (
	"os"
	"strings"
	"testing"
)

func TestWizard_CustomCommandNamesSyncUnderTheChosenName(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	answersFile := writeFile(t, e.home, "answers.yaml", `
sdd.specCommandName: fe-spec
sdd.planCommandName: fe-plan
sdd.workflowSkillName: fe-sdd-workflow
continuity.handoffCommandName: fe-handoff
continuity.resumeCommandName: fe-resume
`)
	e.mustRun("wizard", "--answers", answersFile, "--yes")
	e.mustRun("sync", "--yes")

	// The renamed commands land on disk under the chosen name, not the
	// default — this is what a person actually types as /fe-spec.
	for _, name := range []string{"fe-spec.md", "fe-plan.md", "fe-handoff.md", "fe-resume.md"} {
		if _, err := os.Stat(e.claude("commands", name)); err != nil {
			t.Errorf("expected renamed command file commands/%s: %v", name, err)
		}
	}
	for _, name := range []string{"spec.md", "plan.md", "handoff.md", "resume.md"} {
		if _, err := os.Stat(e.claude("commands", name)); err == nil {
			t.Errorf("default command file commands/%s should not exist when renamed", name)
		}
	}

	skill, err := os.ReadFile(e.claude("skills", "fe-sdd-workflow", "SKILL.md"))
	if err != nil {
		t.Fatalf("renamed sdd-workflow skill missing: %v", err)
	}
	if !strings.Contains(string(skill), "/fe-spec") || !strings.Contains(string(skill), "/fe-handoff") {
		t.Errorf("renamed skill missing cross-references:\n%s", skill)
	}
}

func TestWizard_PrefixedCommandsReachAgentHome(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	answersFile := writeFile(t, e.home, "answers.yaml", `
naming.prefix: af
sdd.enable: true
continuity.handoff: true
`)
	e.mustRun("wizard", "--answers", answersFile, "--yes")
	e.mustRun("sync", "--yes")

	// Defaulted command names pick up the namespace prefix, so /af-plan lands
	// on disk instead of the built-in-shadowing /plan.
	for _, name := range []string{"af-plan.md", "af-spec.md", "af-handoff.md", "af-resume.md"} {
		if _, err := os.Stat(e.claude("commands", name)); err != nil {
			t.Errorf("expected prefixed command commands/%s: %v", name, err)
		}
	}
	if _, err := os.Stat(e.claude("commands", "plan.md")); err == nil {
		t.Error("bare commands/plan.md should not exist when a prefix is set")
	}
	if _, err := os.Stat(e.claude("skills", "af-sdd-workflow", "SKILL.md")); err != nil {
		t.Errorf("prefixed sdd-workflow skill missing: %v", err)
	}
}

func TestNew_WarnsButSucceedsOnReservedCommandName(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	out := e.mustRun("new", "command", "plan") // exit 0 — a warning, not a block
	if !strings.Contains(out, "Created command") {
		t.Fatalf("new should still create the command:\n%s", out)
	}
	if !strings.Contains(out, "built-in /plan") {
		t.Errorf("new should warn about the reserved name:\n%s", out)
	}
}

func TestDoctor_NotesReservedNamesWithoutFailing(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	out := e.mustRun("doctor") // must exit 0 despite the reserved name
	if !strings.Contains(out, "plan") || !strings.Contains(out, "built-in /plan") {
		t.Errorf("doctor should note the reserved name:\n%s", out)
	}
	if !strings.Contains(out, "All good.") {
		t.Errorf("doctor should still pass overall:\n%s", out)
	}
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := dir + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

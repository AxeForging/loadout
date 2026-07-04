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

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := dir + "/" + name
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

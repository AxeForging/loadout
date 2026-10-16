package wizard

import (
	"strings"
	"testing"
)

func generatedByID(a Answers) map[string]string {
	out := map[string]string{}
	for _, s := range artifactSpecs(a) {
		for _, body := range s.files {
			out[s.id] += body
		}
	}
	return out
}

func issuesAnswers() Answers {
	return Answers{
		"sdd.enable": true, "sdd.backend": "issues", "sdd.style": "full",
		"continuity.handoff":            true,
		"sdd.specCommandName":           "spec-af",
		"sdd.planCommandName":           "plan-af",
		"sdd.workflowSkillName":         "sdd-workflow-af",
		"continuity.handoffCommandName": "handoff-af",
		"continuity.resumeCommandName":  "resume-af",
	}
}

func TestIssuesBackend_EveryArtifactDescribesTheIssuesOnlyFlow(t *testing.T) {
	got := generatedByID(issuesAnswers())
	want := map[string][]string{
		"spec-af":         {"spec(NNN):", "<!-- plan:start -->", "<!-- plan:end -->", "run /plan-af after the spec is approved", "gh issue create", "--backend issues", "## Rollout"},
		"plan-af":         {"<!-- plan:start -->", "**N. <title>**", "gh issue edit N --body-file", "loadout spec check NNN <N> --backend issues", "Closes #N"},
		"handoff-af":      {"<!-- handoff -->", "gh issue comment N", "/resume-af", ".git/info/exclude"},
		"resume-af":       {"<!-- handoff -->", "/handoff-af", "<!-- plan:start -->"},
		"sdd-workflow-af": {"name: sdd-workflow-af", "/spec-af", "/plan-af", "/handoff-af", "/resume-af", "Re-fetch the body"},
		"sdd":             {"GitHub issues", "`/spec-af`", "`/plan-af`", "Closes #N"},
		"context-handoff": {"`<!-- handoff -->`", "/handoff-af"},
	}
	for id, phrases := range want {
		body, ok := got[id]
		if !ok {
			t.Errorf("artifact %q not generated", id)
			continue
		}
		for _, p := range phrases {
			if !strings.Contains(body, p) {
				t.Errorf("%s: missing %q", id, p)
			}
		}
		// Nothing points at spec/plan files in the repo.
		for _, banned := range []string{".plan.md", "sdd.yaml", "loadout spec init <short-name>\n"} {
			if strings.Contains(body, banned) {
				t.Errorf("%s: mentions repo-file flow %q", id, banned)
			}
		}
	}
}

func TestIssuesBackend_LightStyleUsesShortSections(t *testing.T) {
	a := issuesAnswers()
	a["sdd.style"] = "light"
	spec := generatedByID(a)["spec-af"]
	if !strings.Contains(spec, "## Problem") || strings.Contains(spec, "## Rollout") || !strings.Contains(spec, "--style light") {
		t.Errorf("light issues spec command:\n%s", spec)
	}
}

func TestIssuesBackend_OtherBackendsAndDisabledSDDKeepFileBasedHandoff(t *testing.T) {
	for _, a := range []Answers{
		{"sdd.enable": true, "sdd.backend": "local", "continuity.handoff": true, "naming.prefix": ""},
		{"sdd.enable": true, "sdd.backend": "github", "continuity.handoff": true, "naming.prefix": ""},
		{"sdd.enable": false, "sdd.backend": "issues", "continuity.handoff": true, "naming.prefix": ""},
	} {
		got := generatedByID(a)
		if got["handoff"] != handoffCommand() || got["resume"] != resumeCommand("handoff") {
			t.Errorf("answers %v: handoff/resume must stay file-based", a)
		}
		if a.Bool("sdd.enable") && !strings.Contains(got["plan"], ".plan.md") {
			t.Errorf("answers %v: plan command must keep the file-based plan", a)
		}
	}
}

func TestSchema_OffersIssuesBackend(t *testing.T) {
	for _, q := range Questions() {
		if q.ID != "sdd.backend" {
			continue
		}
		for _, o := range q.Options {
			if o.Value == "issues" {
				return
			}
		}
		t.Fatalf("sdd.backend options = %+v, want issues", q.Options)
	}
	t.Fatal("sdd.backend question not found")
}

package sdd

import (
	"strings"
	"testing"
)

const samplePlanBody = "## Context\r\n" +
	"- [ ] **1. outside the plan is ignored**\r\n" +
	"<!-- plan:start -->\r\n" +
	"## Plan\r\n" +
	"- [ ] **1. Parser** — files: a.go — covers: R1\r\n" +
	"  - [X] **2. Nested** — files: b.go\r\n" +
	"- [ ] Marked step <!-- sdd:wire-cli -->\r\n" +
	"- [ ] unnumbered step, listed but not addressable\r\n" +
	"```\r\n" +
	"- [ ] **7. fenced**\r\n" +
	"```\r\n" +
	"<!-- plan:end -->\r\n" +
	"- [ ] **8. after the plan**\r\n"

func TestParsePlanSteps_ReadsOnlyThePlanSectionWithNumberOrMarkerIDs(t *testing.T) {
	steps, err := ParsePlanSteps([]byte(samplePlanBody))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range steps {
		got = append(got, s.ID+"|"+map[bool]string{true: "x", false: " "}[s.Done]+"|"+s.Text)
	}
	want := []string{
		"1| |**1. Parser** — files: a.go — covers: R1",
		"2|x|**2. Nested** — files: b.go",
		"wire-cli| |Marked step",
		"| |unnumbered step, listed but not addressable",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("steps:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestParsePlanSteps_NoMarkersMeansNoSteps(t *testing.T) {
	steps, err := ParsePlanSteps([]byte("## Plan\n- [ ] **1. not in markers**\n"))
	if err != nil || len(steps) != 0 {
		t.Fatalf("steps = %+v, err = %v; want none without plan markers", steps, err)
	}
}

func TestSetPlanStep_FlipsOneCharacterAndPreservesCRLF(t *testing.T) {
	out, changed, err := SetPlanStep([]byte(samplePlanBody), "1", true)
	if err != nil || !changed {
		t.Fatalf("SetPlanStep() = changed %v, err %v", changed, err)
	}
	want := strings.Replace(samplePlanBody, "- [ ] **1. Parser**", "- [x] **1. Parser**", 1)
	if string(out) != want {
		t.Fatalf("document changed beyond the one checkbox:\n%q", out)
	}
	// Idempotent, and the marked-ID form works too.
	if _, changed, _ := SetPlanStep(out, "1", true); changed {
		t.Error("re-checking a done step reported a change")
	}
	if out2, changed, err := SetPlanStep(out, "wire-cli", true); err != nil || !changed || !strings.Contains(string(out2), "- [x] Marked step") {
		t.Errorf("marker step not toggled: %v %v", changed, err)
	}
}

func TestSetPlanStep_UnknownOrFencedIDListsValidIDs(t *testing.T) {
	for _, id := range []string{"7", "8", "nope"} {
		_, _, err := SetPlanStep([]byte(samplePlanBody), id, true)
		if err == nil || !strings.Contains(err.Error(), "1, 2, wire-cli") {
			t.Errorf("SetPlanStep(%q) error = %v, want valid IDs listed", id, err)
		}
	}
}

func TestParsePlanSteps_DuplicateNumbersAreAnError(t *testing.T) {
	body := "<!-- plan:start -->\n- [ ] **1. a**\n- [ ] **1. b**\n<!-- plan:end -->\n"
	if _, err := ParsePlanSteps([]byte(body)); err == nil {
		t.Fatal("duplicate step numbers must be rejected")
	}
}

func TestResolve_AcceptsIssueReference(t *testing.T) {
	specs := []SpecInfo{{Number: 10, Name: "010-drift", Issue: 23}, {Number: 11, Name: "011-issues", Issue: 25}}
	if s, err := Resolve(specs, "#25"); err != nil || s.Number != 11 {
		t.Errorf("Resolve(#25) = %+v, %v", s, err)
	}
	if _, err := Resolve(specs, "#99"); err == nil {
		t.Error("unknown issue reference accepted")
	}
	if _, err := Resolve(specs, "#x"); err == nil {
		t.Error("malformed issue reference accepted")
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"drift-safe sync":   "drift-safe-sync",
		"  Issues ONLY!! ":  "issues-only",
		"SDD — v2 (issues)": "sdd-v2-issues",
		"!!!":               "",
	} {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadConfig_AcceptsIssuesBackend(t *testing.T) {
	dir := t.TempDir()
	if err := (Config{Backend: BackendIssues}).Save(dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(dir)
	if err != nil || cfg.Backend != BackendIssues {
		t.Fatalf("LoadConfig() = %+v, %v", cfg, err)
	}
}

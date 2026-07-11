package sdd

import (
	"strings"
	"testing"
)

const sampleSpec = "# 001 — demo\n" +
	"\n" +
	"## Checklist\n" +
	"\n" +
	"- [ ] wire the adapter <!-- sdd:wire-adapter -->\n" +
	"  - [x] nested done step <!-- sdd:nested-step -->\n" +
	"* [ ] star bullet works too <!-- sdd:star-step -->\n" +
	"- [ ] no marker so not a step\n" +
	"\n" +
	"```bash\n" +
	"- [ ] looks like a step but is code <!-- sdd:code-step -->\n" +
	"```\n" +
	"\n" +
	"~~~\n" +
	"- [ ] tilde-fenced too <!-- sdd:tilde-step -->\n" +
	"~~~\n" +
	"\n" +
	"tail text\n"

func TestParseSteps_FindsMarkedStepsAtAnyDepthAndSkipsFences(t *testing.T) {
	steps, err := ParseSteps([]byte(sampleSpec))
	if err != nil {
		t.Fatalf("ParseSteps() error = %v", err)
	}
	if len(steps) != 3 {
		t.Fatalf("steps = %+v, want 3 (fenced and unmarked lines excluded)", steps)
	}
	if steps[0].ID != "wire-adapter" || steps[0].Done {
		t.Errorf("step 0 = %+v", steps[0])
	}
	if steps[1].ID != "nested-step" || !steps[1].Done {
		t.Errorf("step 1 = %+v (nested [x] must parse as done)", steps[1])
	}
	if steps[2].ID != "star-step" || steps[2].Text != "star bullet works too" {
		t.Errorf("step 2 = %+v", steps[2])
	}
}

func TestParseSteps_UppercaseXCountsAsDone(t *testing.T) {
	steps, err := ParseSteps([]byte("- [X] shouty done <!-- sdd:shouty -->\n"))
	if err != nil || len(steps) != 1 || !steps[0].Done {
		t.Fatalf("steps = %+v, err = %v", steps, err)
	}
}

func TestParseSteps_DuplicateIDIsAnError(t *testing.T) {
	doc := "- [ ] one <!-- sdd:dup -->\n- [ ] two <!-- sdd:dup -->\n"
	if _, err := ParseSteps([]byte(doc)); err == nil || !strings.Contains(err.Error(), "dup") {
		t.Fatalf("expected duplicate-id error, got %v", err)
	}
}

func TestSetStep_TogglesOnlyTheCheckboxByte(t *testing.T) {
	out, changed, err := SetStep([]byte(sampleSpec), "wire-adapter", true)
	if err != nil || !changed {
		t.Fatalf("SetStep() changed=%v err=%v", changed, err)
	}
	want := strings.Replace(sampleSpec, "- [ ] wire the adapter", "- [x] wire the adapter", 1)
	if string(out) != want {
		t.Errorf("document mutated beyond the checkbox:\n%s", out)
	}
	// Everything else — including the fenced fake steps — is untouched.
	if !strings.Contains(string(out), "- [ ] looks like a step but is code") {
		t.Error("fenced content was modified")
	}
}

func TestSetStep_IsIdempotent(t *testing.T) {
	once, changed, err := SetStep([]byte(sampleSpec), "nested-step", true)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("checking an already-checked step must report changed=false")
	}
	if string(once) != sampleSpec {
		t.Error("no-op SetStep altered the document")
	}
}

func TestSetStep_UncheckWorks(t *testing.T) {
	out, changed, err := SetStep([]byte(sampleSpec), "nested-step", false)
	if err != nil || !changed {
		t.Fatalf("uncheck: changed=%v err=%v", changed, err)
	}
	if !strings.Contains(string(out), "- [ ] nested done step") {
		t.Errorf("uncheck failed:\n%s", out)
	}
}

func TestSetStep_UnknownIDListsValidSteps(t *testing.T) {
	_, _, err := SetStep([]byte(sampleSpec), "nope", true)
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	for _, want := range []string{"wire-adapter", "nested-step", "star-step"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list valid id %s, got: %v", want, err)
		}
	}
}

func TestSetStep_PreservesCRLFDocuments(t *testing.T) {
	doc := "# spec\r\n\r\n- [ ] windows step <!-- sdd:win-step -->\r\n\r\ntail\r\n"
	out, changed, err := SetStep([]byte(doc), "win-step", true)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	want := strings.Replace(doc, "- [ ]", "- [x]", 1)
	if string(out) != want {
		t.Errorf("CRLF document corrupted:\ngot:  %q\nwant: %q", out, want)
	}
}

func TestSetStep_NoTrailingNewlineDocumentSurvives(t *testing.T) {
	doc := "- [ ] last line no newline <!-- sdd:last-step -->"
	out, changed, err := SetStep([]byte(doc), "last-step", true)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if string(out) != "- [x] last line no newline <!-- sdd:last-step -->" {
		t.Errorf("got %q", out)
	}
}

package wizard

import "testing"

func TestCustomName_UsesGivenValueWhenValid(t *testing.T) {
	a := Answers{"sdd.workflowSkillName": "fe-sdd-workflow"}
	if got := customName(a, "sdd.workflowSkillName", "sdd-workflow"); got != "fe-sdd-workflow" {
		t.Errorf("customName() = %q, want %q", got, "fe-sdd-workflow")
	}
}

func TestCustomName_FallsBackOnAbsentOrEmpty(t *testing.T) {
	if got := customName(Answers{}, "sdd.workflowSkillName", "sdd-workflow"); got != "sdd-workflow" {
		t.Errorf("absent answer: customName() = %q, want default", got)
	}
	a := Answers{"sdd.workflowSkillName": "  "}
	if got := customName(a, "sdd.workflowSkillName", "sdd-workflow"); got != "sdd-workflow" {
		t.Errorf("blank answer: customName() = %q, want default", got)
	}
}

func TestCustomName_FallsBackOnInvalidIdentifier(t *testing.T) {
	// Spaces, uppercase and other characters outside store.ValidateName's
	// lowercase/digit/dash rule must never reach a generated file path or
	// slash command — silently falling back is safer than a broken sync.
	for _, bad := range []string{"Has Spaces", "UPPER", "../escape", "trailing-"} {
		if got := customName(Answers{"x": bad}, "x", "fallback"); got != "fallback" {
			t.Errorf("customName(%q) = %q, want fallback", bad, got)
		}
	}
}

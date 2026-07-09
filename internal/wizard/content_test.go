package wizard

import (
	"strings"
	"testing"
)

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

func TestChangeScopeGuidance_EachModeHasDistinctText(t *testing.T) {
	minimal := changeScopeGuidance("minimal")
	balanced := changeScopeGuidance("balanced")
	thorough := changeScopeGuidance("thorough")

	if minimal == balanced || balanced == thorough || minimal == thorough {
		t.Fatal("each change-scope mode must produce distinct guidance text")
	}
	if !strings.Contains(minimal, "fewest files") {
		t.Errorf("minimal mode should stress fewest files: %q", minimal)
	}
	if !strings.Contains(minimal, "reuse") && !strings.Contains(minimal, "duplicat") {
		t.Errorf("minimal mode should warn against duplication: %q", minimal)
	}
	if !strings.Contains(thorough, "clearly improve") {
		t.Errorf("thorough mode should allow broader changes when they help: %q", thorough)
	}
}

func TestChangeScopeGuidance_UnknownModeFallsBackToBalanced(t *testing.T) {
	if got := changeScopeGuidance(""); got != changeScopeGuidance("balanced") {
		t.Errorf("empty mode should fall back to balanced guidance, got %q", got)
	}
	if got := changeScopeGuidance("bogus"); got != changeScopeGuidance("balanced") {
		t.Errorf("unknown mode should fall back to balanced guidance, got %q", got)
	}
}

func TestCleanItem_StripsOneLeadingBulletMarker(t *testing.T) {
	cases := map[string]string{
		"- always test":        "always test",
		"* always test":        "always test",
		"• always test":        "always test",
		"  - always test  ":    "always test",
		"always test":          "always test",
		"- - always test":      "- always test",
		"":                     "",
		"   ":                  "",
		"-":                    "",
		"not-a-bullet, really": "not-a-bullet, really",
	}
	for in, want := range cases {
		if got := cleanItem(in); got != want {
			t.Errorf("cleanItem(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCustomInstruction_SanitizesUserBullets(t *testing.T) {
	a := Answers{"standards.custom": []string{"- Always ensure tests that are real", "edge cases", "  "}}
	got := customInstruction(a)
	want := "# Custom instructions\n\nTeam-specific rules — follow these over general defaults when they conflict.\n\n- Always ensure tests that are real\n- edge cases\n"
	if got != want {
		t.Errorf("customInstruction() =\n%q\nwant\n%q", got, want)
	}
}

func TestCustomInstruction_EmptyReturnsEmpty(t *testing.T) {
	if got := customInstruction(Answers{}); got != "" {
		t.Errorf("customInstruction() with no answers = %q, want empty", got)
	}
	if got := customInstruction(Answers{"standards.custom": []string{"  ", "-"}}); got != "" {
		t.Errorf("customInstruction() with only blank/bullet-only entries = %q, want empty", got)
	}
}

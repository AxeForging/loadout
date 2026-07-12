package wizard

import "testing"

// The web builder previews resolved install names from the Namespaced flag,
// so the flag must stay in lockstep with the names generate.go actually
// prefixes — flag drift would make the UI preview lie.
func TestSections_NamespacedFlagMatchesPrefixedNames(t *testing.T) {
	want := map[string]bool{
		"sdd.specCommandName":           true,
		"sdd.planCommandName":           true,
		"sdd.workflowSkillName":         true,
		"continuity.handoffCommandName": true,
		"continuity.resumeCommandName":  true,
	}
	seen := map[string]bool{}
	for _, q := range Questions() {
		if q.Namespaced {
			if !want[q.ID] {
				t.Errorf("question %q is flagged Namespaced but generate.go does not prefix it", q.ID)
			}
			if q.Type != TypeText {
				t.Errorf("namespaced question %q must be free text, got %q", q.ID, q.Type)
			}
			if _, ok := q.Default.(string); !ok {
				t.Errorf("namespaced question %q needs a string default for the prefix rule", q.ID)
			}
			seen[q.ID] = true
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("question %q is prefixed by generate.go but not flagged Namespaced", id)
		}
	}
}

func TestSections_NamespacePrefixQuestionExists(t *testing.T) {
	for _, q := range Questions() {
		if q.ID == NamespacePrefixQuestionID {
			if q.Type != TypeText {
				t.Fatalf("prefix question must be free text, got %q", q.Type)
			}
			return
		}
	}
	t.Fatalf("no question with ID %q in the schema", NamespacePrefixQuestionID)
}

// Prefixing behavior itself (default gets prefix, rename wins verbatim) —
// asserted through the public artifact plan so the schema flag, the answer
// plumbing and the generator stay one story.
func TestArtifactSpecs_PrefixAppliesToDefaultsOnly(t *testing.T) {
	a := Answers{
		NamespacePrefixQuestionID: "af",
		"sdd.enable":              true,
		"continuity.handoff":      true,
		"sdd.planCommandName":     "blueprint", // explicit rename: no prefix
	}
	ids := map[string]bool{}
	for _, spec := range artifactSpecs(a) {
		ids[spec.id] = true
	}
	for _, want := range []string{"af-spec", "blueprint", "af-sdd-workflow", "af-handoff", "af-resume"} {
		if !ids[want] {
			t.Errorf("expected artifact %q in plan, got %v", want, ids)
		}
	}
	if ids["af-plan"] || ids["plan"] {
		t.Errorf("renamed plan command must not also appear as default or prefixed default: %v", ids)
	}
}

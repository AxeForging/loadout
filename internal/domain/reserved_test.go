package domain

import "testing"

func TestReservedNameWarning_FlagsClaudeCodeBuiltins(t *testing.T) {
	for _, name := range []string{"plan", "review", "init", "security-review", "resume"} {
		if got := ReservedNameWarning(KindCommand, name); got == "" {
			t.Errorf("command %q should warn, got no warning", name)
		}
		// Skills are slash-invocable too, so they collide as well.
		if got := ReservedNameWarning(KindSkill, name); got == "" {
			t.Errorf("skill %q should warn, got no warning", name)
		}
	}
}

func TestReservedCommandNames_SortedAndConsistentWithWarnings(t *testing.T) {
	names := ReservedCommandNames()
	if len(names) == 0 {
		t.Fatal("expected a non-empty reserved list")
	}
	for i, n := range names {
		if i > 0 && names[i-1] >= n {
			t.Errorf("list not sorted: %q before %q", names[i-1], n)
		}
		if ReservedNameWarning(KindCommand, n) == "" {
			t.Errorf("listed name %q should also warn via ReservedNameWarning", n)
		}
	}
}

func TestReservedNameWarning_AllowsPrefixedAndUnknownNames(t *testing.T) {
	for _, name := range []string{"my-plan", "af-review", "handoff", "commit-and-pr", "linting"} {
		if got := ReservedNameWarning(KindCommand, name); got != "" {
			t.Errorf("name %q should be safe, got warning %q", name, got)
		}
	}
	// Kinds that never become slash commands are always safe, even for a
	// reserved word.
	if got := ReservedNameWarning(KindInstruction, "plan"); got != "" {
		t.Errorf("instruction %q should not warn, got %q", "plan", got)
	}
}

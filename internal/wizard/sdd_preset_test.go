package wizard

import (
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

// The sdd preset must generate artifacts whose bodies drive the `loadout
// spec` CLI — this guards the preset (and generators) against drifting from
// the command surface they instruct agents to call.
func TestSDDPreset_GeneratesArtifactsWiredToTheSpecCLI(t *testing.T) {
	answers, err := LoadPreset("sdd")
	if err != nil {
		t.Fatalf("LoadPreset(sdd) error = %v", err)
	}
	if !answers.Bool("sdd.enable") {
		t.Fatal("sdd preset must enable sdd")
	}

	byID := map[string]artifactSpec{}
	for _, s := range artifactSpecs(answers) {
		byID[s.id] = s
	}

	workflow, ok := byID["sdd-workflow"]
	if !ok || workflow.kind != domain.KindSkill {
		t.Fatalf("sdd preset must generate the sdd-workflow skill; got %v", byID)
	}
	skillBody := workflow.files["SKILL.md"]
	for _, want := range []string{"loadout spec check", "loadout spec status", "<!-- sdd:"} {
		if !strings.Contains(skillBody, want) {
			t.Errorf("sdd-workflow skill missing %q — the completion loop must go through the CLI", want)
		}
	}

	spec, ok := byID["spec"]
	if !ok || spec.kind != domain.KindCommand {
		t.Fatalf("sdd preset must generate the spec command")
	}
	if !strings.Contains(spec.files[""], "loadout spec init") {
		t.Error("spec command must scaffold via 'loadout spec init' (numbering + issue creation live there)")
	}

	plan, ok := byID["plan"]
	if !ok || plan.kind != domain.KindCommand {
		t.Fatalf("sdd preset must generate the plan command")
	}
	for _, want := range []string{"loadout spec check", "<!-- sdd:"} {
		if !strings.Contains(plan.files[""], want) {
			t.Errorf("plan command missing %q", want)
		}
	}
}

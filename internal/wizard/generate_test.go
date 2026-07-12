package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Init(filepath.Join(t.TempDir(), "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// specIDs returns the set of artifact IDs artifactSpecs emits for answers a.
func specIDs(a Answers) map[string]bool {
	ids := map[string]bool{}
	for _, s := range artifactSpecs(a) {
		ids[s.id] = true
	}
	return ids
}

func TestArtifactSpecs_PrefixesDefaultCommandAndSkillNames(t *testing.T) {
	a := Answers{"sdd.enable": true, "continuity.handoff": true, "naming.prefix": "af"}
	ids := specIDs(a)
	for _, want := range []string{"af-plan", "af-spec", "af-handoff", "af-resume", "af-sdd-workflow"} {
		if !ids[want] {
			t.Errorf("missing prefixed artifact %q (got %v)", want, ids)
		}
	}
	for _, unwanted := range []string{"plan", "spec", "handoff", "resume"} {
		if ids[unwanted] {
			t.Errorf("bare %q should have been prefixed", unwanted)
		}
	}
}

func TestArtifactSpecs_ExplicitRenameIgnoresPrefix(t *testing.T) {
	a := Answers{"sdd.enable": true, "naming.prefix": "af", "sdd.planCommandName": "roadmap"}
	ids := specIDs(a)
	if !ids["roadmap"] {
		t.Errorf("explicit rename should win verbatim, got %v", ids)
	}
	if ids["af-roadmap"] || ids["af-plan"] {
		t.Errorf("prefix must not apply to an explicitly renamed command, got %v", ids)
	}
}

func TestArtifactSpecs_InvalidPrefixIsIgnored(t *testing.T) {
	a := Answers{"sdd.enable": true, "naming.prefix": "Not Valid!"}
	ids := specIDs(a)
	if !ids["plan"] || ids["Not Valid!-plan"] {
		t.Errorf("invalid prefix should be ignored, leaving bare names, got %v", ids)
	}
}

func TestNameWarnings_EmptyByDefaultThanksToLdPrefix(t *testing.T) {
	// Safe out of the box: an untouched questionnaire generates /ld-plan etc.,
	// which shadows nothing — warnings appear only once the prefix is cleared.
	if warnings := NameWarnings(Answers{"sdd.enable": true}); len(warnings) != 0 {
		t.Fatalf("default (ld-prefixed) names should not warn, got %v", warnings)
	}
}

func TestNameWarnings_FlagsBarePlanCommandWhenPrefixCleared(t *testing.T) {
	a := Answers{"sdd.enable": true, "naming.prefix": ""}
	warnings := NameWarnings(a)
	if len(warnings) == 0 {
		t.Fatal("bare /plan command should produce a reserved-name warning")
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "plan") {
		t.Errorf("warnings should mention plan: %v", warnings)
	}
}

func TestNameWarnings_EmptyWhenPrefixed(t *testing.T) {
	a := Answers{"sdd.enable": true, "continuity.handoff": true, "naming.prefix": "af"}
	if warnings := NameWarnings(a); len(warnings) != 0 {
		t.Errorf("prefixed names should not warn, got %v", warnings)
	}
}

func TestPresets_StillValidateWithNamingSection(t *testing.T) {
	for _, name := range []string{"go-cli", "web-app"} {
		a, err := LoadPreset(name)
		if err != nil {
			t.Fatalf("LoadPreset(%q) error = %v", name, err)
		}
		if _, err := a.validate(); err != nil {
			t.Errorf("preset %q no longer validates after adding naming section: %v", name, err)
		}
	}
}

func TestGenerate_GoCliPresetProducesConventionArtifacts(t *testing.T) {
	a, err := LoadPreset("go-cli")
	if err != nil {
		t.Fatalf("LoadPreset() error = %v", err)
	}
	s := testStore(t)
	g, err := Generate(s, a)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	wantIDs := []string{"core", "toolchain", "conventions", "commit-and-pr", "linting", "docker-workflows", "sdd", "ld-plan", "ld-spec", "ld-sdd-workflow", "testing", "ld-handoff", "ld-resume", "context-handoff"}
	got := map[string]domain.Artifact{}
	for _, art := range g.Created {
		got[art.ID] = art
	}
	for _, id := range wantIDs {
		if _, ok := got[id]; !ok {
			t.Errorf("missing generated artifact %q", id)
		}
	}

	// Conventions instruction reflects the chosen answers.
	raw, err := os.ReadFile(filepath.Join(s.Root, "instructions", "conventions.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, want := range []string{"Conventional Commits", "GitHub flow", "golangci-lint", "lefthook"} {
		if !strings.Contains(text, want) {
			t.Errorf("conventions.md missing %q:\n%s", want, text)
		}
	}

	// The commit-and-pr skill ships the lefthook snippet with a commit-msg gate.
	snippet, err := os.ReadFile(filepath.Join(s.Root, "skills", "commit-and-pr", "references", "lefthook.yml"))
	if err != nil {
		t.Fatalf("lefthook snippet missing: %v", err)
	}
	if !strings.Contains(string(snippet), "commit-msg:") || !strings.Contains(string(snippet), "feat|fix") {
		t.Errorf("lefthook snippet lacks commit-msg gate:\n%s", snippet)
	}
	if !strings.Contains(string(snippet), "gofumpt") {
		t.Errorf("lefthook snippet should wire gofumpt for the fmt hook:\n%s", snippet)
	}

	// Testing strategy reflects the go-cli preset (Go stdlib, no FE noise).
	testing_, err := os.ReadFile(filepath.Join(s.Root, "instructions", "testing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(testing_), "go test -race") {
		t.Errorf("testing.md missing Go guidance:\n%s", testing_)
	}
	if strings.Contains(string(testing_), "Component tests") {
		t.Errorf("go-cli preset (feFramework none) should not emit component-test guidance:\n%s", testing_)
	}

	// Context handoff artifacts use the cross-agent location.
	handoff, err := os.ReadFile(filepath.Join(s.Root, "commands", "ld-handoff.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(handoff), ".agents/context/handoff.md") {
		t.Errorf("handoff command missing canonical path:\n%s", handoff)
	}
}

func TestGenerate_TogglesRespectAnswers(t *testing.T) {
	a := Answers{
		"toolchain.docker": false,
		"sdd.enable":       false,
		"conv.linters":     []string{},
		"conv.commits":     "freeform",
		"conv.prTitles":    "freeform",
	}
	s := testStore(t)
	g, err := Generate(s, a)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	for _, art := range g.Created {
		switch art.ID {
		case "docker-workflows", "sdd", "plan", "spec", "sdd-workflow", "linting", "commit-and-pr":
			t.Errorf("artifact %q should not be generated for these answers", art.ID)
		}
	}
}

func TestGenerate_SkipsExistingArtifactsInsteadOfOverwriting(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindInstruction, "core"); err != nil {
		t.Fatal(err)
	}
	userContent, _ := os.ReadFile(filepath.Join(s.Root, "instructions", "core.md"))

	a, _ := LoadPreset("go-cli")
	g, err := Generate(s, a)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if !contains(g.Skipped, "core") {
		t.Errorf("core should be skipped, got skipped=%v", g.Skipped)
	}
	after, _ := os.ReadFile(filepath.Join(s.Root, "instructions", "core.md"))
	if string(after) != string(userContent) {
		t.Error("existing artifact was overwritten")
	}
}

func TestGenerate_ScopedTargetsLandOnArtifacts(t *testing.T) {
	a := Answers{"targets.agents": []string{"claude-code", "codex"}}
	s := testStore(t)
	g, err := Generate(s, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, art := range g.Created {
		if len(art.Targets) != 2 {
			t.Errorf("artifact %s targets = %v, want claude-code+codex", art.ID, art.Targets)
		}
	}
}

func TestAnswers_ValidateRejectsUnknownIDsAndBadValues(t *testing.T) {
	if _, err := (Answers{"nope.nope": true}).validate(); err == nil {
		t.Error("unknown ID should fail validation")
	}
	if _, err := (Answers{"conv.branching": "chaos"}).validate(); err == nil {
		t.Error("invalid select value should fail validation")
	}
	if _, err := (Answers{"conv.linters": []string{"clippy"}}).validate(); err == nil {
		t.Error("invalid multi value should fail validation")
	}
	if _, err := (Answers{"sdd.enable": "yes"}).validate(); err == nil {
		t.Error("non-bool for bool question should fail validation")
	}
}

func TestLoadPreset_AllEmbeddedPresetsAreValid(t *testing.T) {
	names := Presets()
	if len(names) < 2 {
		t.Fatalf("expected embedded presets, got %v", names)
	}
	for _, name := range names {
		if _, err := LoadPreset(name); err != nil {
			t.Errorf("preset %s invalid: %v", name, err)
		}
	}
}

func TestGenerate_OtherFreeTextFlowsIntoGeneratedContent(t *testing.T) {
	a := Answers{
		"profile.languages":               []string{"go", "other"},
		"profile.languages.other":         "Java",
		"toolchain.packageManagers":       []string{"go-modules", "other"},
		"toolchain.packageManagers.other": "Maven",
		"conv.linters":                    []string{"golangci-lint", "other"},
		"conv.linters.other":              "Checkstyle",
		"quality.unitTools":               []string{"go-testing", "other"},
		"quality.unitTools.other":         "JUnit",
	}
	s := testStore(t)
	if _, err := Generate(s, a); err != nil {
		t.Fatal(err)
	}

	core, err := os.ReadFile(filepath.Join(s.Root, "instructions", "core.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(core), "Java") || strings.Contains(string(core), "Other") {
		t.Errorf("core.md should list Java, not the generic Other placeholder:\n%s", core)
	}

	toolchain, err := os.ReadFile(filepath.Join(s.Root, "instructions", "toolchain.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toolchain), "Maven") {
		t.Errorf("toolchain.md missing custom package manager Maven:\n%s", toolchain)
	}

	linting, err := os.ReadFile(filepath.Join(s.Root, "skills", "linting", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(linting), "Checkstyle") {
		t.Errorf("linting skill missing custom linter Checkstyle:\n%s", linting)
	}

	testingDoc, err := os.ReadFile(filepath.Join(s.Root, "instructions", "testing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(testingDoc), "JUnit") {
		t.Errorf("testing.md missing custom test runner JUnit:\n%s", testingDoc)
	}
}

func TestGenerate_CustomCommandNamesPropagateToEveryCrossReference(t *testing.T) {
	a := Answers{
		"sdd.specCommandName":           "fe-spec",
		"sdd.planCommandName":           "fe-plan",
		"sdd.workflowSkillName":         "fe-sdd-workflow",
		"continuity.handoffCommandName": "fe-handoff",
		"continuity.resumeCommandName":  "fe-resume",
	}
	s := testStore(t)
	g, err := Generate(s, a)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, art := range g.Created {
		got[art.ID] = true
	}
	for _, id := range []string{"fe-spec", "fe-plan", "fe-sdd-workflow", "fe-handoff", "fe-resume"} {
		if !got[id] {
			t.Errorf("expected an artifact named %q, got IDs: %v", id, got)
		}
	}
	// The old hardcoded IDs must not exist alongside the renamed ones.
	for _, id := range []string{"spec", "plan", "sdd-workflow", "handoff", "resume"} {
		if got[id] {
			t.Errorf("unexpected artifact with the default ID %q when renamed", id)
		}
	}

	workflow, ok := s.Artifact("fe-sdd-workflow")
	if !ok {
		t.Fatal("fe-sdd-workflow artifact missing")
	}
	content, err := s.Content(workflow)
	if err != nil {
		t.Fatal(err)
	}
	skill := string(content.Files["SKILL.md"])
	for _, want := range []string{"name: fe-sdd-workflow", "/fe-spec", "/fe-plan", "/fe-handoff", "/fe-resume"} {
		if !strings.Contains(skill, want) {
			t.Errorf("sdd-workflow skill missing renamed reference %q:\n%s", want, skill)
		}
	}
	for _, oldRef := range []string{"/spec ", "/plan:", "/resume ", "/handoff "} {
		if strings.Contains(skill, oldRef) {
			t.Errorf("sdd-workflow skill still references the default name %q:\n%s", oldRef, skill)
		}
	}

	sddDoc, err := os.ReadFile(filepath.Join(s.Root, "instructions", "sdd.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sddDoc), "/fe-spec") || !strings.Contains(string(sddDoc), "/fe-plan") {
		t.Errorf("sdd.md missing renamed command references:\n%s", sddDoc)
	}

	resumeDoc, err := os.ReadFile(filepath.Join(s.Root, "commands", "fe-resume.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(resumeDoc), "/fe-handoff") {
		t.Errorf("resume command should reference the renamed handoff command:\n%s", resumeDoc)
	}

	handoffSkill, ok := s.Artifact("context-handoff")
	if !ok {
		t.Fatal("context-handoff artifact missing")
	}
	handoffSkillContent, err := s.Content(handoffSkill)
	if err != nil {
		t.Fatal(err)
	}
	skillDoc := string(handoffSkillContent.Files["SKILL.md"])
	if !strings.Contains(skillDoc, "/fe-handoff") || !strings.Contains(skillDoc, "/fe-resume") {
		t.Errorf("context-handoff skill missing renamed references:\n%s", skillDoc)
	}
}

func TestGenerate_DefaultNamesCarryTheSafePrefix(t *testing.T) {
	a, _ := LoadPreset("go-cli")
	s := testStore(t)
	g, err := Generate(s, a)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, art := range g.Created {
		got[art.ID] = true
	}
	for _, id := range []string{"ld-spec", "ld-plan", "ld-sdd-workflow", "ld-handoff", "ld-resume"} {
		if !got[id] {
			t.Errorf("default artifact ID %q missing when no custom name given", id)
		}
	}
	for _, bare := range []string{"spec", "plan"} {
		if got[bare] {
			t.Errorf("bare %q generated — defaults must carry the ld prefix", bare)
		}
	}
}

func TestGenerate_ChangeScopeReachesTheWorkflowSkill(t *testing.T) {
	s := testStore(t)
	if _, err := Generate(s, Answers{"sdd.changeScope": "minimal"}); err != nil {
		t.Fatal(err)
	}
	skill, err := os.ReadFile(filepath.Join(s.Root, "skills", "ld-sdd-workflow", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill), "fewest files") {
		t.Errorf("sdd-workflow skill missing minimal-diff guidance:\n%s", skill)
	}

	s2 := testStore(t)
	if _, err := Generate(s2, Answers{"sdd.changeScope": "thorough"}); err != nil {
		t.Fatal(err)
	}
	skill2, err := os.ReadFile(filepath.Join(s2.Root, "skills", "ld-sdd-workflow", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(skill2), "clearly improve") {
		t.Errorf("sdd-workflow skill missing thorough-mode guidance:\n%s", skill2)
	}
	if strings.Contains(string(skill2), "fewest files") {
		t.Errorf("thorough mode should not carry minimal-diff wording:\n%s", skill2)
	}
}

func TestPlan_CustomInstructionsOnlyWhenAnswered(t *testing.T) {
	s := testStore(t)
	create, _ := Plan(s, Answers{})
	if contains(create, "custom-instructions") {
		t.Errorf("custom-instructions should not be planned without answers: %v", create)
	}

	s2 := testStore(t)
	create2, _ := Plan(s2, Answers{"standards.custom": []string{"always test"}})
	if !contains(create2, "custom-instructions") {
		t.Errorf("custom-instructions should be planned when standards.custom is set: %v", create2)
	}

	g, err := Generate(s2, Answers{"standards.custom": []string{"always test", "no coverage theater"}})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(idsOf(g.Created), "custom-instructions") {
		t.Fatalf("custom-instructions artifact not created, got: %v", g.Created)
	}
	raw, err := os.ReadFile(filepath.Join(s2.Root, "instructions", "custom-instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "- always test\n") || !strings.Contains(text, "- no coverage theater\n") {
		t.Errorf("custom-instructions.md missing clean bullets:\n%s", text)
	}
}

func idsOf(arts []domain.Artifact) []string {
	out := make([]string, 0, len(arts))
	for _, a := range arts {
		out = append(out, a.ID)
	}
	return out
}

func TestGenerate_CommandsRenderAsValidSingleFileArtifacts(t *testing.T) {
	a, _ := LoadPreset("web-app")
	s := testStore(t)
	if _, err := Generate(s, a); err != nil {
		t.Fatal(err)
	}
	art, ok := s.Artifact("ld-spec")
	if !ok {
		t.Fatal("ld-spec command missing")
	}
	content, err := s.Content(art)
	if err != nil || len(content.Files) != 1 {
		t.Fatalf("spec content = %v, err %v", content, err)
	}
	for _, raw := range content.Files {
		if !strings.Contains(string(raw), "## Design") {
			t.Errorf("web-app preset (sdd.style=full) should produce the full spec template:\n%s", raw)
		}
	}
}

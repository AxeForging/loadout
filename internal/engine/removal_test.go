package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

func TestPlanRemoval_MergeReplaceFile_QueuesDelete(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindCommand, "plan"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	installed, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	rp, err := env.eng.PlanRemoval(context.Background(), installed)
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 1 {
		t.Fatalf("Items = %v, want exactly one delete", rp.Items)
	}
	it := rp.Items[0]
	if it.Op != OpDelete || it.IsDir {
		t.Errorf("item = %+v, want a single-file OpDelete", it)
	}
	if it.AbsPath != filepath.Join(env.home, "commands", "plan.md") {
		t.Errorf("AbsPath = %s", it.AbsPath)
	}
}

func TestPlanRemoval_SkillArtifact_CollapsesToSingleDirectoryDelete(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	// Add a second file to the skill so its render produces >1 target file.
	if err := os.MkdirAll(filepath.Join(env.store.Root, "skills", "review", "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.store.Root, "skills", "review", "references", "checklist.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	installed, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	rp, err := env.eng.PlanRemoval(context.Background(), installed)
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 1 {
		t.Fatalf("Items = %v, want exactly one collapsed directory delete despite 2 rendered files", rp.Items)
	}
	it := rp.Items[0]
	if it.Op != OpDelete || !it.IsDir {
		t.Errorf("item = %+v, want a directory OpDelete", it)
	}
	if it.AbsPath != filepath.Join(env.home, "skills", "review") {
		t.Errorf("AbsPath = %s", it.AbsPath)
	}
}

func TestPlanRemoval_MergeBlock_StripsBlockKeepsRestOfFile(t *testing.T) {
	env := newTestEnv(t)
	userContent := "# my personal claude config\n\nkeep me\n"
	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	if err := os.WriteFile(claudeMD, []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.New(domain.KindInstruction, "style"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	installed, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	rp, err := env.eng.PlanRemoval(context.Background(), installed)
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 1 || rp.Items[0].Op != OpUpdate {
		t.Fatalf("Items = %v, want one block-strip OpUpdate", rp.Items)
	}
	desired := string(rp.Items[0].Desired)
	if strings.Contains(desired, "loadout:begin style") {
		t.Errorf("block not stripped from Desired:\n%s", desired)
	}
	if !strings.Contains(desired, "keep me") {
		t.Errorf("user content lost from Desired:\n%s", desired)
	}
}

func TestPlanRemoval_MergeBlock_AlreadyAbsent_NoopButClearsState(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindInstruction, "style"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	// Simulate the block already having been removed by hand.
	if err := os.WriteFile(filepath.Join(env.home, "CLAUDE.md"), []byte("nothing managed here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"style"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %v, want none (block already absent)", rp.Items)
	}
	if len(rp.ClearState) != 1 || rp.ClearState[0].Artifact != "style" {
		t.Errorf("ClearState = %v, want [style]", rp.ClearState)
	}
}

func TestPlanRemoval_MergeBlock_BrokenMarkers_WarnsDoesNotTouchFile(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindInstruction, "style"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	broken := "<!-- loadout:begin style -->\nno end marker\n"
	if err := os.WriteFile(claudeMD, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"style"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %v, want none — broken block must not be touched", rp.Items)
	}
	if len(rp.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one", rp.Warnings)
	}
	raw, err := os.ReadFile(claudeMD)
	if err != nil || string(raw) != broken {
		t.Errorf("file was modified despite structural damage: %q", raw)
	}
}

func TestPlanRemoval_MergeJSON_WarnsLeavesFileClearsState(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindHook, "lint"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	settingsPath := filepath.Join(env.home, "settings.json")
	before, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"lint"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}

	// One warning for the JSON fragment; the hook script (MergeReplace) is a
	// real delete Item.
	var jsonWarned bool
	for _, w := range rp.Warnings {
		if w.RelPath == "settings.json" {
			jsonWarned = true
		}
	}
	if !jsonWarned {
		t.Errorf("Warnings = %v, want a warning for settings.json", rp.Warnings)
	}
	for _, it := range rp.Items {
		if it.RelPath == "settings.json" {
			t.Errorf("settings.json should never be a removal Item, got %+v", it)
		}
	}
	if len(rp.ClearState) != 1 || rp.ClearState[0].Artifact != "lint" {
		t.Errorf("ClearState = %v, want [lint]", rp.ClearState)
	}
	after, err := os.ReadFile(settingsPath)
	if err != nil || string(after) != string(before) {
		t.Error("settings.json must be left byte-identical")
	}
}

func TestPlanRemoval_UnknownArtifactInManifest_WarnsDoesNotCrash(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	// Remove the artifact from the manifest, as a hand-edit would.
	var kept []domain.Artifact
	for _, a := range env.store.Manifest.Artifacts {
		if a.ID != "review" {
			kept = append(kept, a)
		}
	}
	env.store.Manifest.Artifacts = kept

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"review"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v (must not error/crash)", err)
	}
	if len(rp.Warnings) == 0 {
		t.Error("expected a warning for the artifact no longer in the manifest")
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %v, want none — must not guess", rp.Items)
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "review", "SKILL.md")); err != nil {
		t.Error("file should still be on disk — PlanRemoval performs no writes")
	}
}

func TestPlanRemoval_AgentNotDetected_SkipsButClearsState(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "claude-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(claudecode.NewWithHome(home))
	eng := &Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")}

	if _, err := s.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	plan, err := eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}

	// Simulate the agent home vanishing entirely before the switch.
	if err := os.RemoveAll(home); err != nil {
		t.Fatal(err)
	}

	rp, err := eng.PlanRemoval(context.Background(), []string{"review"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Skips) != 1 {
		t.Fatalf("Skips = %v, want one", rp.Skips)
	}
	if len(rp.ClearState) != 1 || rp.ClearState[0].Artifact != "review" {
		t.Errorf("ClearState = %v, want [review] even though the agent is unreachable", rp.ClearState)
	}
}

// fakeOwnerAdapter wraps claudecode's adapter but claims to own nothing,
// proving PlanRemoval's Owned() allowlist actually blocks a delete.
type fakeOwnerAdapter struct{ *claudecode.Adapter }

func (fakeOwnerAdapter) Owned(home string) ([]string, error) { return nil, nil }

func TestPlanRemoval_RejectsDeleteOutsideOwnedPrefix(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "claude-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(fakeOwnerAdapter{claudecode.NewWithHome(home)})
	eng := &Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")}

	if _, err := s.New(domain.KindCommand, "plan"); err != nil {
		t.Fatal(err)
	}
	plan, err := eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}

	rp, err := eng.PlanRemoval(context.Background(), []string{"plan"})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %v, want none — Owned() allowlist should have refused the delete", rp.Items)
	}
	if len(rp.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly one refusal warning", rp.Warnings)
	}
}

func TestApplySwitch_WithoutConfirmationWritesNothing(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "keep")
	env.addSkill(t, "gone")
	env.applyAll(t)

	installPlan, err := env.eng.Plan(context.Background(), Filter{Artifacts: []string{"keep"}})
	if err != nil {
		t.Fatal(err)
	}
	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := env.eng.ApplySwitch(context.Background(), installPlan, removalPlan, nil); err != ErrNotConfirmed {
		t.Fatalf("ApplySwitch(nil confirm) error = %v, want ErrNotConfirmed", err)
	}
	if _, _, err := env.eng.ApplySwitch(context.Background(), installPlan, removalPlan, func(*Plan, *RemovalPlan) bool { return false }); err != ErrNotConfirmed {
		t.Fatalf("ApplySwitch(decline) error = %v, want ErrNotConfirmed", err)
	}

	if _, err := os.Stat(filepath.Join(env.home, "skills", "gone", "SKILL.md")); err != nil {
		t.Fatal("'gone' should still be on disk — nothing should have been removed without confirmation")
	}
}

func TestApplySwitch_NoChangesNeverCallsConfirm(t *testing.T) {
	env := newTestEnv(t)
	called := false
	if _, _, err := env.eng.ApplySwitch(context.Background(), &Plan{}, &RemovalPlan{}, func(*Plan, *RemovalPlan) bool {
		called = true
		return true
	}); err != nil {
		t.Fatalf("ApplySwitch() error = %v, want nil for a true no-op", err)
	}
	if called {
		t.Error("confirm should never be called when both plans are empty")
	}
}

func TestApplySwitch_InstallAndRemovalInOneStateSave(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "old")
	env.applyAll(t)
	env.addSkill(t, "new")

	installPlan, err := env.eng.Plan(context.Background(), Filter{Artifacts: []string{"new"}})
	if err != nil {
		t.Fatal(err)
	}
	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"old"})
	if err != nil {
		t.Fatal(err)
	}
	installed, removed, err := env.eng.ApplySwitch(context.Background(), installPlan, removalPlan, func(*Plan, *RemovalPlan) bool { return true })
	if err != nil {
		t.Fatalf("ApplySwitch() error = %v", err)
	}
	if installed != 1 || removed != 1 {
		t.Errorf("installed=%d removed=%d, want 1,1", installed, removed)
	}

	if _, err := os.Stat(filepath.Join(env.home, "skills", "new", "SKILL.md")); err != nil {
		t.Error("'new' should now be installed")
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "old")); !os.IsNotExist(err) {
		t.Error("'old' directory should be gone")
	}

	ids, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "new" {
		t.Errorf("InstalledArtifactIDs() = %v, want [new] — one state save reflecting both halves", ids)
	}
}

func TestApplySwitch_PartialWriteThenRerunConverges(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "gone")
	env.applyAll(t)

	// Simulate a crash that removed the directory from disk but never got to
	// save state.yaml (state still thinks "gone" is installed).
	if err := os.RemoveAll(filepath.Join(env.home, "skills", "gone")); err != nil {
		t.Fatal(err)
	}

	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"gone"})
	if err != nil {
		t.Fatal(err)
	}
	_, removed, err := env.eng.ApplySwitch(context.Background(), &Plan{}, removalPlan, func(*Plan, *RemovalPlan) bool { return true })
	if err != nil {
		t.Fatalf("rerun after partial write should converge, got error: %v", err)
	}
	if removed != 1 {
		t.Errorf("removed = %d, want 1 (os.RemoveAll on an already-absent dir is a safe no-op)", removed)
	}

	ids, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("InstalledArtifactIDs() = %v, want empty — state now converged", ids)
	}
}

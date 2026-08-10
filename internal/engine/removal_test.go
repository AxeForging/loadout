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
	rp, err := env.eng.PlanRemoval(context.Background(), installed, RemovalOptions{})
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
	rp, err := env.eng.PlanRemoval(context.Background(), installed, RemovalOptions{})
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
	rp, err := env.eng.PlanRemoval(context.Background(), installed, RemovalOptions{})
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

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"style"}, RemovalOptions{})
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

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"style"}, RemovalOptions{})
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

func TestPlanRemoval_Hook_RemovesSettingsEntryKeepsUserBytes(t *testing.T) {
	env := newTestEnv(t)
	userSettings := "{\n\t\"model\": \"opus\",\n\t\"env\": {\"FOO\": \"bar\"}\n}\n"
	settingsPath := filepath.Join(env.home, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(userSettings), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.New(domain.KindHook, "lint"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"lint"}, RemovalOptions{})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}

	// The hook script (MergeReplace) is a delete Item; settings.json must be an
	// OpUpdate item whose Desired has the merged hook entry surgically removed
	// (ROADMAP:104 — this used to leak the entry forever).
	var settings *Item
	for i := range rp.Items {
		if rp.Items[i].RelPath == "settings.json" {
			settings = &rp.Items[i]
		}
	}
	if settings == nil {
		t.Fatalf("Items = %+v, want an OpUpdate item for settings.json", rp.Items)
	}
	if settings.Op != OpUpdate {
		t.Errorf("settings.json Op = %s, want %s", settings.Op, OpUpdate)
	}
	desired := string(settings.Desired)
	if strings.Contains(desired, "loadout-lint") || strings.Contains(desired, "hooks") {
		t.Errorf("hook entry not removed from Desired:\n%s", desired)
	}
	if !strings.Contains(desired, "\t\"model\": \"opus\"") || !strings.Contains(desired, "{\"FOO\": \"bar\"}") {
		t.Errorf("user settings bytes not preserved:\n%s", desired)
	}
	if len(rp.ClearState) != 1 || rp.ClearState[0].Artifact != "lint" {
		t.Errorf("ClearState = %v, want [lint]", rp.ClearState)
	}
}

func TestPlanRemoval_MCP_RemovesUserScopeServerEntryKeepsOthers(t *testing.T) {
	fakeUserHome := t.TempDir()
	t.Setenv("HOME", fakeUserHome)
	env := newTestEnv(t)

	userConfig := "{\n\t\"numStartups\": 42,\n\t\"mcpServers\": {\n\t\t\"user-own\": {\"command\": \"deno\"}\n\t}\n}\n"
	configPath := filepath.Join(fakeUserHome, ".claude.json")
	if err := os.WriteFile(configPath, []byte(userConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := env.store.New(domain.KindMCP, "gh"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"gh"}, RemovalOptions{})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 1 {
		t.Fatalf("Items = %+v, want one OpUpdate for .claude.json", rp.Items)
	}
	it := rp.Items[0]
	if it.Op != OpUpdate || it.RelPath != ".claude.json" {
		t.Errorf("item = %+v, want OpUpdate on .claude.json", it)
	}
	desired := string(it.Desired)
	if strings.Contains(desired, "\"gh\"") {
		t.Errorf("server entry not removed from Desired:\n%s", desired)
	}
	if !strings.Contains(desired, "\t\"numStartups\": 42") || !strings.Contains(desired, "\"user-own\": {\"command\": \"deno\"}") {
		t.Errorf("user config bytes not preserved:\n%s", desired)
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

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"review"}, RemovalOptions{})
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

	rp, err := eng.PlanRemoval(context.Background(), []string{"review"}, RemovalOptions{})
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

	rp, err := eng.PlanRemoval(context.Background(), []string{"plan"}, RemovalOptions{})
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
	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"gone"}, RemovalOptions{})
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
	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"old"}, RemovalOptions{})
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

	removalPlan, err := env.eng.PlanRemoval(context.Background(), []string{"gone"}, RemovalOptions{})
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

func TestPlanRemoval_EditedReplaceFile_SkippedWithoutForceRemovedWith(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindCommand, "plan"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	cmdPath := filepath.Join(env.home, "commands", "plan.md")
	if err := os.WriteFile(cmdPath, []byte("user rewrote this\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"plan"}, RemovalOptions{})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %+v, want none — edited file must be skipped", rp.Items)
	}
	if len(rp.Warnings) != 1 || !strings.Contains(rp.Warnings[0].Reason, "--force") {
		t.Errorf("Warnings = %v, want one pointing at --force", rp.Warnings)
	}

	forced, err := env.eng.PlanRemoval(context.Background(), []string{"plan"}, RemovalOptions{Force: true})
	if err != nil {
		t.Fatalf("PlanRemoval(force) error = %v", err)
	}
	if len(forced.Items) != 1 || forced.Items[0].Op != OpDelete {
		t.Errorf("forced Items = %+v, want the delete", forced.Items)
	}
}

func TestPlanRemoval_SkillDirWithUserAddedFile_SkippedWithoutForce(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	if err := os.WriteFile(filepath.Join(env.home, "skills", "review", "my-notes.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"review"}, RemovalOptions{})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %+v, want none — dir with a user file must be skipped", rp.Items)
	}
	if len(rp.Warnings) != 1 || !strings.Contains(rp.Warnings[0].Reason, "not installed by loadout") {
		t.Errorf("Warnings = %v, want one naming the foreign file", rp.Warnings)
	}
}

func TestPlanRemoval_EditedBlock_SkippedWithoutForce(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindInstruction, "style"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "<!-- loadout:end style -->", "user edit inside the block\n<!-- loadout:end style -->", 1)
	if err := os.WriteFile(claudeMD, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"style"}, RemovalOptions{})
	if err != nil {
		t.Fatalf("PlanRemoval() error = %v", err)
	}
	if len(rp.Items) != 0 {
		t.Errorf("Items = %+v, want none — edited block must be skipped", rp.Items)
	}
	if len(rp.Warnings) != 1 || !strings.Contains(rp.Warnings[0].Reason, "--force") {
		t.Errorf("Warnings = %v, want one pointing at --force", rp.Warnings)
	}
}

func TestApplyRemoval_WithoutConfirmationWritesNothing(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "gone")
	env.applyAll(t)

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"gone"}, RemovalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.eng.ApplyRemoval(context.Background(), rp, nil); err != ErrNotConfirmed {
		t.Fatalf("ApplyRemoval(nil confirm) error = %v, want ErrNotConfirmed", err)
	}
	if _, err := env.eng.ApplyRemoval(context.Background(), rp, func(*RemovalPlan) bool { return false }); err != ErrNotConfirmed {
		t.Fatalf("ApplyRemoval(decline) error = %v, want ErrNotConfirmed", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "gone", "SKILL.md")); err != nil {
		t.Fatal("nothing may be removed without confirmation")
	}
}

func TestApplyRemoval_RemovesFilesAndClearsState(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "gone")
	env.applyAll(t)

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"gone"}, RemovalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	n, err := env.eng.ApplyRemoval(context.Background(), rp, func(*RemovalPlan) bool { return true })
	if err != nil {
		t.Fatalf("ApplyRemoval() error = %v", err)
	}
	if n != 1 {
		t.Errorf("removed = %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "gone")); !os.IsNotExist(err) {
		t.Error("skill directory should be gone")
	}
	ids, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("InstalledArtifactIDs() = %v, want empty", ids)
	}
}

func TestApplyRemoval_SnapshotsStoreFilesAndRestoreResurrects(t *testing.T) {
	env := newTestEnv(t)
	backupDir := filepath.Join(env.store.Root, "..", "backups")
	env.eng.BackupDir = backupDir
	env.addSkill(t, "gone")
	env.applyAll(t)

	rp, err := env.eng.PlanRemoval(context.Background(), []string{"gone"}, RemovalOptions{RemoveStore: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.StoreFiles) != 2 {
		t.Fatalf("StoreFiles = %+v, want content dir + manifest", rp.StoreFiles)
	}
	if _, err := env.eng.ApplyRemoval(context.Background(), rp, func(*RemovalPlan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	// Simulate the CLI's follow-up store deletion.
	if err := env.store.RemoveArtifact("gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.store.Root, "skills", "gone")); !os.IsNotExist(err) {
		t.Fatal("store content should be deleted")
	}

	snaps, err := ListSnapshots(backupDir)
	if err != nil || len(snaps) == 0 {
		t.Fatalf("ListSnapshots() = %v, %v — want the rm snapshot", snaps, err)
	}
	snap := snaps[0]
	if snap.Trigger != "rm" {
		t.Errorf("Trigger = %q, want rm", snap.Trigger)
	}
	if _, err := env.eng.ApplyRestore(context.Background(), &snap, func(*Snapshot) bool { return true }); err != nil {
		t.Fatalf("ApplyRestore() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.store.Root, "skills", "gone", "SKILL.md")); err != nil {
		t.Error("restore should resurrect the store content")
	}
	raw, err := os.ReadFile(filepath.Join(env.store.Root, "loadout.yaml"))
	if err != nil || !strings.Contains(string(raw), "gone") {
		t.Error("restore should resurrect the manifest entry")
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "gone", "SKILL.md")); err != nil {
		t.Error("restore should resurrect the installed skill file")
	}
}

func TestApplyRemoval_DriftSkipRetainsStateSoForceStillWorks(t *testing.T) {
	env := newTestEnv(t)
	if _, err := env.store.New(domain.KindCommand, "plan"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	cmdPath := filepath.Join(env.home, "commands", "plan.md")
	if err := os.WriteFile(cmdPath, []byte("user rewrote this\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// First rm: drift-skips the edited file but must NOT forget it was
	// loadout-installed — otherwise --force afterwards has nothing to act on.
	rp, err := env.eng.PlanRemoval(context.Background(), []string{"plan"}, RemovalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.eng.ApplyRemoval(context.Background(), rp, func(*RemovalPlan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	ids, err := env.eng.InstalledArtifactIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "plan" {
		t.Fatalf("InstalledArtifactIDs() = %v, want the drift-skipped record retained", ids)
	}

	forced, err := env.eng.PlanRemoval(context.Background(), []string{"plan"}, RemovalOptions{Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(forced.Items) != 1 {
		t.Fatalf("forced Items = %+v, want the delete to still be plannable", forced.Items)
	}
	if _, err := env.eng.ApplyRemoval(context.Background(), forced, func(*RemovalPlan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cmdPath); !os.IsNotExist(err) {
		t.Error("--force should remove the edited file")
	}
	ids, _ = env.eng.InstalledArtifactIDs()
	if len(ids) != 0 {
		t.Errorf("state should be fully cleared after the forced removal, got %v", ids)
	}
}

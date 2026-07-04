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

// testEnv wires a real store, a claude-code adapter rooted in a temp home,
// and an engine with a temp state file.
type testEnv struct {
	eng   *Engine
	store *store.Store
	home  string // fake ~/.claude
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatalf("store.Init() error = %v", err)
	}
	home := filepath.Join(dir, "claude-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(claudecode.NewWithHome(home))
	return &testEnv{
		eng:   &Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")},
		store: s,
		home:  home,
	}
}

func (env *testEnv) addSkill(t *testing.T, name string) domain.Artifact {
	t.Helper()
	a, err := env.store.New(domain.KindSkill, name)
	if err != nil {
		t.Fatalf("New(skill) error = %v", err)
	}
	return a
}

func (env *testEnv) applyAll(t *testing.T) {
	t.Helper()
	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
}

func (env *testEnv) statusOf(t *testing.T, artifact string) domain.SyncState {
	t.Helper()
	rows, _, err := env.eng.Status(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	for _, r := range rows {
		if r.Artifact == artifact {
			return r.State
		}
	}
	t.Fatalf("no status row for %s (rows: %v)", artifact, rows)
	return ""
}

func TestPlan_CreatesMissingSkillFiles(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	changes := plan.Changes()
	if len(changes) != 1 || changes[0].Op != OpCreate {
		t.Fatalf("expected 1 create, got %v", changes)
	}
	if changes[0].RelPath != "skills/review/SKILL.md" {
		t.Errorf("RelPath = %s", changes[0].RelPath)
	}
}

func TestApply_WithoutConfirmationWritesNothing(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}

	// nil confirm callback
	if _, err := env.eng.Apply(context.Background(), plan, nil); err != ErrNotConfirmed {
		t.Fatalf("Apply(nil) error = %v, want ErrNotConfirmed", err)
	}
	// declining confirm callback
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return false }); err != ErrNotConfirmed {
		t.Fatalf("Apply(decline) error = %v, want ErrNotConfirmed", err)
	}

	if _, err := os.Stat(filepath.Join(env.home, "skills")); !os.IsNotExist(err) {
		t.Fatal("agent home was written to without confirmation")
	}
	if _, err := os.Stat(env.eng.StatePath); !os.IsNotExist(err) {
		t.Fatal("state file was written without confirmation")
	}
}

func TestApply_InstallsAndSecondPlanIsNoop(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	if _, err := os.Stat(filepath.Join(env.home, "skills", "review", "SKILL.md")); err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	if got := env.statusOf(t, "review"); got != domain.StateInSync {
		t.Errorf("status = %s, want in-sync", got)
	}

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes()) != 0 {
		t.Errorf("second plan should be empty, got %v", plan.Changes())
	}
}

func TestStatus_MissingBeforeInstall(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	if got := env.statusOf(t, "review"); got != domain.StateMissing {
		t.Errorf("status = %s, want missing", got)
	}
}

func TestStatus_DetectsLocalDriftWhenAgentFileEdited(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	installed := filepath.Join(env.home, "skills", "review", "SKILL.md")
	if err := os.WriteFile(installed, []byte("user edited this\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "review"); got != domain.StateDriftedLocal {
		t.Errorf("status = %s, want drifted-local", got)
	}
}

func TestStatus_DetectsStoreDriftWhenStoreUpdated(t *testing.T) {
	env := newTestEnv(t)
	a := env.addSkill(t, "review")
	env.applyAll(t)

	storeFile := filepath.Join(env.store.Root, a.Path, "SKILL.md")
	if err := os.WriteFile(storeFile, []byte("---\nname: review\n---\nv2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "review"); got != domain.StateDriftedStore {
		t.Errorf("status = %s, want drifted-store", got)
	}
}

func TestStatus_ConflictWhenBothStoreAndDiskChanged(t *testing.T) {
	env := newTestEnv(t)
	a := env.addSkill(t, "review")
	env.applyAll(t)

	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path, "SKILL.md"), []byte("store v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.home, "skills", "review", "SKILL.md"), []byte("disk v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "review"); got != domain.StateConflict {
		t.Errorf("status = %s, want conflict", got)
	}
}

func TestStatus_ConflictWhenUnmanagedFileInTheWay(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")

	preexisting := filepath.Join(env.home, "skills", "review", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(preexisting), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(preexisting, []byte("not created by loadout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "review"); got != domain.StateConflict {
		t.Errorf("status = %s, want conflict", got)
	}
}

func TestInstruction_ManagedBlockPreservesUserClaudeMD(t *testing.T) {
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

	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.HasPrefix(s, userContent) {
		t.Errorf("user CLAUDE.md content not preserved:\n%s", s)
	}
	if !strings.Contains(s, "<!-- loadout:begin style -->") {
		t.Errorf("managed block missing:\n%s", s)
	}
	if got := env.statusOf(t, "style"); got != domain.StateInSync {
		t.Errorf("status = %s, want in-sync", got)
	}
}

func TestInstruction_StoreEditFlowsIntoBlockOnResync(t *testing.T) {
	env := newTestEnv(t)
	a, err := env.store.New(domain.KindInstruction, "style")
	if err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path), []byte("updated rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "style"); got != domain.StateDriftedStore {
		t.Fatalf("status = %s, want drifted-store", got)
	}
	env.applyAll(t)

	raw, _ := os.ReadFile(filepath.Join(env.home, "CLAUDE.md"))
	if !strings.Contains(string(raw), "updated rule") {
		t.Errorf("resync did not update block:\n%s", raw)
	}
	if got := env.statusOf(t, "style"); got != domain.StateInSync {
		t.Errorf("status after resync = %s, want in-sync", got)
	}
}

func TestHook_InstallMergesSettingsWithoutClobberingUserSettings(t *testing.T) {
	env := newTestEnv(t)
	settingsPath := filepath.Join(env.home, "settings.json")
	if err := os.WriteFile(settingsPath, []byte(`{"model": "opus", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "mine.sh"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := env.store.New(domain.KindHook, "lint"); err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{`"model"`, "mine.sh", "loadout-lint.sh"} {
		if !strings.Contains(s, want) {
			t.Errorf("settings.json missing %s:\n%s", want, s)
		}
	}
	script := filepath.Join(env.home, "hooks", "loadout-lint.sh")
	info, err := os.Stat(script)
	if err != nil {
		t.Fatalf("hook script not installed: %v", err)
	}
	if info.Mode().Perm()&0o100 == 0 {
		t.Error("hook script not executable")
	}
}

func TestPlan_SkipsAgentWhoseHomeIsMissing(t *testing.T) {
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(claudecode.NewWithHome(filepath.Join(dir, "does-not-exist")))
	eng := &Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")}

	plan, err := eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Skips) != 1 || plan.Skips[0].Agent != domain.AgentClaudeCode {
		t.Errorf("expected claude-code skip, got %v", plan.Skips)
	}
}

func TestPlan_FilterLimitsArtifacts(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "one")
	env.addSkill(t, "two")

	plan, err := env.eng.Plan(context.Background(), Filter{Artifacts: []string{"one"}})
	if err != nil {
		t.Fatal(err)
	}
	changes := plan.Changes()
	if len(changes) != 1 || changes[0].Artifact.ID != "one" {
		t.Errorf("filter leaked: %v", changes)
	}
}

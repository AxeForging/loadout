package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AxeForging/loadout/internal/domain"
)

// enableBackups points the env's engine at a temp backup dir and installs a
// deterministic, strictly-increasing clock so every snapshot gets a distinct ID.
func (env *testEnv) enableBackups(t *testing.T) string {
	t.Helper()
	bdir := filepath.Join(filepath.Dir(env.eng.StatePath), "backups")
	env.eng.BackupDir = bdir
	base := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	var n int
	env.eng.Clock = func() time.Time {
		n++
		return base.Add(time.Duration(n) * time.Second)
	}
	return bdir
}

func TestApply_SnapshotsOriginalContentBeforeOverwrite(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "plan")
	abs := env.writeUserFile(t, "skills/plan/SKILL.md", "ORIGINAL\n")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	plan.OverwriteConflicts = true
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if raw, _ := os.ReadFile(abs); string(raw) == "ORIGINAL\n" {
		t.Fatal("forced apply did not overwrite the file")
	}

	snaps, err := ListSnapshots(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	backup := filepath.Join(snaps[0].dir, "files", "claude-code", "skills", "plan", "SKILL.md")
	raw, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("snapshot missing original file: %v", err)
	}
	if string(raw) != "ORIGINAL\n" {
		t.Errorf("snapshot content = %q, want ORIGINAL", raw)
	}
}

func TestApply_TakesNoSnapshotWhenOnlyCreating(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "review") // nothing on disk yet — a pure create
	env.applyAll(t)

	snaps, err := ListSnapshots(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 0 {
		t.Errorf("snapshots = %d, want 0 (pure create has nothing to preserve)", len(snaps))
	}
}

func TestApplySwitch_SnapshotsDeletedSkillDirectory(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	// Remove "review" via a switch (empty install plan + removal of the skill).
	removal, err := env.eng.PlanRemoval(context.Background(), []string{"review"}, RemovalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := env.eng.ApplySwitch(context.Background(), &Plan{}, removal, func(*Plan, *RemovalPlan) bool { return true }); err != nil {
		t.Fatalf("ApplySwitch() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.home, "skills", "review")); !os.IsNotExist(err) {
		t.Fatal("skill directory should have been deleted by the switch")
	}

	snaps, err := ListSnapshots(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	backup := filepath.Join(snaps[0].dir, "files", "claude-code", "skills", "review", "SKILL.md")
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("deleted skill directory not preserved in snapshot: %v", err)
	}
}

func TestApplyRestore_WritesOriginalBytesBackAfterConfirm(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "plan")
	abs := env.writeUserFile(t, "skills/plan/SKILL.md", "ORIGINAL\n")

	plan, _ := env.eng.Plan(context.Background(), Filter{})
	plan.OverwriteConflicts = true
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}

	snaps, _ := ListSnapshots(bdir)
	snap, err := LoadSnapshot(bdir, snaps[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := env.eng.ApplyRestore(context.Background(), snap, func(*Snapshot) bool { return true })
	if err != nil {
		t.Fatalf("ApplyRestore() error = %v", err)
	}
	if n != 1 {
		t.Errorf("restored %d, want 1", n)
	}
	if raw, _ := os.ReadFile(abs); string(raw) != "ORIGINAL\n" {
		t.Errorf("restore did not bring the original back: %q", raw)
	}
}

func TestApplyRestore_DeclinedConfirmWritesNothing(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "plan")
	abs := env.writeUserFile(t, "skills/plan/SKILL.md", "ORIGINAL\n")
	plan, _ := env.eng.Plan(context.Background(), Filter{})
	plan.OverwriteConflicts = true
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	overwritten, _ := os.ReadFile(abs)

	snaps, _ := ListSnapshots(bdir)
	snap, _ := LoadSnapshot(bdir, snaps[0].ID)

	if _, err := env.eng.ApplyRestore(context.Background(), snap, nil); err != ErrNotConfirmed {
		t.Fatalf("ApplyRestore(nil) = %v, want ErrNotConfirmed", err)
	}
	if _, err := env.eng.ApplyRestore(context.Background(), snap, func(*Snapshot) bool { return false }); err != ErrNotConfirmed {
		t.Fatalf("ApplyRestore(decline) = %v, want ErrNotConfirmed", err)
	}
	if raw, _ := os.ReadFile(abs); string(raw) != string(overwritten) {
		t.Error("declined restore modified the file")
	}
}

func TestApplyRestore_DoesNotTouchStateFile(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "plan")
	env.writeUserFile(t, "skills/plan/SKILL.md", "ORIGINAL\n")
	plan, _ := env.eng.Plan(context.Background(), Filter{})
	plan.OverwriteConflicts = true
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	stateBefore, err := os.ReadFile(env.eng.StatePath)
	if err != nil {
		t.Fatal(err)
	}

	snaps, _ := ListSnapshots(bdir)
	snap, _ := LoadSnapshot(bdir, snaps[0].ID)
	if _, err := env.eng.ApplyRestore(context.Background(), snap, func(*Snapshot) bool { return true }); err != nil {
		t.Fatal(err)
	}

	stateAfter, err := os.ReadFile(env.eng.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(stateBefore) != string(stateAfter) {
		t.Error("ApplyRestore modified state.yaml; it must leave install state untouched")
	}
}

func TestPruneSnapshots_KeepsNewestN(t *testing.T) {
	bdir := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var ids []string
	for i := 0; i < 5; i++ {
		src := filepath.Join(t.TempDir(), "f.txt")
		if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		snap, err := captureSnapshot(bdir, "sync",
			[]snapTarget{{agent: domain.AgentClaudeCode, relPath: "commands/x.md", absPath: src, op: OpUpdate}},
			base.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, snap.ID)
	}

	if err := pruneSnapshots(bdir, 2); err != nil {
		t.Fatal(err)
	}
	snaps, err := ListSnapshots(bdir)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("kept %d snapshots, want 2", len(snaps))
	}
	// The two newest (last created) must be the survivors.
	kept := map[string]bool{snaps[0].ID: true, snaps[1].ID: true}
	if !kept[ids[4]] || !kept[ids[3]] {
		t.Errorf("prune kept %v, want newest two %s, %s", kept, ids[4], ids[3])
	}
}

package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

// addInstruction creates an instruction artifact with the given body.
func (env *testEnv) addInstruction(t *testing.T, name, body string) domain.Artifact {
	t.Helper()
	a, err := env.store.New(domain.KindInstruction, name)
	if err != nil {
		t.Fatalf("New(instruction) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return a
}

func (env *testEnv) plan(t *testing.T) *Plan {
	t.Helper()
	p, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	return p
}

func (env *testEnv) stateHash(t *testing.T, artifact, relPath string) string {
	t.Helper()
	st, err := loadState(env.eng.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	return st.get(artifact, "claude-code", relPath)
}

// Reproduces the incident behind spec 010: a user hand-edits the managed
// block loadout installed into CLAUDE.md, and a plain sync silently replaced
// it with the store version.
func TestApply_DoesNotSilentlyOverwriteHandEditedManagedBlock(t *testing.T) {
	env := newTestEnv(t)
	env.addInstruction(t, "conventions", "Use conventional commits.\n")
	env.applyAll(t)

	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	raw, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(raw), "Use conventional commits.", "Use conventional commits.\nPR bodies stay short.", 1)
	if err := os.WriteFile(claudeMD, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	hashBefore := env.stateHash(t, "conventions", "CLAUDE.md")
	if got := env.statusOf(t, "conventions"); got != domain.StateDriftedLocal {
		t.Fatalf("precondition: status = %s, want drifted-local", got)
	}

	p := env.plan(t)
	if len(p.Changes()) != 0 {
		t.Fatalf("Changes() = %d item(s), want the local edit excluded by default", len(p.Changes()))
	}
	locals := p.LocalEdits()
	if len(locals) != 1 || locals[0].RelPath != "CLAUDE.md" {
		t.Fatalf("LocalEdits() = %d item(s), want CLAUDE.md", len(locals))
	}

	if _, err := env.eng.Apply(context.Background(), p, func(*Plan) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(claudeMD); string(got) != edited {
		t.Fatalf("plain sync overwrote the hand-edited block:\n%s", got)
	}
	if got := env.stateHash(t, "conventions", "CLAUDE.md"); got != hashBefore {
		t.Errorf("state hash changed for a skipped local edit: %q → %q", hashBefore, got)
	}
	if got := env.statusOf(t, "conventions"); got != domain.StateDriftedLocal {
		t.Errorf("status after skipped sync = %s, want still drifted-local", got)
	}
}

func TestApply_DoesNotSilentlyOverwriteHandEditedSkillFile(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.addSkill(t, "other")
	env.applyAll(t)

	abs := filepath.Join(env.home, "skills", "review", "SKILL.md")
	if err := os.WriteFile(abs, []byte("my tweaks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Run a sync that has other work to do, so the skip is not a trivial noop.
	other := filepath.Join(env.home, "skills", "other", "SKILL.md")
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}

	p := env.plan(t)
	n, err := env.eng.Apply(context.Background(), p, func(*Plan) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("applied %d change(s), want only the missing file", n)
	}
	if got, _ := os.ReadFile(abs); string(got) != "my tweaks\n" {
		t.Fatalf("sync overwrote hand-edited skill: %q", got)
	}
	if got := env.statusOf(t, "review"); got != domain.StateDriftedLocal {
		t.Errorf("status = %s, want drifted-local (install record preserved)", got)
	}
}

func TestApply_OverwritesLocalEditWhenForcedAndSnapshotsIt(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	env.addSkill(t, "review")
	env.applyAll(t)

	abs := filepath.Join(env.home, "skills", "review", "SKILL.md")
	if err := os.WriteFile(abs, []byte("my tweaks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := env.plan(t)
	p.OverwriteConflicts = true
	if n, err := env.eng.Apply(context.Background(), p, func(*Plan) bool { return true }); err != nil || n != 1 {
		t.Fatalf("Apply() = %d, %v; want 1 forced write", n, err)
	}
	if got, _ := os.ReadFile(abs); string(got) == "my tweaks\n" {
		t.Fatal("forced sync did not overwrite the local edit")
	}
	if got := env.statusOf(t, "review"); got != domain.StateInSync {
		t.Errorf("status after forced sync = %s, want in-sync", got)
	}
	snaps, err := ListSnapshots(bdir)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("snapshots = %d (err %v), want 1", len(snaps), err)
	}
	saved, err := os.ReadFile(filepath.Join(snaps[0].dir, "files", "claude-code", "skills", "review", "SKILL.md"))
	if err != nil || string(saved) != "my tweaks\n" {
		t.Errorf("snapshot = %q (err %v), want the local edit", saved, err)
	}
}

func TestPlan_BothStoreAndDiskMovedIsALocalEditNotAnUnownedConflict(t *testing.T) {
	env := newTestEnv(t)
	a := env.addInstruction(t, "conventions", "v1\n")
	env.applyAll(t)

	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	raw, _ := os.ReadFile(claudeMD)
	if err := os.WriteFile(claudeMD, []byte(strings.Replace(string(raw), "v1", "v1 local", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := env.plan(t)
	if len(p.Changes()) != 0 {
		t.Fatalf("Changes() = %d item(s), want both-moved block held back", len(p.Changes()))
	}
	if len(p.LocalEdits()) != 1 {
		t.Fatalf("LocalEdits() = %d item(s), want 1", len(p.LocalEdits()))
	}
}

func TestPlan_StoreOnlyChangeStillUpdatesNormally(t *testing.T) {
	env := newTestEnv(t)
	a := env.addInstruction(t, "conventions", "v1\n")
	env.applyAll(t)
	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := env.plan(t)
	if len(p.LocalEdits()) != 0 || len(p.Changes()) != 1 {
		t.Fatalf("drifted-store must stay a plain update: changes=%d local=%d", len(p.Changes()), len(p.LocalEdits()))
	}
}

// Two managed blocks share CLAUDE.md: the held-back local edit of one must
// survive the write of the other, and forcing must update both.
func TestApply_SharedFileKeepsSkippedBlockWhileUpdatingSibling(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "forced"}[force], func(t *testing.T) {
			env := newTestEnv(t)
			env.addInstruction(t, "alpha", "alpha v1\n")
			beta := env.addInstruction(t, "beta", "beta v1\n")
			env.applyAll(t)

			claudeMD := filepath.Join(env.home, "CLAUDE.md")
			raw, _ := os.ReadFile(claudeMD)
			if err := os.WriteFile(claudeMD, []byte(strings.Replace(string(raw), "alpha v1", "alpha MINE", 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(env.store.Root, beta.Path), []byte("beta v2\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			p := env.plan(t)
			// The sibling's planned content (what `diff` shows) must not
			// carry the store version of the held-back block.
			for _, it := range p.Changes() {
				if it.Artifact.ID == "beta" && !strings.Contains(string(it.Desired), "alpha MINE") {
					t.Errorf("beta's planned content reverts the held-back alpha edit:\n%s", it.Desired)
				}
			}
			p.OverwriteConflicts = force
			if _, err := env.eng.Apply(context.Background(), p, func(*Plan) bool { return true }); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(claudeMD)
			if !strings.Contains(string(got), "beta v2") {
				t.Errorf("sibling block not updated:\n%s", got)
			}
			wantAlpha, lostAlpha := "alpha MINE", "alpha v1"
			if force {
				wantAlpha, lostAlpha = "alpha v1", "alpha MINE"
			}
			if !strings.Contains(string(got), wantAlpha) || strings.Contains(string(got), lostAlpha) {
				t.Errorf("alpha block: want %q, not %q:\n%s", wantAlpha, lostAlpha, got)
			}
		})
	}
}

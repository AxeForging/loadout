package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func editStoreSkill(t *testing.T, m *Manager, name, body string) {
	t.Helper()
	path := filepath.Join(m.Store.Root, "skills", name, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func clearSourceHash(t *testing.T, m *Manager, name string) {
	t.Helper()
	if err := m.Store.SetSourceHash(name, ""); err != nil {
		t.Fatal(err)
	}
}

func TestPlanUpdate_LocalEditWithUnrelatedUpstreamCommitIsLocallyEdited(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "alpha v1")
	u.writeSkill("beta", "beta v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "alpha", "customized")
	u.writeSkill("beta", "beta v2")
	u.commit("beta only")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.LocallyEdited) != 1 || plan.LocallyEdited[0].Name != "alpha" {
		t.Errorf("LocallyEdited = %v, want [alpha]", plan.LocallyEdited)
	}
	if len(plan.Changed) != 1 || plan.Changed[0].Name != "beta" {
		t.Errorf("Changed = %v, want [beta]", plan.Changed)
	}
	if len(plan.Conflicts) != 0 {
		t.Errorf("Conflicts = %v, want none", plan.Conflicts)
	}

	// Apply must refresh beta, leave alpha untouched.
	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "alpha", "SKILL.md"))
	if string(raw) != "customized" {
		t.Errorf("local edit clobbered: %s", raw)
	}
}

func TestPlanUpdate_BothSidesMovedIsConflict_GatedByForceDiscard(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "alpha v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "alpha", "customized")
	u.writeSkill("alpha", "alpha v2 upstream")
	u.commit("rework alpha")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Name != "alpha" {
		t.Fatalf("Conflicts = %v, want [alpha]", plan.Conflicts)
	}

	// Without force: content untouched, pin still moves.
	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "alpha", "SKILL.md"))
	if string(raw) != "customized" {
		t.Fatalf("conflict written without force-discard: %s", raw)
	}

	// The conflict stays visible even though the pin is now up-to-date.
	plan, err = m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.UpToDate {
		t.Error("plan should be up-to-date after pin move")
	}
	if len(plan.Conflicts) != 1 {
		t.Fatalf("skipped conflict lost after pin move: %v", plan.Conflicts)
	}

	// Forced: upstream wins and the baseline is restamped.
	if err := m.ApplyUpdate(plan, true); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(m.Store.Root, "skills", "alpha", "SKILL.md"))
	if !strings.Contains(string(raw), "alpha v2 upstream") {
		t.Fatalf("force-discard did not apply upstream: %s", raw)
	}
	art, _ := m.Store.Artifact("alpha")
	if art.SourceHash == "" {
		t.Error("baseline not restamped after forced write")
	}
	plan, err = m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 0 || len(plan.LocallyEdited) != 0 {
		t.Errorf("after forced apply the skill should be clean, got conflicts=%v edited=%v", plan.Conflicts, plan.LocallyEdited)
	}
}

func TestPlanUpdate_MissingBaselineBackfilledFromPinnedCommit(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "alpha v1")
	u.writeSkill("beta", "beta v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-008 store: no recorded baseline.
	clearSourceHash(t, m, "alpha")
	editStoreSkill(t, m, "alpha", "customized")
	u.writeSkill("beta", "beta v2")
	u.commit("beta only")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.LocallyEdited) != 1 || plan.LocallyEdited[0].Name != "alpha" {
		t.Errorf("backfill should detect the local edit, got LocallyEdited=%v Changed=%v", plan.LocallyEdited, plan.Changed)
	}
	if len(plan.LegacyBaseline) != 0 {
		t.Errorf("reachable pin must not be legacy, got %v", plan.LegacyBaseline)
	}
}

func TestPlanUpdate_UnreachablePinDegradesToLabeledLegacy(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "alpha v1")
	u.commit("initial")

	m := newManager(t)
	// file:// forces a real fetch (a plain local path would hardlink the
	// whole object store, keeping even rewritten-away commits reachable).
	if _, err := m.Add(context.Background(), "src", "file://"+u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	clearSourceHash(t, m, "alpha")
	editStoreSkill(t, m, "alpha", "customized")

	// Rewrite upstream history so the pinned commit no longer exists in a
	// fresh clone.
	u.writeSkill("alpha", "alpha v2 rewritten")
	u.git("add", "-A")
	u.git("commit", "--amend", "-m", "rewritten initial")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.LegacyBaseline) != 1 || plan.LegacyBaseline[0] != "alpha" {
		t.Errorf("LegacyBaseline = %v, want [alpha]", plan.LegacyBaseline)
	}
	if len(plan.Changed) != 1 || plan.Changed[0].Name != "alpha" {
		t.Errorf("legacy skill should fall back to two-way changed, got %v", plan.Changed)
	}
}

func TestPlanUpdate_UpstreamNameCollisionReportedAsOwnedElsewhere(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Store.New(domain.KindSkill, "mine"); err != nil {
		t.Fatal(err)
	}
	u.writeSkill("mine", "upstream impostor")
	u.commit("adds colliding skill")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.OwnedElsewhere) != 1 || plan.OwnedElsewhere[0] != "mine" {
		t.Errorf("OwnedElsewhere = %v, want [mine]", plan.OwnedElsewhere)
	}
}

func TestApplyUpdate_FirstUpdateStampsMissingBaselines(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "alpha v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	// Pre-008 store: pristine content, no baseline recorded.
	clearSourceHash(t, m, "alpha")
	u.writeSkill("beta", "beta v1")
	u.commit("adds beta")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	art, _ := m.Store.Artifact("alpha")
	if art.SourceHash == "" {
		t.Error("pristine legacy skill should have its baseline backfilled on apply")
	}
}

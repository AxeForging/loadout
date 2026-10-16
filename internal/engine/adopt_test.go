package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter/geminicli"
	"github.com/AxeForging/loadout/internal/domain"
)

func yes(*AdoptPlan) bool { return true }

func TestAdopt_ManagedBlockEditLandsInStoreAndArtifactIsInSync(t *testing.T) {
	env := newTestEnv(t)
	bdir := env.enableBackups(t)
	a := env.addInstruction(t, "conventions", "Use conventional commits.\n")
	env.applyAll(t)

	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	raw, _ := os.ReadFile(claudeMD)
	edited := "# mine\n\n" + strings.Replace(string(raw), "commits.", "commits.\nPR bodies stay short.", 1)
	if err := os.WriteFile(claudeMD, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	ap, err := env.eng.PlanAdopt(context.Background(), "conventions", "claude-code")
	if err != nil {
		t.Fatalf("PlanAdopt() error = %v", err)
	}
	if len(ap.Changed) != 1 {
		t.Fatalf("Changed = %v, want the instruction file", ap.Changed)
	}
	if n, err := env.eng.ApplyAdopt(context.Background(), ap, yes); err != nil || n != 1 {
		t.Fatalf("ApplyAdopt() = %d, %v", n, err)
	}

	got, _ := os.ReadFile(filepath.Join(env.store.Root, a.Path))
	if string(got) != "Use conventional commits.\nPR bodies stay short.\n" {
		t.Errorf("store content = %q", got)
	}
	if after, _ := os.ReadFile(claudeMD); string(after) != edited {
		t.Errorf("adopt modified the agent home:\n%s", after)
	}
	if s := env.statusOf(t, "conventions"); s != domain.StateInSync {
		t.Errorf("status after adopt = %s, want in-sync", s)
	}
	// The previous store version is recoverable.
	if snaps, _ := ListSnapshots(bdir); len(snaps) != 1 || snaps[0].Trigger != "adopt" {
		t.Errorf("want one 'adopt' snapshot, got %d", len(snaps))
	}

	// A later store edit must read as drifted-store (install record refreshed),
	// not as a conflict that would be held back.
	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path), []byte("v3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s := env.statusOf(t, "conventions"); s != domain.StateDriftedStore {
		t.Errorf("status after store edit = %s, want drifted-store", s)
	}
}

func TestAdopt_SkillReportsDiskOnlyFilesAsIgnored(t *testing.T) {
	env := newTestEnv(t)
	a := env.addSkill(t, "review")
	env.applyAll(t)

	dir := filepath.Join(env.home, "skills", "review")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review\n---\nmine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ap, err := env.eng.PlanAdopt(context.Background(), "review", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(ap.Ignored) != 1 || ap.Ignored[0] != "notes.md" {
		t.Errorf("Ignored = %v, want [notes.md]", ap.Ignored)
	}
	if _, err := env.eng.ApplyAdopt(context.Background(), ap, yes); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.store.Root, a.Path, "notes.md")); !os.IsNotExist(err) {
		t.Error("adopt added a disk-only file to the store")
	}
	got, _ := os.ReadFile(filepath.Join(env.store.Root, a.Path, "SKILL.md"))
	if !strings.Contains(string(got), "mine") {
		t.Errorf("store SKILL.md = %q", got)
	}
}

func TestAdopt_DeclinedConfirmWritesNothing(t *testing.T) {
	env := newTestEnv(t)
	a := env.addInstruction(t, "conventions", "v1\n")
	env.applyAll(t)
	claudeMD := filepath.Join(env.home, "CLAUDE.md")
	raw, _ := os.ReadFile(claudeMD)
	_ = os.WriteFile(claudeMD, []byte(strings.Replace(string(raw), "v1", "v1 mine", 1)), 0o644)

	ap, err := env.eng.PlanAdopt(context.Background(), "conventions", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	for _, confirm := range []func(*AdoptPlan) bool{nil, func(*AdoptPlan) bool { return false }} {
		if _, err := env.eng.ApplyAdopt(context.Background(), ap, confirm); !errors.Is(err, ErrNotConfirmed) {
			t.Fatalf("ApplyAdopt() error = %v, want ErrNotConfirmed", err)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(env.store.Root, a.Path)); string(got) != "v1\n" {
		t.Errorf("store written without confirmation: %q", got)
	}
}

func TestPlanAdopt_RefusesUnknownOrUninstalled(t *testing.T) {
	env := newTestEnv(t)
	env.addInstruction(t, "conventions", "v1\n")

	if _, err := env.eng.PlanAdopt(context.Background(), "nope", "claude-code"); err == nil {
		t.Error("unknown artifact accepted")
	}
	if _, err := env.eng.PlanAdopt(context.Background(), "conventions", "not-an-agent"); err == nil {
		t.Error("unknown agent accepted")
	}
	// Never synced: there is no block on disk to adopt.
	_, err := env.eng.PlanAdopt(context.Background(), "conventions", "claude-code")
	if err == nil || !strings.Contains(err.Error(), "nothing to adopt") {
		t.Errorf("PlanAdopt() on uninstalled = %v, want 'nothing to adopt'", err)
	}
}

func TestPlanAdopt_InSyncArtifactHasNoChanges(t *testing.T) {
	env := newTestEnv(t)
	env.addSkill(t, "review")
	env.applyAll(t)
	ap, err := env.eng.PlanAdopt(context.Background(), "review", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if len(ap.Changed) != 0 {
		t.Errorf("Changed = %v, want none", ap.Changed)
	}
}

func TestPlanAdopt_RefusesAgentThatRewritesTheFormat(t *testing.T) {
	env := newTestEnv(t)
	gemHome := filepath.Join(filepath.Dir(env.home), "gemini-home")
	if err := os.MkdirAll(gemHome, 0o755); err != nil {
		t.Fatal(err)
	}
	env.eng.Registry.Register(geminicli.NewWithHome(gemHome))
	a, err := env.store.New(domain.KindCommand, "plan")
	if err != nil {
		t.Fatal(err)
	}
	env.applyAll(t)
	before, _ := os.ReadFile(filepath.Join(env.store.Root, a.Path))

	// gemini-cli installs commands as generated skills — not reversible.
	_, err = env.eng.PlanAdopt(context.Background(), "plan", "gemini-cli")
	if !errors.Is(err, ErrNotReversible) {
		t.Fatalf("PlanAdopt(gemini-cli) error = %v, want ErrNotReversible", err)
	}
	// claude-code installs the command verbatim — reversible.
	if _, err := env.eng.PlanAdopt(context.Background(), "plan", "claude-code"); err != nil {
		t.Fatalf("PlanAdopt(claude-code) error = %v", err)
	}
	if after, _ := os.ReadFile(filepath.Join(env.store.Root, a.Path)); string(after) != string(before) {
		t.Error("PlanAdopt wrote to the store")
	}
}

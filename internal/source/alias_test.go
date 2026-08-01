package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func TestAdd_AliasImportsUnderLocalNameAndRecordsUpstreamName(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("plan", "upstream plan v1")
	u.commit("initial")

	m := newManager(t)
	// The user already owns "plan" — the alias avoids the collision.
	if _, err := m.Store.New(domain.KindSkill, "plan"); err != nil {
		t.Fatal(err)
	}
	res, err := m.Add(context.Background(), "src", u.dir, "main", "", map[string]string{"plan": "team-plan"})
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if len(res.Added) != 1 || res.Added[0] != "team-plan" {
		t.Fatalf("Added = %v, want [team-plan]", res.Added)
	}
	art, ok := m.Store.Artifact("team-plan")
	if !ok {
		t.Fatal("aliased artifact not in manifest")
	}
	if art.SourceName != "plan" {
		t.Errorf("SourceName = %q, want plan", art.SourceName)
	}
	if art.SourceHash == "" {
		t.Error("aliased import must record a baseline hash")
	}
	// The user's own "plan" is untouched.
	own, _ := m.Store.Artifact("plan")
	if own.Origin != "local" {
		t.Errorf("user's artifact origin = %q, want local", own.Origin)
	}
}

func TestAdd_AliasForUnknownUpstreamSkillErrors(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("plan", "v1")
	u.commit("initial")

	m := newManager(t)
	_, err := m.Add(context.Background(), "src", u.dir, "main", "", map[string]string{"ghost": "team-ghost"})
	if err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("alias for missing upstream skill should error, got %v", err)
	}
}

func TestUpdate_AliasedSkillTracksRealUpstreamSkill(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("plan", "upstream plan v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Store.New(domain.KindSkill, "plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", map[string]string{"plan": "team-plan"}); err != nil {
		t.Fatal(err)
	}

	// Upstream reworks the REAL skill; the alias must be classified, the
	// user's identically-named local skill must not.
	u.writeSkill("plan", "upstream plan v2")
	u.commit("rework plan")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changed) != 1 || plan.Changed[0].Name != "plan" || plan.Changed[0].LocalID != "team-plan" {
		t.Fatalf("Changed = %+v, want upstream plan mapped to team-plan", plan.Changed)
	}
	if len(plan.OwnedElsewhere) != 0 {
		t.Errorf("aliased skill must not be reported owned-elsewhere: %v", plan.OwnedElsewhere)
	}

	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "team-plan", "SKILL.md"))
	if !strings.Contains(string(raw), "upstream plan v2") {
		t.Errorf("aliased artifact not refreshed:\n%s", raw)
	}
	own, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "plan", "SKILL.md"))
	if strings.Contains(string(own), "upstream") {
		t.Error("user's own skill was overwritten via the alias")
	}
}

func TestUpdate_AliasedSkillLocalEditsAreProtected(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("plan", "upstream v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", map[string]string{"plan": "team-plan"}); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "team-plan", "my tweaks")
	u.writeSkill("plan", "upstream v2")
	u.commit("rework")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].LocalID != "team-plan" {
		t.Fatalf("Conflicts = %+v, want the aliased skill", plan.Conflicts)
	}
	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "team-plan", "SKILL.md"))
	if string(raw) != "my tweaks" {
		t.Errorf("aliased local edit clobbered without force: %s", raw)
	}
}

func TestImportSkill_AliasesPreviouslySkippedSkill(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "upstream review")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Store.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	if res, err := m.Add(context.Background(), "src", u.dir, "main", "", nil); err != nil || len(res.Skipped) != 1 {
		t.Fatalf("setup: skipped = %v, err %v", res, err)
	}

	art, err := m.ImportSkill(context.Background(), "src", "review", "team-review")
	if err != nil {
		t.Fatalf("ImportSkill() error = %v", err)
	}
	if art.ID != "team-review" || art.SourceName != "review" || art.SourceHash == "" {
		t.Errorf("artifact = %+v, want aliased with baseline", art)
	}

	// Errors: taken local name, unknown upstream skill, unknown source.
	if _, err := m.ImportSkill(context.Background(), "src", "review", "team-review"); err == nil {
		t.Error("duplicate local name must error")
	}
	if _, err := m.ImportSkill(context.Background(), "src", "ghost", "x"); err == nil {
		t.Error("unknown upstream skill must error")
	}
	if _, err := m.ImportSkill(context.Background(), "nope", "review", "y"); err == nil {
		t.Error("unknown source must error")
	}
}

func TestUpstreamDiff_ResolvesAlias(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("plan", "upstream body")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", map[string]string{"plan": "team-plan"}); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "team-plan", "local body")

	text, err := m.UpstreamDiff(context.Background(), "team-plan")
	if err != nil {
		t.Fatalf("UpstreamDiff() error = %v", err)
	}
	if !strings.Contains(text, "-local body") || !strings.Contains(text, "+upstream body") {
		t.Errorf("aliased diff wrong:\n%s", text)
	}
	if !strings.Contains(text, "upstream/src/plan/") {
		t.Errorf("diff should reference the REAL upstream skill name:\n%s", text)
	}
}

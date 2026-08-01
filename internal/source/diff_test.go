package source

import (
	"context"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func TestUpstreamDiff_ShowsStoreVsUpstream(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "upstream body")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "alpha", "my local body")

	text, err := m.UpstreamDiff(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("UpstreamDiff() error = %v", err)
	}
	if !strings.Contains(text, "-my local body") || !strings.Contains(text, "+upstream body") {
		t.Errorf("diff should show local vs upstream lines:\n%s", text)
	}
	if !strings.Contains(text, "store/") || !strings.Contains(text, "upstream/") {
		t.Errorf("diff headers should label the two sides:\n%s", text)
	}
}

func TestUpstreamDiff_IdenticalContentYieldsEmpty(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("alpha", "same")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	text, err := m.UpstreamDiff(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if text != "" {
		t.Errorf("identical skill should produce no diff, got:\n%s", text)
	}
}

func TestUpstreamDiff_RejectsNonSourceArtifacts(t *testing.T) {
	m := newManager(t)
	if _, err := m.Store.New(domain.KindSkill, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.UpstreamDiff(context.Background(), "mine"); err == nil {
		t.Error("diff --upstream on a local artifact must error")
	}
	if _, err := m.UpstreamDiff(context.Background(), "ghost"); err == nil {
		t.Error("diff --upstream on a missing artifact must error")
	}
}

func TestSkillStates_OkEditedLegacy(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("pristine", "a")
	u.writeSkill("tweaked", "b")
	u.writeSkill("old-import", "c")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", ""); err != nil {
		t.Fatal(err)
	}
	editStoreSkill(t, m, "tweaked", "user changed this")
	clearSourceHash(t, m, "old-import")

	states, err := m.SkillStates("src")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]SkillState{"pristine": SkillOK, "tweaked": SkillEdited, "old-import": SkillLegacy}
	for id, st := range want {
		if states[id] != st {
			t.Errorf("state[%s] = %s, want %s", id, states[id], st)
		}
	}
}

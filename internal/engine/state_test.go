package engine

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestState_RemoveGroupDropsEmptyArtifactEntirely(t *testing.T) {
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	st.setGroup("demo", "claude-code", map[string]string{"skills/demo/SKILL.md": "hash1"})
	st.setGroup("demo", "codex", map[string]string{"skills/demo/SKILL.md": "hash1"})

	st.removeGroup("demo", "claude-code")
	if _, ok := st.Artifacts["demo"]["claude-code"]; ok {
		t.Error("claude-code record should be gone")
	}
	if _, ok := st.Artifacts["demo"]["codex"]; !ok {
		t.Error("codex record should be untouched")
	}

	st.removeGroup("demo", "codex")
	if _, ok := st.Artifacts["demo"]; ok {
		t.Error("artifact key should be pruned once no agents remain")
	}
}

func TestState_RemoveGroupOnUnknownArtifactIsSafeNoop(t *testing.T) {
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	st.removeGroup("nope", "claude-code") // must not panic
	if len(st.Artifacts) != 0 {
		t.Errorf("Artifacts = %v, want empty", st.Artifacts)
	}
}

func TestState_ArtifactIDsSortedAndDeduped(t *testing.T) {
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	st.setGroup("zeta", "claude-code", map[string]string{"a": "h"})
	st.setGroup("alpha", "claude-code", map[string]string{"a": "h"})
	st.setGroup("alpha", "codex", map[string]string{"a": "h"})

	got := st.artifactIDs()
	want := []string{"alpha", "zeta"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("artifactIDs() = %v, want %v", got, want)
	}
}

func TestState_ArtifactIDsExcludesEmptyAgentMaps(t *testing.T) {
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	st.setGroup("demo", "claude-code", map[string]string{"a": "h"})
	st.removeGroup("demo", "claude-code") // prunes the artifact key entirely

	if got := st.artifactIDs(); len(got) != 0 {
		t.Errorf("artifactIDs() = %v, want empty after removeGroup emptied it", got)
	}
}

func TestEngine_InstalledArtifactIDs(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.yaml")
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	st.setGroup("demo", "claude-code", map[string]string{"skills/demo/SKILL.md": "hash1"})
	if err := st.save(statePath); err != nil {
		t.Fatal(err)
	}

	e := &Engine{StatePath: statePath}
	got, err := e.InstalledArtifactIDs()
	if err != nil {
		t.Fatalf("InstalledArtifactIDs() error = %v", err)
	}
	if len(got) != 1 || got[0] != "demo" {
		t.Errorf("InstalledArtifactIDs() = %v, want [demo]", got)
	}
}

func TestEngine_InstalledArtifactIDs_EmptyWhenStateAbsent(t *testing.T) {
	e := &Engine{StatePath: filepath.Join(t.TempDir(), "state.yaml")}
	got, err := e.InstalledArtifactIDs()
	if err != nil {
		t.Fatalf("InstalledArtifactIDs() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("InstalledArtifactIDs() = %v, want empty", got)
	}
}

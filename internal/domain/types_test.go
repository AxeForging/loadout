package domain

import (
	"encoding/json"
	"testing"
)

// TestArtifact_JSONUsesLowercaseKeys guards against a regression where
// Artifact only carried yaml tags: the web API serializes artifacts straight
// to JSON for the Store page, and without json tags Go falls back to the
// exported field names (ID, Kind, ...), leaving the frontend's a.id/a.kind
// lookups undefined.
func TestArtifact_JSONUsesLowercaseKeys(t *testing.T) {
	raw, err := json.Marshal(Artifact{ID: "core", Kind: KindInstruction, Path: "instructions/core.md"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"id", "kind", "path"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected JSON key %q, got %s", key, raw)
		}
	}
}

// TestSourceRef_JSONUsesLowercaseKeys is the same regression guard for the
// Sources page, which reads a.name/a.url/a.commit from /api/sources.
func TestSourceRef_JSONUsesLowercaseKeys(t *testing.T) {
	raw, err := json.Marshal(SourceRef{Name: "team-skills", URL: "git@github.com:acme/team-skills.git", Commit: "abc123"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, key := range []string{"name", "url", "commit"} {
		if _, ok := got[key]; !ok {
			t.Errorf("expected JSON key %q, got %s", key, raw)
		}
	}
}

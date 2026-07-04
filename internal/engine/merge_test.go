package engine

import (
	"encoding/json"
	"testing"
)

func TestJSONMerge_MergesNestedMapsWithFragmentScalarsWinning(t *testing.T) {
	existing := []byte(`{"env": {"KEEP": "yes", "OVERRIDE": "old"}, "mine": true}`)
	fragment := []byte(`{"env": {"OVERRIDE": "new", "ADDED": "1"}}`)

	out, err := jsonMerge(existing, fragment)
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	env := got["env"].(map[string]any)
	if env["KEEP"] != "yes" || env["OVERRIDE"] != "new" || env["ADDED"] != "1" {
		t.Errorf("env merged wrong: %v", env)
	}
	if got["mine"] != true {
		t.Error("unrelated user key lost")
	}
}

func TestJSONMerge_UnionsArraysWithoutDuplicating(t *testing.T) {
	existing := []byte(`{"hooks": {"Stop": [{"matcher": "a"}]}}`)
	fragment := []byte(`{"hooks": {"Stop": [{"matcher": "a"}, {"matcher": "b"}]}}`)

	out, err := jsonMerge(existing, fragment)
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	stop := got["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 2 {
		t.Errorf("Stop entries = %d, want 2 (no duplicate of a): %v", len(stop), stop)
	}
}

func TestJSONMerge_SemanticNoopKeepsExistingBytesUntouched(t *testing.T) {
	// User formatting (4-space indent) must not cause phantom drift.
	existing := []byte("{\n    \"env\": {\n        \"FOO\": \"bar\"\n    }\n}\n")
	fragment := []byte(`{"env": {"FOO": "bar"}}`)

	out, err := jsonMerge(existing, fragment)
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}
	if string(out) != string(existing) {
		t.Errorf("no-op merge rewrote the file:\n%s", out)
	}
}

func TestJSONMerge_EmptyExistingProducesFragmentDocument(t *testing.T) {
	out, err := jsonMerge(nil, []byte(`{"a": 1}`))
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if got["a"] != float64(1) {
		t.Errorf("got %v", got)
	}
}

func TestJSONMerge_FailsOnCorruptExistingFile(t *testing.T) {
	if _, err := jsonMerge([]byte("{not json"), []byte(`{}`)); err == nil {
		t.Fatal("expected error for invalid existing JSON")
	}
}

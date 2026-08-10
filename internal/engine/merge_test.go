package engine

import (
	"encoding/json"
	"fmt"
	"strings"
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

// Would have caught the "1094 lines removed" phantom-diff bug: MCP install
// used to run the whole ~/.claude.json through json.MarshalIndent, so every
// unrelated key of Claude Code's own state (userID, numStartups, projects…)
// moved lines and every diff looked catastrophic. The write must only touch
// the mcpServers subtree; unrelated key order and formatting stay verbatim.
func TestJSONMerge_PreservesUnrelatedKeyOrderAndFormatting(t *testing.T) {
	existing := []byte(`{
  "numStartups": 1094,
  "userID": "abc",
  "projects": {"~/x": {"allowedTools": []}},
  "mcpServers": {
    "playwright": {"type": "stdio", "command": "npx"}
  }
}
`)
	fragment := []byte(`{"mcpServers": {"context7": {"type": "stdio", "command": "npx"}}}`)

	out, err := jsonMerge(existing, fragment)
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}
	s := string(out)

	// Unrelated keys keep their original insertion order (numStartups → userID
	// → projects) before the mcpServers subtree loadout manages.
	idxStartups := strings.Index(s, `"numStartups"`)
	idxUser := strings.Index(s, `"userID"`)
	idxProjects := strings.Index(s, `"projects"`)
	idxMCP := strings.Index(s, `"mcpServers"`)
	if idxStartups >= idxUser || idxUser >= idxProjects || idxProjects >= idxMCP {
		t.Errorf("unrelated keys reordered — old bug regression:\n%s", s)
	}

	// Existing sibling in the touched subtree is preserved.
	if !strings.Contains(s, `"playwright"`) || !strings.Contains(s, `"context7"`) {
		t.Errorf("mcpServers merge lost or missed entries:\n%s", s)
	}

	// Byte-level: unrelated top-level values are byte-identical, not just
	// semantically equal. Catches any accidental re-serialization.
	for _, needle := range []string{`"numStartups": 1094`, `"userID": "abc"`, `"projects": {"~/x": {"allowedTools": []}}`} {
		if !strings.Contains(s, needle) {
			t.Errorf("expected byte-identical %q in output, got:\n%s", needle, s)
		}
	}
}

// The diff loadout shows before apply must match what apply actually writes.
// The regression here was: MarshalIndent reordered keys, so the diff looked
// like a full-file rewrite when apply was semantically a one-key insertion.
func TestJSONMerge_DiffIsProportionalToTheActualChange(t *testing.T) {
	var b strings.Builder
	b.WriteString("{\n")
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&b, "  \"userKey%d\": %d,\n", i, i)
	}
	b.WriteString("  \"mcpServers\": {\"playwright\": {\"type\": \"stdio\", \"command\": \"npx\"}}\n}\n")
	existing := []byte(b.String())
	fragment := []byte(`{"mcpServers": {"context7": {"type": "stdio", "command": "npx"}}}`)

	out, err := jsonMerge(existing, fragment)
	if err != nil {
		t.Fatalf("jsonMerge() error = %v", err)
	}

	// The user's 50 unrelated lines must survive byte-for-byte at the top of
	// the file — otherwise the pre-apply diff misleads about the change.
	for i := 0; i < 50; i++ {
		needle := fmt.Sprintf("\"userKey%d\": %d", i, i)
		if !strings.Contains(string(out), needle) {
			t.Errorf("unrelated user line %q missing after merge — output would produce a huge, misleading diff", needle)
		}
	}
}

func TestJSONMerge_FailsOnCorruptExistingFile(t *testing.T) {
	if _, err := jsonMerge([]byte("{not json"), []byte(`{}`)); err == nil {
		t.Fatal("expected error for invalid existing JSON")
	}
}

func TestJSONRemove_DeletesExactMatchKeyPreservesSiblingBytes(t *testing.T) {
	existing := []byte("{\n    \"mine\": true,\n    \"managed\": {\"a\": 1}\n}\n")
	fragment := []byte(`{"managed": {"a": 1}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if !changed || len(drifted) != 0 {
		t.Fatalf("changed=%v drifted=%v, want changed with no drift", changed, drifted)
	}
	if strings.Contains(string(out), "managed") {
		t.Errorf("managed key not removed:\n%s", out)
	}
	if !strings.Contains(string(out), "\n    \"mine\": true") {
		t.Errorf("sibling bytes not preserved:\n%s", out)
	}
}

func TestJSONRemove_RemovesOneServerEntryKeepsUserServers(t *testing.T) {
	existing := []byte(`{"mcpServers": {"user-own": {"command": "deno"}, "gh": {"command": "npx", "args": ["-y"]}}}`)
	fragment := []byte(`{"mcpServers": {"gh": {"command": "npx", "args": ["-y"]}}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if !changed || len(drifted) != 0 {
		t.Fatalf("changed=%v drifted=%v, want clean removal", changed, drifted)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	servers := got["mcpServers"].(map[string]any)
	if _, gone := servers["gh"]; gone {
		t.Errorf("gh entry survived: %v", servers)
	}
	if _, kept := servers["user-own"]; !kept {
		t.Errorf("user's own server lost: %v", servers)
	}
}

func TestJSONRemove_PrunesEmptiedParentObjects(t *testing.T) {
	existing := []byte(`{"model": "opus", "hooks": {"Stop": [{"matcher": "a"}]}}`)
	fragment := []byte(`{"hooks": {"Stop": [{"matcher": "a"}]}}`)

	out, changed, _, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if !changed {
		t.Fatal("want a change")
	}
	if strings.Contains(string(out), "hooks") {
		t.Errorf("emptied hooks object not pruned:\n%s", out)
	}
	if !strings.Contains(string(out), `"model": "opus"`) {
		t.Errorf("user key lost:\n%s", out)
	}
}

func TestJSONRemove_ArrayRemovesOnlyInstalledItemsKeepsUserItems(t *testing.T) {
	existing := []byte(`{"hooks": {"Stop": [{"matcher": "user"}, {"matcher": "ours"}]}}`)
	fragment := []byte(`{"hooks": {"Stop": [{"matcher": "ours"}]}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if !changed || len(drifted) != 0 {
		t.Fatalf("changed=%v drifted=%v, want clean partial removal", changed, drifted)
	}
	var got map[string]any
	json.Unmarshal(out, &got)
	stop := got["hooks"].(map[string]any)["Stop"].([]any)
	if len(stop) != 1 || stop[0].(map[string]any)["matcher"] != "user" {
		t.Errorf("Stop = %v, want only the user's entry", stop)
	}
}

func TestJSONRemove_EditedArrayItemReportsDriftChangesNothing(t *testing.T) {
	existing := []byte(`{"hooks": {"Stop": [{"matcher": "edited-by-user"}]}}`)
	fragment := []byte(`{"hooks": {"Stop": [{"matcher": "ours"}]}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if changed {
		t.Errorf("edited array item must not be touched:\n%s", out)
	}
	if len(drifted) != 1 || drifted[0] != "hooks.Stop" {
		t.Errorf("drifted = %v, want [hooks.Stop]", drifted)
	}
	if string(out) != string(existing) {
		t.Error("bytes must be identical when nothing changed")
	}
}

func TestJSONRemove_DriftRollsBackSiblingRemovalsInSameObject(t *testing.T) {
	// User edited "command" inside our server entry: the whole entry must stay
	// intact — not get gutted down to just the edited key.
	existing := []byte(`{"mcpServers": {"gh": {"command": "my-custom-wrapper", "args": ["-y"]}}}`)
	fragment := []byte(`{"mcpServers": {"gh": {"command": "npx", "args": ["-y"]}}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if changed {
		t.Errorf("partially-edited entry must be left whole:\n%s", out)
	}
	if len(drifted) != 1 || drifted[0] != "mcpServers.gh.command" {
		t.Errorf("drifted = %v, want [mcpServers.gh.command]", drifted)
	}
	if !strings.Contains(string(out), `"args"`) {
		t.Errorf("sibling key was removed despite drift:\n%s", out)
	}
}

func TestJSONRemove_ForceRemovesDriftedValuesAndPrunes(t *testing.T) {
	existing := []byte(`{"keep": 1, "mcpServers": {"gh": {"command": "my-custom-wrapper", "args": ["-y"]}}}`)
	fragment := []byte(`{"mcpServers": {"gh": {"command": "npx", "args": ["-y"]}}}`)

	out, changed, _, err := jsonRemove(existing, fragment, true)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if !changed {
		t.Fatal("force must remove the drifted entry")
	}
	if strings.Contains(string(out), "mcpServers") {
		t.Errorf("drifted entry (and emptied parent) not removed under force:\n%s", out)
	}
	if !strings.Contains(string(out), `"keep": 1`) {
		t.Errorf("user key lost under force:\n%s", out)
	}
}

func TestJSONRemove_FragmentAbsentIsNoopWithIdenticalBytes(t *testing.T) {
	existing := []byte("{\n  \"only\": \"user stuff\"\n}\n")
	fragment := []byte(`{"mcpServers": {"gh": {"command": "npx"}}}`)

	out, changed, drifted, err := jsonRemove(existing, fragment, false)
	if err != nil {
		t.Fatalf("jsonRemove() error = %v", err)
	}
	if changed || len(drifted) != 0 {
		t.Errorf("changed=%v drifted=%v, want pure no-op", changed, drifted)
	}
	if string(out) != string(existing) {
		t.Error("no-op must return the input bytes untouched")
	}
}

func TestJSONRemove_InvalidExistingFileErrorsLoudly(t *testing.T) {
	if _, _, _, err := jsonRemove([]byte("{broken"), []byte(`{"a": 1}`), false); err == nil {
		t.Fatal("want an error for a corrupt existing file")
	}
}

func TestJSONRemove_EmptyExistingIsNoop(t *testing.T) {
	out, changed, _, err := jsonRemove(nil, []byte(`{"a": 1}`), false)
	if err != nil || changed || len(out) != 0 {
		t.Fatalf("out=%q changed=%v err=%v, want empty no-op", out, changed, err)
	}
}

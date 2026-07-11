package sdd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig_MissingFileDefaultsToLocal(t *testing.T) {
	cfg, err := LoadConfig(t.TempDir())
	if err != nil || cfg.Backend != BackendLocal {
		t.Fatalf("cfg = %+v, err = %v", cfg, err)
	}
}

func TestConfig_RoundTripAndUnknownBackendRejected(t *testing.T) {
	dir := t.TempDir()
	in := Config{Backend: BackendGitHub, Repo: "AxeForging/loadout", Labels: []string{"spec"}, Issues: map[string]int{"001-demo": 42}}
	if err := in.Save(dir); err != nil {
		t.Fatal(err)
	}
	out, err := LoadConfig(dir)
	if err != nil || out.Repo != in.Repo || out.Issues["001-demo"] != 42 || len(out.Labels) != 1 {
		t.Fatalf("round trip lost data: %+v, err %v", out, err)
	}

	if err := os.WriteFile(filepath.Join(dir, ConfigFile), []byte("backend: jira\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(dir); err == nil {
		t.Fatal("unknown backend must be rejected")
	}
}

func TestCreateSpec_NumbersSequentiallyAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	first, err := CreateSpec(dir, "web-import")
	if err != nil || first.Name != "001-web-import" {
		t.Fatalf("first = %+v, err %v", first, err)
	}
	second, err := CreateSpec(dir, "mcp-configs")
	if err != nil || second.Number != 2 {
		t.Fatalf("second = %+v, err %v", second, err)
	}
	if _, err := CreateSpec(dir, "web-import"); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("re-create must refuse: %v", err)
	}
	raw, _ := os.ReadFile(first.Path)
	for _, section := range []string{"## Context", "## Requirements", "## Test plan", "## Rollout", "<!-- sdd:review -->"} {
		if !strings.Contains(string(raw), section) {
			t.Errorf("template missing %s", section)
		}
	}
	if _, err := ParseSteps(raw); err != nil {
		t.Errorf("template steps must parse: %v", err)
	}
}

func TestListSpecs_IgnoresPlansAndForeignFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"001-alpha.md", "001-alpha.plan.md", "002-beta.md", "notes.md", "sdd.yaml"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	specs, err := ListSpecs(dir, Config{})
	if err != nil || len(specs) != 2 {
		t.Fatalf("specs = %+v, err %v", specs, err)
	}
	if specs[0].Name != "001-alpha" || specs[1].Name != "002-beta" {
		t.Errorf("order/names wrong: %+v", specs)
	}
}

func TestResolve_ByNumberPrefixAndAmbiguity(t *testing.T) {
	specs := []SpecInfo{
		{Number: 1, Name: "001-web-import"},
		{Number: 2, Name: "002-web-export"},
		{Number: 7, Name: "007-sdd-workflow"},
	}
	if s, err := Resolve(specs, "7"); err != nil || s.Name != "007-sdd-workflow" {
		t.Errorf("by number: %+v, %v", s, err)
	}
	if s, err := Resolve(specs, "sdd"); err != nil || s.Number != 7 {
		t.Errorf("by fragment: %+v, %v", s, err)
	}
	if _, err := Resolve(specs, "web"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous fragment must error: %v", err)
	}
	if _, err := Resolve(specs, "99"); err == nil {
		t.Error("unknown number must error")
	}
}

func TestLocalBackend_SetStepPersistsAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	spec, err := CreateSpec(dir, "demo")
	if err != nil {
		t.Fatal(err)
	}
	b := &LocalBackend{Dir: dir}
	ctx := context.Background()

	changed, err := b.SetStep(ctx, spec, "implement", true)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	changed, err = b.SetStep(ctx, spec, "implement", true)
	if err != nil || changed {
		t.Fatalf("second check must be a no-op: changed=%v err=%v", changed, err)
	}
	steps, err := b.Steps(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	var done int
	for _, s := range steps {
		if s.Done {
			done++
		}
	}
	if done != 1 {
		t.Errorf("steps = %+v, want exactly one done", steps)
	}
}

func TestParseRepoURL_SSHAndHTTPSForms(t *testing.T) {
	cases := map[string]string{
		"git@github.com:AxeForging/loadout.git":     "AxeForging/loadout",
		"https://github.com/AxeForging/loadout.git": "AxeForging/loadout",
		"https://github.com/AxeForging/loadout":     "AxeForging/loadout",
		"ssh://git@github.com/AxeForging/loadout":   "AxeForging/loadout",
	}
	for url, want := range cases {
		got, err := ParseRepoURL(url)
		if err != nil || got != want {
			t.Errorf("ParseRepoURL(%q) = %q, %v; want %q", url, got, err, want)
		}
	}
	if _, err := ParseRepoURL("not-a-url"); err == nil {
		t.Error("garbage url must error")
	}
}

func TestCompare_ReportsStructureAndStateDivergence(t *testing.T) {
	local := []Step{{ID: "a", Text: "one", Done: true}, {ID: "b", Text: "two", Done: false}}
	same := []Step{{ID: "a", Text: "one", Done: true}, {ID: "b", Text: "two", Done: false}}
	if rep := Compare(local, same); !rep.InSync() {
		t.Errorf("identical lists must be in sync: %+v", rep)
	}

	stateDiff := []Step{{ID: "a", Text: "one", Done: false}, {ID: "b", Text: "two", Done: true}}
	rep := Compare(local, stateDiff)
	if rep.StructureChanged || len(rep.StateDiffs) != 2 {
		t.Errorf("state divergence: %+v", rep)
	}

	reworded := []Step{{ID: "a", Text: "one reworded", Done: true}, {ID: "b", Text: "two", Done: false}}
	if rep := Compare(local, reworded); !rep.StructureChanged {
		t.Error("text change must count as structure change")
	}

	missing := []Step{{ID: "a", Text: "one", Done: true}}
	if rep := Compare(local, missing); !rep.StructureChanged {
		t.Error("missing remote step must count as structure change")
	}
}

func TestFindProjectRoot_WalksUpToGitDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FindProjectRoot(nested)
	if err != nil || got != root {
		t.Errorf("FindProjectRoot = %q, %v; want %q", got, err, root)
	}
}

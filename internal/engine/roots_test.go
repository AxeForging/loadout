package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
)

// addMCP writes an mcp artifact with the given scope into the test store.
func (env *testEnv) addMCP(t *testing.T, name, scope string) domain.Artifact {
	t.Helper()
	a, err := env.store.New(domain.KindMCP, name)
	if err != nil {
		t.Fatalf("New(mcp) error = %v", err)
	}
	spec := "command: npx\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\nscope: " + scope + "\n"
	if err := os.WriteFile(filepath.Join(env.store.Root, a.Path, "mcp.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPlan_MCPUserScope_TargetsUserHomeClaudeJSON(t *testing.T) {
	env := newTestEnv(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	env.addMCP(t, "github", "user")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	changes := plan.Changes()
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %v", changes)
	}
	it := changes[0]
	if it.AbsPath != filepath.Join(userHome, ".claude.json") {
		t.Errorf("AbsPath = %s, want under user home %s", it.AbsPath, userHome)
	}
	if it.Root != adapter.RootUserHome || it.StateKey != "user-home::.claude.json" {
		t.Errorf("Root = %q, StateKey = %q", it.Root, it.StateKey)
	}
}

func TestApply_MCPUserScope_WritesAndRecordsRootQualifiedState(t *testing.T) {
	env := newTestEnv(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	env.addMCP(t, "github", "user")
	env.applyAll(t)

	raw, err := os.ReadFile(filepath.Join(userHome, ".claude.json"))
	if err != nil {
		t.Fatalf(".claude.json not written: %v", err)
	}
	if !strings.Contains(string(raw), `"${GITHUB_TOKEN}"`) {
		t.Errorf("placeholder not written verbatim:\n%s", raw)
	}

	st, err := loadState(env.eng.StatePath)
	if err != nil {
		t.Fatal(err)
	}
	if st.get("github", "claude-code", "user-home::.claude.json") == "" {
		t.Errorf("state missing root-qualified key; state = %+v", st.Artifacts)
	}
	// Second plan: everything in sync, no phantom drift.
	if got := env.statusOf(t, "github"); got != domain.StateInSync {
		t.Errorf("status after apply = %s, want in-sync", got)
	}
}

func TestApply_MCPUserScope_PreservesForeignEntriesAndUserState(t *testing.T) {
	env := newTestEnv(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	existing := `{"numStartups": 42, "mcpServers": {"mine": {"command": "deno"}}}`
	if err := os.WriteFile(filepath.Join(userHome, ".claude.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	env.addMCP(t, "github", "user")
	env.applyAll(t)

	raw, _ := os.ReadFile(filepath.Join(userHome, ".claude.json"))
	for _, want := range []string{`"numStartups"`, `"mine"`, `"github"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("merged .claude.json lost %s:\n%s", want, raw)
		}
	}
}

func TestPlan_MCPProjectScope_WithoutProjectRootIsNotApplicable(t *testing.T) {
	env := newTestEnv(t)
	env.addMCP(t, "search", "project")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Changes()) != 0 {
		t.Fatalf("project target without root must not plan writes: %v", plan.Changes())
	}
	found := false
	for _, na := range plan.NotApplicable {
		if na.Artifact == "search" {
			found = true
		}
	}
	if !found {
		t.Error("project-scoped artifact should be reported not applicable without a project root")
	}
}

func TestApply_MCPProjectScope_WritesIntoProjectRoot(t *testing.T) {
	env := newTestEnv(t)
	project := t.TempDir()
	env.eng.ProjectRoot = project
	env.addMCP(t, "search", "project")
	env.applyAll(t)

	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); err != nil {
		t.Fatalf(".mcp.json not written into project root: %v", err)
	}
	st, _ := loadState(env.eng.StatePath)
	key := "project:" + project + "::.mcp.json"
	if st.get("search", "claude-code", key) == "" {
		t.Errorf("state missing project-qualified key %q; state = %+v", key, st.Artifacts)
	}
}

func TestApply_NonAgentHomeRoots_WriteNothingWithoutConfirmation(t *testing.T) {
	env := newTestEnv(t)
	userHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", userHome)
	env.eng.ProjectRoot = project
	env.addMCP(t, "github", "user")
	env.addMCP(t, "search", "project")

	plan, err := env.eng.Plan(context.Background(), Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.eng.Apply(context.Background(), plan, nil); err != ErrNotConfirmed {
		t.Fatalf("Apply(nil) error = %v, want ErrNotConfirmed", err)
	}
	if _, err := env.eng.Apply(context.Background(), plan, func(*Plan) bool { return false }); err != ErrNotConfirmed {
		t.Fatalf("Apply(decline) error = %v, want ErrNotConfirmed", err)
	}
	if _, err := os.Stat(filepath.Join(userHome, ".claude.json")); !os.IsNotExist(err) {
		t.Fatal("user home was written to without confirmation")
	}
	if _, err := os.Stat(filepath.Join(project, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("project root was written to without confirmation")
	}
	if _, err := os.Stat(env.eng.StatePath); !os.IsNotExist(err) {
		t.Fatal("state file was written without confirmation")
	}
}

func TestStatus_MCPHandEditedEntryIsDriftedLocal(t *testing.T) {
	env := newTestEnv(t)
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	env.addMCP(t, "github", "user")
	env.applyAll(t)

	path := filepath.Join(userHome, ".claude.json")
	raw, _ := os.ReadFile(path)
	edited := strings.Replace(string(raw), "npx", "bunx", 1)
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := env.statusOf(t, "github"); got != domain.StateDriftedLocal {
		t.Errorf("status after hand edit = %s, want drifted-local", got)
	}
}

func TestJoinUnder_RejectsEscapingAndAbsolutePaths(t *testing.T) {
	base := t.TempDir()
	for _, rel := range []string{"../evil", "..", "/etc/passwd", ""} {
		if _, err := joinUnder(base, rel); err == nil {
			t.Errorf("joinUnder(%q) should be rejected", rel)
		}
	}
	if abs, err := joinUnder(base, "a/b.json"); err != nil || abs != filepath.Join(base, "a", "b.json") {
		t.Errorf("joinUnder(a/b.json) = %s, %v", abs, err)
	}
}

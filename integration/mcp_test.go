//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// storePath returns a path inside the default store of this env's fake HOME.
func (e *env) storePath(parts ...string) string {
	return filepath.Join(append([]string{e.home, ".local", "share", "loadout", "store"}, parts...)...)
}

func (e *env) writeStoreMCP(t *testing.T, name, spec string) {
	t.Helper()
	if err := os.WriteFile(e.storePath("mcps", name, "mcp.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMCP_UserScope_SyncWritesEveryAgentNativeConfig(t *testing.T) {
	e := newEnv(t)
	// Fake homes so gemini-cli and cursor detect as installed too.
	for _, dir := range []string{".gemini", ".cursor"} {
		if err := os.MkdirAll(filepath.Join(e.home, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	e.mustRun("init")
	e.mustRun("new", "mcp", "github")
	e.writeStoreMCP(t, "github",
		"command: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-github\"]\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\nscope: user\n")

	e.mustRun("sync", "--yes")

	// claude-code: ~/.claude.json — a SIBLING of ~/.claude, the first file
	// loadout manages outside an agent home.
	claudeJSON, err := os.ReadFile(filepath.Join(e.home, ".claude.json"))
	if err != nil {
		t.Fatalf("~/.claude.json not written: %v", err)
	}
	for _, want := range []string{`"github"`, `"npx"`, `"${GITHUB_TOKEN}"`} {
		if !strings.Contains(string(claudeJSON), want) {
			t.Errorf("~/.claude.json missing %s:\n%s", want, claudeJSON)
		}
	}

	gemini, err := os.ReadFile(filepath.Join(e.home, ".gemini", "settings.json"))
	if err != nil || !strings.Contains(string(gemini), `"mcpServers"`) {
		t.Errorf("gemini settings.json missing mcpServers (err %v):\n%s", err, gemini)
	}
	cursor, err := os.ReadFile(filepath.Join(e.home, ".cursor", "mcp.json"))
	if err != nil || !strings.Contains(string(cursor), `"github"`) {
		t.Errorf("cursor mcp.json missing entry (err %v):\n%s", err, cursor)
	}

	out := e.mustRun("status")
	if !strings.Contains(out, "in-sync") {
		t.Fatalf("status after sync:\n%s", out)
	}
}

func TestMCP_Invariant_NoWriteWithoutConfirmationOutsideAgentHomes(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "mcp", "github")
	project := t.TempDir()

	// EOF stdin and explicit "n" must leave user home and project untouched.
	if out, err := e.run("", "sync", "--project", project); err != nil {
		t.Fatalf("sync with EOF stdin errored: %v\n%s", err, out)
	}
	if out, err := e.run("n\n", "sync", "--project", project); err != nil {
		t.Fatalf("sync with 'n' errored: %v\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(e.home, ".claude.json")); !os.IsNotExist(err) {
		t.Fatal("~/.claude.json written without confirmation")
	}
	entries, _ := os.ReadDir(project)
	if len(entries) != 0 {
		t.Fatalf("project dir written without confirmation: %v", entries)
	}
}

func TestMCP_ProjectScope_RequiresProjectRootAndWritesDotMCPJSON(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "mcp", "search")
	e.writeStoreMCP(t, "search",
		"transport: http\nurl: https://mcp.example.com/v1\nscope: project\n")

	// Without a project root the artifact is n/a — sync must not fail, and
	// must not write anywhere.
	out := e.mustRun("sync", "--yes")
	if strings.Contains(out, ".mcp.json") {
		t.Fatalf("project-scoped artifact synced without a project root:\n%s", out)
	}

	project := t.TempDir()
	e.mustRun("sync", "--project", project, "--yes")
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil {
		t.Fatalf(".mcp.json not written: %v", err)
	}
	if !strings.Contains(string(raw), `"https://mcp.example.com/v1"`) {
		t.Errorf(".mcp.json content wrong:\n%s", raw)
	}

	// State lands in the machine-local data dir, never inside the project.
	entries, err := os.ReadDir(project)
	if err != nil {
		t.Fatal(err)
	}
	for _, en := range entries {
		if en.Name() != ".mcp.json" && en.Name() != ".cursor" && en.Name() != ".gemini" {
			t.Errorf("unexpected file in project dir: %s", en.Name())
		}
	}
	state, err := os.ReadFile(filepath.Join(e.home, ".local", "share", "loadout", "state.yaml"))
	if err != nil {
		t.Fatalf("state.yaml missing: %v", err)
	}
	if !strings.Contains(string(state), "project:"+project) {
		t.Errorf("state.yaml missing project-qualified record:\n%s", state)
	}
}

func TestMCP_Drift_HandEditIsDriftedLocalAndForeignEntriesSurvive(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	// A pre-existing user-managed MCP server must survive every sync.
	foreign := `{"numStartups": 7, "mcpServers": {"mine": {"command": "deno", "args": ["run", "server.ts"]}}}`
	if err := os.WriteFile(filepath.Join(e.home, ".claude.json"), []byte(foreign), 0o644); err != nil {
		t.Fatal(err)
	}

	e.mustRun("new", "mcp", "github")
	e.mustRun("sync", "--yes")

	path := filepath.Join(e.home, ".claude.json")
	raw, _ := os.ReadFile(path)
	for _, want := range []string{`"numStartups"`, `"mine"`, `"deno"`, `"github"`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("user content lost after sync — missing %s:\n%s", want, raw)
		}
	}

	// Hand-edit the managed entry: loadout must report drifted-local, and a
	// re-sync must restore it while STILL preserving the foreign entry.
	edited := strings.Replace(string(raw), `"npx"`, `"bunx"`, 1)
	if edited == string(raw) {
		t.Fatal("test setup: managed command not found to edit")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	out := e.mustRun("status")
	if !strings.Contains(out, "drifted-local") {
		t.Fatalf("status should show drifted-local after hand edit:\n%s", out)
	}
	e.mustRun("sync", "--yes")
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), `"npx"`) || !strings.Contains(string(raw), `"mine"`) {
		t.Fatalf("re-sync should restore managed entry and keep foreign one:\n%s", raw)
	}
}

func TestMCP_LiteralSecret_RefusedByNewEditDoctorAndSync(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "mcp", "github")
	e.writeStoreMCP(t, "github",
		"command: npx\nenv:\n  GITHUB_TOKEN: ghp_liveSecret123\n")

	out, err := e.run("", "doctor")
	if err == nil && !strings.Contains(out, "${") {
		t.Fatalf("doctor should flag the literal secret:\n%s", out)
	}

	out, err = e.run("", "sync", "--yes")
	if err == nil {
		t.Fatalf("sync must refuse to install a literal secret:\n%s", out)
	}
	if !strings.Contains(out, "${") {
		t.Errorf("sync error should point at placeholder usage:\n%s", out)
	}
	if _, statErr := os.Stat(filepath.Join(e.home, ".claude.json")); !os.IsNotExist(statErr) {
		t.Fatal("literal secret reached ~/.claude.json")
	}
}

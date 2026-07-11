//go:build integration

// Package integration drives the real loadout binary against a temp HOME with
// a fake ~/.claude, asserting the end-to-end promises: init → new → sync
// installs, drift is detected, and nothing is ever written without
// confirmation.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		binPath = filepath.Join(os.TempDir(), "loadout-integration-test")
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/loadout")
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			buildErr = err
			t.Logf("build output:\n%s", out)
		}
	})
	if buildErr != nil {
		t.Fatalf("build failed: %v", buildErr)
	}
	return binPath
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(wd)
}

// env is one isolated machine: temp HOME with a fake ~/.claude.
type env struct {
	t    *testing.T
	home string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, home: home}
}

func (e *env) claude(parts ...string) string {
	return filepath.Join(append([]string{e.home, ".claude"}, parts...)...)
}

// run executes loadout with stdin content and returns combined output.
func (e *env) run(stdin string, args ...string) (string, error) {
	e.t.Helper()
	return e.runIn("", "", stdin, args...)
}

// runIn executes loadout from dir (default: test cwd) with an optional
// directory prepended to PATH — used to put stub binaries (e.g. a fake gh)
// in front of the real ones.
func (e *env) runIn(dir, pathPrepend, stdin string, args ...string) (string, error) {
	e.t.Helper()
	cmd := exec.Command(binary(e.t), args...)
	cmd.Dir = dir
	environ := append(os.Environ(),
		"HOME="+e.home,
		"XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(e.home, ".local", "share"),
	)
	if pathPrepend != "" {
		environ = append(environ, "PATH="+pathPrepend+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd.Env = environ
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (e *env) mustRun(args ...string) string {
	e.t.Helper()
	out, err := e.run("", args...)
	if err != nil {
		e.t.Fatalf("loadout %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestHappyPath_InitNewSyncInstallsSkill(t *testing.T) {
	e := newEnv(t)

	out := e.mustRun("init")
	if !strings.Contains(out, "Initialized store") {
		t.Fatalf("init output: %s", out)
	}
	e.mustRun("new", "skill", "demo")

	out = e.mustRun("status")
	if !strings.Contains(out, "demo") || !strings.Contains(out, "missing") {
		t.Fatalf("status should show demo as missing:\n%s", out)
	}

	out = e.mustRun("sync", "--yes")
	if !strings.Contains(out, "Applied 1 change(s)") {
		t.Fatalf("sync output: %s", out)
	}

	raw, err := os.ReadFile(e.claude("skills", "demo", "SKILL.md"))
	if err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	if !strings.Contains(string(raw), "name: demo") {
		t.Errorf("installed SKILL.md wrong:\n%s", raw)
	}

	out = e.mustRun("status")
	if !strings.Contains(out, "in-sync") {
		t.Fatalf("status after sync:\n%s", out)
	}
}

// TestInvariant_NoWriteWithoutConfirmation is the product's core promise:
// without --yes or an interactive "y", agent homes must not change at all.
func TestInvariant_NoWriteWithoutConfirmation(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")

	before := snapshotDir(t, e.claude())

	// EOF on stdin (no answer), explicit "n", and dry-run must all be no-ops.
	if out, err := e.run("", "sync"); err != nil {
		t.Fatalf("sync with EOF stdin errored: %v\n%s", err, out)
	}
	if out, err := e.run("n\n", "sync"); err != nil {
		t.Fatalf("sync with 'n' errored: %v\n%s", err, out)
	}
	e.mustRun("sync", "--dry-run")

	after := snapshotDir(t, e.claude())
	if len(before) != len(after) {
		t.Fatalf("agent home changed without confirmation: before=%v after=%v", before, after)
	}
	for path, hash := range before {
		if after[path] != hash {
			t.Fatalf("agent home file %s changed without confirmation", path)
		}
	}
}

func TestSync_PreservesUserClaudeMDAroundManagedBlock(t *testing.T) {
	e := newEnv(t)
	userContent := "# my personal notes\n\nprecious user content\n"
	if err := os.WriteFile(e.claude("CLAUDE.md"), []byte(userContent), 0o644); err != nil {
		t.Fatal(err)
	}

	e.mustRun("init")
	e.mustRun("new", "instruction", "style")
	e.mustRun("sync", "--yes")

	raw, err := os.ReadFile(e.claude("CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.HasPrefix(s, userContent) {
		t.Errorf("user content not preserved byte-for-byte:\n%s", s)
	}
	if !strings.Contains(s, "<!-- loadout:begin style -->") {
		t.Errorf("managed block missing:\n%s", s)
	}
}

func TestStatus_ReportsLocalDriftAfterUserEditsInstalledFile(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "skill", "demo")
	e.mustRun("sync", "--yes")

	if err := os.WriteFile(e.claude("skills", "demo", "SKILL.md"), []byte("edited by user\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := e.mustRun("status")
	if !strings.Contains(out, "drifted-local") {
		t.Fatalf("status should report drifted-local:\n%s", out)
	}

	out = e.mustRun("diff")
	if !strings.Contains(out, "edited by user") {
		t.Fatalf("diff should show the local edit:\n%s", out)
	}
}

func TestInstall_TargetsSingleAgentAndRejectsUnknown(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	if out, err := e.run("", "install", "not-an-agent"); err == nil {
		t.Fatalf("install of unknown agent should fail:\n%s", out)
	}

	e.mustRun("install", "claude-code", "--yes")
	if _, err := os.Stat(e.claude("commands", "plan.md")); err != nil {
		t.Fatalf("command not installed: %v", err)
	}
}

func TestAgents_ListsClaudeCodeDetection(t *testing.T) {
	e := newEnv(t)
	out := e.mustRun("agents")
	if !strings.Contains(out, "claude-code") || !strings.Contains(out, "yes") {
		t.Fatalf("agents output:\n%s", out)
	}
}

func TestHelpAndVersion_Smoke(t *testing.T) {
	e := newEnv(t)
	if out := e.mustRun("--help"); !strings.Contains(out, "loadout") {
		t.Fatalf("--help output:\n%s", out)
	}
	if out := e.mustRun("version"); !strings.Contains(out, "Version:") {
		t.Fatalf("version output:\n%s", out)
	}
}

// snapshotDir maps every file under root to a stable content fingerprint.
func snapshotDir(t *testing.T, root string) map[string]string {
	t.Helper()
	snap := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		snap[path] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snap
}

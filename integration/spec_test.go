//go:build integration

package integration

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newProject creates a temp git repo with an origin remote — the project
// `loadout spec` commands operate on.
func newProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "remote", "add", "origin", "git@github.com:acme/demo.git")
	return dir
}

// stubGH writes a fake gh binary that persists issue bodies on disk and
// records every invocation — the external boundary is the only thing mocked.
func stubGH(t *testing.T) (binDir, stateDir string) {
	t.Helper()
	binDir, stateDir = t.TempDir(), t.TempDir()
	script := `#!/usr/bin/env bash
set -euo pipefail
state="` + stateDir + `"
echo "$*" >> "$state/calls.log"
case "$1-$2" in
  issue-create)
    cat > "$state/issue.body"
    echo "https://github.com/acme/demo/issues/12"
    ;;
  issue-view)
    python3 -c 'import json,sys; print(json.dumps({"body": open(sys.argv[1]).read()}))' "$state/issue.body"
    ;;
  issue-edit)
    cat > "$state/issue.body"
    ;;
  *) exit 0 ;;
esac
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir, stateDir
}

func fileHash(t *testing.T, path string) [32]byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(raw)
}

func TestSpec_LocalBackend_InitCheckStatusLoop(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)

	out, err := e.runIn(project, "", "", "spec", "init", "web-import")
	if err != nil || !strings.Contains(out, "001-web-import") {
		t.Fatalf("init: %v\n%s", err, out)
	}
	out, err = e.runIn(project, "", "", "spec", "init", "mcp-configs")
	if err != nil || !strings.Contains(out, "002-mcp-configs") {
		t.Fatalf("second init must pick 002: %v\n%s", err, out)
	}
	if out, err := e.runIn(project, "", "", "spec", "init", "web-import"); err == nil {
		t.Fatalf("re-init of an existing spec must refuse:\n%s", out)
	}

	specPath := filepath.Join(project, "docs", "specs", "001-web-import.md")

	// check is idempotent: second run exits 0 and leaves the file untouched.
	if out, err := e.runIn(project, "", "", "spec", "check", "1", "implement"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	h1 := fileHash(t, specPath)
	out, err = e.runIn(project, "", "", "spec", "check", "1", "implement")
	if err != nil || !strings.Contains(out, "already") {
		t.Fatalf("second check: %v\n%s", err, out)
	}
	if fileHash(t, specPath) != h1 {
		t.Fatal("idempotent check modified the file")
	}

	// unknown step lists valid IDs and exits non-zero
	out, err = e.runIn(project, "", "", "spec", "check", "1", "nope")
	if err == nil || !strings.Contains(out, "implement") {
		t.Fatalf("unknown step must fail listing valid ids: %v\n%s", err, out)
	}

	// status --json and the CI gate
	out, err = e.runIn(project, "", "", "spec", "status", "1", "--json")
	if err != nil || !strings.Contains(out, `"spec": "001-web-import"`) || !strings.Contains(out, `"done": 1`) {
		t.Fatalf("status --json: %v\n%s", err, out)
	}
	if out, err := e.runIn(project, "", "", "spec", "status", "1", "--require-complete"); err == nil {
		t.Fatalf("--require-complete with open steps must exit non-zero:\n%s", out)
	}
	for _, step := range []string{"review", "tests"} {
		e.mustRunIn(t, project, "spec", "check", "1", step)
	}
	if out, err := e.runIn(project, "", "", "spec", "status", "1", "--require-complete"); err != nil {
		t.Fatalf("--require-complete with all steps done must pass: %v\n%s", err, out)
	}
}

// mustRunIn is a tiny convenience for project-scoped calls.
func (e *env) mustRunIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := e.runIn(dir, "", "", args...)
	if err != nil {
		t.Fatalf("loadout %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func TestSpec_GitHubBackend_IssueLifecycleViaStubbedGH(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)
	ghBin, ghState := stubGH(t)

	out, err := e.runIn(project, ghBin, "", "spec", "init", "checkout-flow", "--backend", "github", "--label", "spec")
	if err != nil {
		t.Fatalf("init github: %v\n%s", err, out)
	}
	if !strings.Contains(out, "issue #12") {
		t.Fatalf("init should report the tracking issue:\n%s", out)
	}

	calls, _ := os.ReadFile(filepath.Join(ghState, "calls.log"))
	if !strings.Contains(string(calls), "issue create") || !strings.Contains(string(calls), "--label spec") {
		t.Fatalf("gh calls missing labeled issue create:\n%s", calls)
	}
	cfg, _ := os.ReadFile(filepath.Join(project, "docs", "specs", "sdd.yaml"))
	for _, want := range []string{"backend: github", "acme/demo", "001-checkout-flow", "12"} {
		if !strings.Contains(string(cfg), want) {
			t.Fatalf("sdd.yaml missing %q:\n%s", want, cfg)
		}
	}

	// check flips the box in the ISSUE body, not the local file.
	specPath := filepath.Join(project, "docs", "specs", "001-checkout-flow.md")
	before := fileHash(t, specPath)
	if out, err := e.runIn(project, ghBin, "", "spec", "check", "1", "implement"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	body, _ := os.ReadFile(filepath.Join(ghState, "issue.body"))
	if !strings.Contains(string(body), "- [x] implementation <!-- sdd:implement -->") {
		t.Fatalf("issue body not updated:\n%s", body)
	}
	if fileHash(t, specPath) != before {
		t.Fatal("github backend must not touch the local spec file on check")
	}

	// The issue and file now disagree — sync refuses to guess a direction.
	out, err = e.runIn(project, ghBin, "", "spec", "sync", "1")
	if err == nil || !strings.Contains(out, "--from-local") {
		t.Fatalf("divergent sync must exit non-zero pointing at direction flags: %v\n%s", err, out)
	}
	// --from-github pulls the state into the local file.
	if out, err := e.runIn(project, ghBin, "", "spec", "sync", "1", "--from-github"); err != nil {
		t.Fatalf("sync --from-github: %v\n%s", err, out)
	}
	local, _ := os.ReadFile(specPath)
	if !strings.Contains(string(local), "- [x] implementation <!-- sdd:implement -->") {
		t.Fatalf("state not pulled into local file:\n%s", local)
	}
}

func TestSpec_GitHubBackend_MissingGHIsActionable(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)
	emptyPath := t.TempDir() // PATH with no gh at all

	if err := os.MkdirAll(filepath.Join(project, "docs", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "specs", "sdd.yaml"), []byte("backend: github\nrepo: acme/demo\nissues:\n  001-demo: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "docs", "specs", "001-demo.md"), []byte("- [ ] x <!-- sdd:x -->\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := e.runWithExactPath(project, emptyPath, "spec", "status", "1")
	if err == nil || !strings.Contains(out, "cli.github.com") {
		t.Fatalf("missing gh must produce an actionable install/auth message: %v\n%s", err, out)
	}
}

// runWithExactPath replaces PATH entirely so a system-installed gh cannot
// be found — the "gh not installed" behavior under test.
func (e *env) runWithExactPath(dir, pathValue string, args ...string) (string, error) {
	e.t.Helper()
	cmd := exec.Command(binary(e.t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"HOME="+e.home,
		"XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(e.home, ".local", "share"),
		"PATH="+pathValue,
	)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestWizard_SDDPresetInstallsCLIWiredArtifacts(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	out, err := e.run("", "wizard", "--preset", "sdd", "--yes")
	if err != nil {
		t.Fatalf("wizard --preset sdd: %v\n%s", err, out)
	}
	e.mustRun("sync", "--yes")

	raw, err := os.ReadFile(e.claude("skills", "sdd-workflow", "SKILL.md"))
	if err != nil {
		t.Fatalf("sdd-workflow skill not installed: %v", err)
	}
	for _, want := range []string{"loadout spec check", "loadout spec status"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("installed skill missing %q — completion loop not wired to the CLI", want)
		}
	}
	cmdRaw, err := os.ReadFile(e.claude("commands", "spec.md"))
	if err != nil {
		t.Fatalf("spec command not installed: %v", err)
	}
	if !strings.Contains(string(cmdRaw), "loadout spec init") {
		t.Errorf("installed /spec command must scaffold via loadout spec init:\n%s", cmdRaw)
	}
}

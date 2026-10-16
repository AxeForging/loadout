//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubGHIssues writes a fake gh that keeps a small issue tracker on disk
// (title/body/labels per issue) — enough for list/create/view/edit and
// label create. Only the external boundary (GitHub) is faked.
func stubGHIssues(t *testing.T) (binDir, stateDir string) {
	t.Helper()
	binDir, stateDir = t.TempDir(), t.TempDir()
	script := `#!/usr/bin/env python3
import json, os, sys
state = ` + "\"" + stateDir + "\"" + `
args = sys.argv[1:]
with open(os.path.join(state, "calls.log"), "a") as f:
    f.write(" ".join(args) + "\n")
def flag(name, default=None):
    return args[args.index(name) + 1] if name in args else default
db_path = os.path.join(state, "issues.json")
db = json.load(open(db_path)) if os.path.exists(db_path) else {}
def save():
    json.dump(db, open(db_path, "w"))
cmd = " ".join(args[:2])
if cmd == "label create":
    sys.exit(0)
if cmd == "issue list":
    label = flag("--label")
    out = [{"number": int(n), "title": i["title"]} for n, i in db.items() if label in i["labels"]]
    print(json.dumps(out)); sys.exit(0)
if cmd == "issue create":
    n = str(max([int(k) for k in db] + [19]) + 1)
    labels = [args[i + 1] for i, a in enumerate(args) if a == "--label"]
    db[n] = {"title": flag("--title"), "body": sys.stdin.read(), "labels": labels}
    save(); print("https://github.com/acme/demo/issues/" + n); sys.exit(0)
if cmd == "issue view":
    print(json.dumps({"body": db[args[2]]["body"]})); sys.exit(0)
if cmd == "issue edit":
    db[args[2]]["body"] = sys.stdin.read(); save(); sys.exit(0)
sys.stderr.write("unexpected gh call: " + cmd + "\n"); sys.exit(1)
`
	if err := os.WriteFile(filepath.Join(binDir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binDir, stateDir
}

type stubIssue struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Labels []string `json:"labels"`
}

// seedIssues replaces the stub tracker's issues (keyed by issue number).
func seedIssues(t *testing.T, stateDir string, issues map[string]stubIssue) {
	t.Helper()
	raw, err := json.Marshal(issues)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "issues.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readIssues(t *testing.T, stateDir string) map[string]stubIssue {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(stateDir, "issues.json"))
	if err != nil {
		t.Fatal(err)
	}
	var issues map[string]stubIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		t.Fatal(err)
	}
	return issues
}

func TestSpecIssues_InitOpensNumberedIssueAndWritesNothingInRepo(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)
	ghBin, ghState := stubGHIssues(t)
	// Spec 009 already exists as an issue; an unrelated labelled issue and a
	// non-spec issue must not affect numbering.
	seedIssues(t, ghState, map[string]stubIssue{
		"7": {Title: "spec(009): artifact removal", Labels: []string{"spec"}},
		"8": {Title: "bug: crash", Labels: []string{"bug"}},
	})

	out, err := e.runIn(project, ghBin, "", "spec", "init", "drift-safe sync", "--backend", "issues")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "#20") || !strings.Contains(out, "spec(010): drift-safe sync") {
		t.Fatalf("init should open spec(010) as issue #20:\n%s", out)
	}
	created, ok := readIssues(t, ghState)["20"]
	if !ok || created.Title != "spec(010): drift-safe sync" || len(created.Labels) != 1 || created.Labels[0] != "spec" {
		t.Fatalf("created issue = %+v", created)
	}
	for _, want := range []string{"<!-- plan:start -->", "<!-- plan:end -->", "## Requirements", "## Rollout"} {
		if !strings.Contains(created.Body, want) {
			t.Errorf("created body missing %q:\n%s", want, created.Body)
		}
	}
	if _, err := os.Stat(filepath.Join(project, "docs")); !os.IsNotExist(err) {
		t.Error("issues backend wrote into the repository")
	}

	// A duplicate name is refused.
	if out, err := e.runIn(project, ghBin, "", "spec", "init", "Drift-safe sync", "--backend", "issues"); err == nil {
		t.Fatalf("duplicate init must fail:\n%s", out)
	}
}

const issuePlanBody = `## Context

- [ ] not a plan step

<!-- plan:start -->
## Plan

- [ ] **1. Parser** — files: a.go — covers: R1
- [x] **2. Backend** — files: b.go — covers: R2
- [ ] **3. CLI wiring** — files: c.go — covers: R3

` + "```" + `
- [ ] **9. inside a fence**
` + "```" + `
<!-- plan:end -->
`

func TestSpecIssues_StatusCheckAndUncheckEditOnlyThePlanCheckbox(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)
	ghBin, ghState := stubGHIssues(t)
	seedIssues(t, ghState, map[string]stubIssue{"25": {Title: "spec(011): issues backend", Body: issuePlanBody, Labels: []string{"spec"}}})

	list, err := e.runIn(project, ghBin, "", "spec", "list", "--backend", "issues")
	if err != nil || !strings.Contains(list, "011-issues-backend") || !strings.Contains(list, "1/3") || !strings.Contains(list, "issue #25") {
		t.Fatalf("list: %v\n%s", err, list)
	}

	// Check by spec number, then by issue reference.
	if out, err := e.runIn(project, ghBin, "", "spec", "check", "011", "1", "--backend", "issues"); err != nil {
		t.Fatalf("check: %v\n%s", err, out)
	}
	// Someone edits the issue in the GitHub UI between our commands: the next
	// check must build on that edit (re-fetch), not on a stale copy.
	db := readIssues(t, ghState)
	edited := db["25"]
	edited.Body = strings.Replace(edited.Body, "## Context", "## Context (edited in UI)", 1)
	db["25"] = edited
	seedIssues(t, ghState, db)
	if out, err := e.runIn(project, ghBin, "", "spec", "check", "#25", "3", "--backend", "issues"); err != nil {
		t.Fatalf("check #25: %v\n%s", err, out)
	}
	if out, err := e.runIn(project, ghBin, "", "spec", "uncheck", "011", "2", "--backend", "issues"); err != nil {
		t.Fatalf("uncheck: %v\n%s", err, out)
	}

	want := strings.NewReplacer(
		"## Context", "## Context (edited in UI)",
		"- [ ] **1. Parser**", "- [x] **1. Parser**",
		"- [x] **2. Backend**", "- [ ] **2. Backend**",
		"- [ ] **3. CLI wiring**", "- [x] **3. CLI wiring**",
	).Replace(issuePlanBody)
	if got := readIssues(t, ghState)["25"].Body; got != want {
		t.Fatalf("body after edits is not the original with exactly three boxes flipped:\nwant %s\ngot  %s", want, got)
	}

	// status reflects it; an unknown step lists the valid IDs.
	if out, err := e.runIn(project, ghBin, "", "spec", "status", "011", "--backend", "issues", "--json"); err != nil || !strings.Contains(out, `"done": 2`) {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if out, err := e.runIn(project, ghBin, "", "spec", "check", "011", "9", "--backend", "issues"); err == nil || !strings.Contains(out, "1, 2, 3") {
		t.Fatalf("fenced/unknown step must fail listing valid ids: %v\n%s", err, out)
	}

	// Every edit was preceded by a fresh view of the body.
	calls, _ := os.ReadFile(filepath.Join(ghState, "calls.log"))
	lines := strings.Split(strings.TrimSpace(string(calls)), "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "issue edit") && (i == 0 || !strings.HasPrefix(lines[i-1], "issue view")) {
			t.Errorf("issue edit not immediately preceded by issue view:\n%s", calls)
		}
	}
}

func TestSpecIssues_SyncIsRefusedAndEnvSelectsBackend(t *testing.T) {
	e := newEnv(t)
	project := newProject(t)
	ghBin, ghState := stubGHIssues(t)
	seedIssues(t, ghState, map[string]stubIssue{"25": {Title: "spec(011): issues backend", Labels: []string{"spec"}}})

	out, err := e.runIn(project, ghBin, "", "spec", "sync", "011", "--backend", "issues")
	if err == nil || !strings.Contains(out, "not applicable") {
		t.Fatalf("sync must be refused on the issues backend: %v\n%s", err, out)
	}

	t.Setenv("LOADOUT_SDD_BACKEND", "issues")
	out, err = e.runIn(project, ghBin, "", "spec", "list")
	if err != nil || !strings.Contains(out, "issue #25") {
		t.Fatalf("LOADOUT_SDD_BACKEND=issues should select the issues backend: %v\n%s", err, out)
	}
}

func TestWizard_IssuesBackendInstallsIssuesOnlyArtifacts(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	answers := filepath.Join(t.TempDir(), "answers.yaml")
	if err := os.WriteFile(answers, []byte("sdd.enable: true\nsdd.backend: issues\nsdd.style: full\ncontinuity.handoff: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := e.run("", "wizard", "--preset", "sdd", "--answers", answers, "--yes"); err != nil {
		t.Fatalf("wizard: %v\n%s", err, out)
	}
	e.mustRun("sync", "--yes")

	for path, want := range map[string]string{
		e.claude("commands", "ld-spec.md"):                "<!-- plan:start -->",
		e.claude("commands", "ld-plan.md"):                "loadout spec check NNN <N> --backend issues",
		e.claude("commands", "ld-handoff.md"):             "<!-- handoff -->",
		e.claude("skills", "ld-sdd-workflow", "SKILL.md"): "GitHub issues",
	} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("not installed: %v", err)
		}
		if !strings.Contains(string(raw), want) || strings.Contains(string(raw), ".plan.md") {
			t.Errorf("%s: want %q and no repo plan file:\n%s", path, want, raw)
		}
	}
}

//go:build integration

package integration

import (
	"os"
	"strings"
	"testing"
)

func TestStoreGitHint_WarnsUntilCommittedAndOnUncommittedEdits(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	e.mustRun("new", "command", "plan")

	for _, args := range [][]string{{"status"}, {"sync", "--dry-run"}, {"doctor"}} {
		if out, _ := e.run("", args...); !strings.Contains(out, "store has no commits") {
			t.Errorf("loadout %s should warn about a store with no commits:\n%s", strings.Join(args, " "), out)
		}
	}

	store := e.storePath()
	gitIn(t, store, "add", "-A")
	gitIn(t, store, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false", "commit", "-qm", "store")
	if out := e.mustRun("status"); strings.Contains(out, "store has") {
		t.Errorf("committed store should not warn:\n%s", out)
	}

	if err := os.WriteFile(e.storePath("commands", "plan.md"), []byte("edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := e.mustRun("sync", "--dry-run"); !strings.Contains(out, "store has 1 uncommitted change(s)") {
		t.Errorf("sync should mention the uncommitted store edit:\n%s", out)
	}
}

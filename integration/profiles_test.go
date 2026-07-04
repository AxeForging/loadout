//go:build integration

package integration

import (
	"strings"
	"testing"
)

func TestProfile_SaveListShowReuseAndRemove(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")

	// Saving a profile from a wizard run doesn't require confirming the
	// artifact write — it's independent bookkeeping of the answers.
	out := e.mustRun("wizard", "--preset", "go-cli", "--save-profile", "acme-cli", "--yes")
	if !strings.Contains(out, `Saved profile "acme-cli"`) {
		t.Fatalf("wizard --save-profile output:\n%s", out)
	}

	out = e.mustRun("profile", "list")
	if !strings.Contains(out, "acme-cli") {
		t.Fatalf("profile list output:\n%s", out)
	}

	out = e.mustRun("profile", "show", "acme-cli")
	if !strings.Contains(out, "toolchain.packageManagers") {
		t.Fatalf("profile show output:\n%s", out)
	}

	// Re-running the wizard from the saved profile reproduces the exact
	// same answers — everything it would generate already exists.
	out = e.mustRun("wizard", "--profile", "acme-cli", "--yes")
	if !strings.Contains(out, "Nothing to generate") {
		t.Fatalf("re-running the same profile should be a no-op:\n%s", out)
	}

	e.mustRun("profile", "rm", "acme-cli")
	out = e.mustRun("profile", "list")
	if strings.Contains(out, "acme-cli") {
		t.Fatalf("profile should be removed:\n%s", out)
	}
	if _, err := e.run("", "profile", "show", "acme-cli"); err == nil {
		t.Fatal("showing a removed profile should error")
	}
}

func TestProfile_UnknownNameFails(t *testing.T) {
	e := newEnv(t)
	e.mustRun("init")
	if _, err := e.run("", "wizard", "--profile", "nope", "--yes"); err == nil {
		t.Fatal("unknown profile should fail")
	}
}

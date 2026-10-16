package store

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestGitHealth_FlagsNoCommitsThenUncommittedThenClean(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := filepath.Join(t.TempDir(), "store")
	s, err := Init(root, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	h, ok := s.GitHealth(ctx)
	if !ok || !h.Repo || h.HasCommits {
		t.Fatalf("fresh store: %+v ok=%v, want repo without commits", h, ok)
	}
	if w := h.Warning(root); !strings.Contains(w, "no commits") || !strings.Contains(w, root) {
		t.Errorf("fresh store warning = %q", w)
	}

	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-qm", "init")
	h, _ = s.GitHealth(ctx)
	if !h.HasCommits || h.Uncommitted != 0 || h.Warning(root) != "" {
		t.Fatalf("committed store: %+v warning=%q, want clean", h, h.Warning(root))
	}

	if err := os.WriteFile(filepath.Join(root, "instructions.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "loadout.yaml"), []byte("edited: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ = s.GitHealth(ctx)
	if h.Uncommitted != 2 || !strings.Contains(h.Warning(root), "2 uncommitted") {
		t.Errorf("dirty store: %+v warning=%q, want 2 uncommitted", h, h.Warning(root))
	}
}

func TestGitHealth_NonRepoStoreIsNotNagged(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	s := &Store{Root: root}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(root))
	h, ok := s.GitHealth(context.Background())
	if !ok || h.Repo || h.Warning(root) != "" {
		t.Errorf("non-repo store: %+v warning=%q, want no warning", h, h.Warning(root))
	}
}

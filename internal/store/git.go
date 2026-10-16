package store

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// GitHealth describes whether the store's edits are recoverable through git.
// The store is git-initialised by `loadout init`, but nothing commits it, so
// a store can sit for months with no history at all.
type GitHealth struct {
	// Repo is true when the store is inside a git work tree.
	Repo bool
	// HasCommits is false for a freshly initialised repo ("no commits yet").
	HasCommits bool
	// Uncommitted counts changed/untracked paths under the store root.
	Uncommitted int
}

// GitHealth inspects the store with the system git. ok is false when git is
// not installed (nothing to report).
func (s *Store) GitHealth(ctx context.Context) (h GitHealth, ok bool) {
	if _, err := exec.LookPath("git"); err != nil {
		return h, false
	}
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = s.Root
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		return out.String(), err
	}
	if out, err := run("rev-parse", "--is-inside-work-tree"); err != nil || strings.TrimSpace(out) != "true" {
		return h, true
	}
	h.Repo = true
	if _, err := run("rev-parse", "--verify", "-q", "HEAD"); err == nil {
		h.HasCommits = true
	}
	if out, err := run("status", "--porcelain", "--untracked-files=all", "--", "."); err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.TrimSpace(line) != "" {
				h.Uncommitted++
			}
		}
	}
	return h, true
}

// Warning is a one-line hint when store edits are not protected by history,
// or "" when the store is committed (or not a git repo at all — a store kept
// elsewhere on purpose is not loadout's business to nag about).
func (h GitHealth) Warning(root string) string {
	switch {
	case !h.Repo:
		return ""
	case !h.HasCommits:
		return fmt.Sprintf("store has no commits — edits to it have no history or rollback. Commit it: git -C %s add -A && git -C %s commit -m \"loadout store\"", root, root)
	case h.Uncommitted > 0:
		return fmt.Sprintf("store has %d uncommitted change(s) — commit them to keep history: git -C %s status", h.Uncommitted, root)
	}
	return ""
}

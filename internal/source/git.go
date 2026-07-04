package source

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// git runs a git command and returns trimmed stdout. Stderr is folded into
// the error so credential and network failures surface verbatim.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	// Never let git prompt for credentials from a non-interactive run.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// cloneFresh replaces dir with a shallow clone of url at ref and returns the
// resolved HEAD commit.
func cloneFresh(ctx context.Context, url, ref, dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	args := []string{"clone", "--quiet", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dir)
	if _, err := git(ctx, "", args...); err != nil {
		return "", err
	}
	return git(ctx, dir, "rev-parse", "HEAD")
}

// cloneFull replaces dir with a full clone (history included, so update
// changelogs can span the old pin) and returns the resolved HEAD commit.
func cloneFull(ctx context.Context, url, ref, dir string) (string, error) {
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", err
	}
	args := []string{"clone", "--quiet"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, url, dir)
	if _, err := git(ctx, "", args...); err != nil {
		return "", err
	}
	return git(ctx, dir, "rev-parse", "HEAD")
}

// lsRemote resolves the remote hash of ref (or HEAD) without cloning.
func lsRemote(ctx context.Context, url, ref string) (string, error) {
	target := "HEAD"
	if ref != "" {
		target = ref
	}
	out, err := git(ctx, "", "ls-remote", url, target)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return "", fmt.Errorf("ls-remote %s: ref %q not found", url, target)
	}
	return fields[0], nil
}

// changelog returns the one-line log between two commits in a clone; a
// failure (e.g. shallow history) degrades to an empty log, never an error.
func changelog(ctx context.Context, dir, from, to string) []string {
	out, err := git(ctx, dir, "log", "--oneline", from+".."+to)
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

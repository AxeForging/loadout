// Package runner drives coding agents the user is ALREADY logged into in
// headless mode (claude -p, codex exec, gemini -p) to draft or enrich
// artifacts — no API keys, it rides the user's existing plan. Runners are
// strictly text-in/text-out: generation happens read-only and sandboxed, and
// whatever comes back goes through the same show-then-confirm gate as every
// other store write. Deterministic templates never depend on a runner being
// present.
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes one prompt against a headless agent and returns its text.
type Runner interface {
	ID() string
	// Available reports whether the agent binary is on PATH.
	Available() bool
	Run(ctx context.Context, prompt string) (string, error)
}

// Registry returns the built-in runners in preference order.
func Registry() []Runner {
	return []Runner{claudeRunner{}, codexRunner{}, geminiRunner{}}
}

// Get resolves a runner by ID.
func Get(id string) (Runner, error) {
	for _, r := range Registry() {
		if r.ID() == id {
			return r, nil
		}
	}
	return nil, fmt.Errorf("unknown runner %q (available: claude, codex, gemini)", id)
}

// First returns the first available runner, or an error naming what to
// install/login.
func First() (Runner, error) {
	for _, r := range Registry() {
		if r.Available() {
			return r, nil
		}
	}
	return nil, fmt.Errorf("no headless agent found on PATH — install and log into one of: claude, codex, gemini")
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %v: %s", name, err, truncate(errBuf.String(), 400))
	}
	return strings.TrimSpace(out.String()), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func onPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// claudeRunner shells out to Claude Code. --output-format json wraps the
// final message in a JSON envelope with a "result" field.
type claudeRunner struct{}

func (claudeRunner) ID() string      { return "claude" }
func (claudeRunner) Available() bool { return onPath("claude") }

func (claudeRunner) Run(ctx context.Context, prompt string) (string, error) {
	out, err := run(ctx, "claude", "-p", prompt, "--output-format", "json")
	if err != nil {
		return "", err
	}
	var envelope struct {
		Result string `json:"result"`
	}
	if jErr := json.Unmarshal([]byte(out), &envelope); jErr == nil && envelope.Result != "" {
		return strings.TrimSpace(envelope.Result), nil
	}
	return out, nil
}

// codexRunner shells out to Codex CLI in a read-only sandbox with approvals
// disabled — generation only, it cannot touch the workspace.
type codexRunner struct{}

func (codexRunner) ID() string      { return "codex" }
func (codexRunner) Available() bool { return onPath("codex") }

func (codexRunner) Run(ctx context.Context, prompt string) (string, error) {
	return run(ctx, "codex",
		"-c", "approval_policy=never",
		"-c", "sandbox_mode=read-only",
		"exec", "--skip-git-repo-check",
		prompt,
	)
}

// geminiRunner shells out to Gemini CLI's non-interactive prompt mode.
type geminiRunner struct{}

func (geminiRunner) ID() string      { return "gemini" }
func (geminiRunner) Available() bool { return onPath("gemini") }

func (geminiRunner) Run(ctx context.Context, prompt string) (string, error) {
	return run(ctx, "gemini", "-p", prompt)
}

package sdd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// gh shells out to the system GitHub CLI — the same philosophy as loadout's
// system-git rule: the user's existing auth (gh auth, SSO, enterprise hosts)
// just works and loadout never stores a credential. Prompts are suppressed;
// stderr is folded into the returned error.
func gh(ctx context.Context, dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	cmd.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
		"NO_COLOR=1",
		"GIT_TERMINAL_PROMPT=0",
	)
	if err := cmd.Run(); err != nil {
		if _, lookErr := exec.LookPath("gh"); lookErr != nil {
			return "", fmt.Errorf("the github backend needs the GitHub CLI: install gh (https://cli.github.com) and run 'gh auth login'")
		}
		return "", fmt.Errorf("gh %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errBuf.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// DetectRepo parses owner/name from the project's origin remote. Both SSH
// (git@github.com:owner/name.git) and HTTPS forms are supported.
func DetectRepo(ctx context.Context, projectRoot string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("detect repo from origin: %v: %s (set `repo:` in %s instead)", err, strings.TrimSpace(errBuf.String()), ConfigFile)
	}
	return ParseRepoURL(strings.TrimSpace(out.String()))
}

var repoURLRe = regexp.MustCompile(`[:/]([^:/]+/[^:/]+?)(\.git)?$`)

// ParseRepoURL extracts owner/name from an SSH or HTTPS git remote URL.
func ParseRepoURL(url string) (string, error) {
	m := repoURLRe.FindStringSubmatch(url)
	if m == nil {
		return "", fmt.Errorf("cannot parse owner/name from remote url %q", url)
	}
	return m[1], nil
}

// GitHubBackend keeps checklist state in one issue per spec. The issue body
// is mutated with the same byte-preserving SetStep as local spec files.
type GitHubBackend struct {
	Dir         string // specs directory (for config updates)
	ProjectRoot string
	Config      *Config
}

// repo resolves the configured or auto-detected owner/name.
func (b *GitHubBackend) repo(ctx context.Context) (string, error) {
	if b.Config.Repo != "" {
		return b.Config.Repo, nil
	}
	repo, err := DetectRepo(ctx, b.ProjectRoot)
	if err != nil {
		return "", err
	}
	b.Config.Repo = repo
	return repo, nil
}

func (b *GitHubBackend) issueOf(spec SpecInfo) (int, error) {
	if spec.Issue == 0 {
		return 0, fmt.Errorf("spec %s has no linked issue — run `loadout spec sync %03d` to create/link it", spec.Name, spec.Number)
	}
	return spec.Issue, nil
}

func (b *GitHubBackend) issueBody(ctx context.Context, repo string, issue int) (string, error) {
	out, err := gh(ctx, b.ProjectRoot, nil, "issue", "view", strconv.Itoa(issue), "--repo", repo, "--json", "body")
	if err != nil {
		return "", err
	}
	var payload struct {
		Body string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		return "", fmt.Errorf("parse gh issue view output: %w", err)
	}
	return payload.Body, nil
}

func (b *GitHubBackend) Steps(ctx context.Context, spec SpecInfo) ([]Step, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return nil, err
	}
	issue, err := b.issueOf(spec)
	if err != nil {
		return nil, err
	}
	body, err := b.issueBody(ctx, repo, issue)
	if err != nil {
		return nil, err
	}
	return ParseSteps([]byte(body))
}

func (b *GitHubBackend) SetStep(ctx context.Context, spec SpecInfo, stepID string, done bool) (bool, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return false, err
	}
	issue, err := b.issueOf(spec)
	if err != nil {
		return false, err
	}
	body, err := b.issueBody(ctx, repo, issue)
	if err != nil {
		return false, err
	}
	out, changed, err := SetStep([]byte(body), stepID, done)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	_, err = gh(ctx, b.ProjectRoot, out, "issue", "edit", strconv.Itoa(issue), "--repo", repo, "--body-file", "-")
	return err == nil, err
}

var issueURLRe = regexp.MustCompile(`/issues/(\d+)\s*$`)

// CreateIssue opens the spec's tracking issue with the given body and
// records the spec → issue mapping in sdd.yaml.
func (b *GitHubBackend) CreateIssue(ctx context.Context, spec SpecInfo, body []byte) (int, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return 0, err
	}
	args := []string{"issue", "create", "--repo", repo, "--title", spec.Name, "--body-file", "-"}
	for _, l := range b.Config.Labels {
		args = append(args, "--label", l)
	}
	out, err := gh(ctx, b.ProjectRoot, body, args...)
	if err != nil {
		return 0, err
	}
	m := issueURLRe.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("gh issue create: cannot find issue number in output %q", out)
	}
	num, _ := strconv.Atoi(m[1])
	if b.Config.Issues == nil {
		b.Config.Issues = map[string]int{}
	}
	b.Config.Issues[spec.Name] = num
	if err := b.Config.Save(b.Dir); err != nil {
		return 0, err
	}
	return num, nil
}

// SetBody replaces the tracking issue's entire body — used by sync when
// pushing local structure/state to GitHub.
func (b *GitHubBackend) SetBody(ctx context.Context, spec SpecInfo, body []byte) (bool, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return false, err
	}
	issue, err := b.issueOf(spec)
	if err != nil {
		return false, err
	}
	_, err = gh(ctx, b.ProjectRoot, body, "issue", "edit", strconv.Itoa(issue), "--repo", repo, "--body-file", "-")
	return err == nil, err
}

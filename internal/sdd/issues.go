package sdd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// BackendIssues keeps the whole spec — sections and plan — in one GitHub
// issue per spec; nothing lives in the repository.
const BackendIssues = "issues"

// DefaultIssueLabel marks spec issues when no label is configured.
const DefaultIssueLabel = "spec"

// specTitleRe matches spec issue titles: "spec(NNN): short name".
var specTitleRe = regexp.MustCompile(`^spec\((\d{1,4})\):\s*(.+?)\s*$`)

// IssuesBackend reads and edits spec issues through the gh CLI. Every
// mutation re-fetches the issue body right before editing it, so concurrent
// edits by other agents or the GitHub UI are never overwritten from a stale
// copy.
type IssuesBackend struct {
	ProjectRoot string
	Config      *Config
}

func (b *IssuesBackend) repo(ctx context.Context) (string, error) {
	gb := GitHubBackend{ProjectRoot: b.ProjectRoot, Config: b.Config}
	return gb.repo(ctx)
}

// Label is the label spec issues carry (first configured label, or "spec").
func (b *IssuesBackend) Label() string {
	if len(b.Config.Labels) > 0 {
		return b.Config.Labels[0]
	}
	return DefaultIssueLabel
}

// List returns every issue labelled as a spec whose title follows
// "spec(NNN): name", open or closed, ordered by number.
func (b *IssuesBackend) List(ctx context.Context) ([]SpecInfo, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return nil, err
	}
	out, err := gh(ctx, b.ProjectRoot, nil, "issue", "list", "--repo", repo, "--label", b.Label(),
		"--state", "all", "--limit", "500", "--json", "number,title")
	if err != nil {
		return nil, err
	}
	var issues []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &issues); err != nil {
		return nil, fmt.Errorf("parse gh issue list output: %w", err)
	}
	var specs []SpecInfo
	for _, is := range issues {
		m := specTitleRe.FindStringSubmatch(is.Title)
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		specs = append(specs, SpecInfo{
			Number: num,
			Name:   fmt.Sprintf("%03d-%s", num, slugify(m[2])),
			Title:  is.Title,
			Issue:  is.Number,
		})
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Number != specs[j].Number {
			return specs[i].Number < specs[j].Number
		}
		return specs[i].Issue < specs[j].Issue
	})
	return specs, nil
}

func (b *IssuesBackend) body(ctx context.Context, spec SpecInfo) (string, string, error) {
	repo, err := b.repo(ctx)
	if err != nil {
		return "", "", err
	}
	gb := GitHubBackend{ProjectRoot: b.ProjectRoot, Config: b.Config}
	body, err := gb.issueBody(ctx, repo, spec.Issue)
	return repo, body, err
}

// Steps returns the plan checklist of the spec issue.
func (b *IssuesBackend) Steps(ctx context.Context, spec SpecInfo) ([]Step, error) {
	_, body, err := b.body(ctx, spec)
	if err != nil {
		return nil, err
	}
	return ParsePlanSteps([]byte(body))
}

// SetStep re-fetches the body, flips one plan checkbox and writes it back.
func (b *IssuesBackend) SetStep(ctx context.Context, spec SpecInfo, stepID string, done bool) (bool, error) {
	repo, body, err := b.body(ctx, spec)
	if err != nil {
		return false, err
	}
	out, changed, err := SetPlanStep([]byte(body), stepID, done)
	if err != nil || !changed {
		return false, err
	}
	_, err = gh(ctx, b.ProjectRoot, out, "issue", "edit", strconv.Itoa(spec.Issue), "--repo", repo, "--body-file", "-")
	return err == nil, err
}

// Create opens spec(NNN): <name> with the issue template body, numbered after
// every existing spec issue and any local spec numbers passed in (so a repo
// moving from files to issues never reuses a number). The label is created
// first if the repo does not have it yet.
func (b *IssuesBackend) Create(ctx context.Context, name string, localSpecs []SpecInfo, fullTemplate bool) (SpecInfo, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return SpecInfo{}, fmt.Errorf("spec name is required")
	}
	repo, err := b.repo(ctx)
	if err != nil {
		return SpecInfo{}, err
	}
	existing, err := b.List(ctx)
	if err != nil {
		return SpecInfo{}, err
	}
	slug := slugify(name)
	if slug == "" {
		return SpecInfo{}, fmt.Errorf("invalid spec name %q: needs at least one letter or digit", name)
	}
	for _, s := range existing {
		if _, rest, _ := strings.Cut(s.Name, "-"); rest == slug {
			return SpecInfo{}, fmt.Errorf("spec issue #%d %q already exists — refusing to open a duplicate", s.Issue, s.Title)
		}
	}
	num := NextNumber(append(append([]SpecInfo{}, existing...), localSpecs...))
	title := fmt.Sprintf("spec(%03d): %s", num, name)

	// Best effort: an existing label makes this fail, which is fine.
	_, _ = gh(ctx, b.ProjectRoot, nil, "label", "create", b.Label(), "--repo", repo,
		"--color", "5319E7", "--description", "Spec + plan (spec-driven development)")

	out, err := gh(ctx, b.ProjectRoot, []byte(issueTemplate(fullTemplate)), "issue", "create", "--repo", repo,
		"--title", title, "--label", b.Label(), "--body-file", "-")
	if err != nil {
		return SpecInfo{}, err
	}
	m := issueURLRe.FindStringSubmatch(out)
	if m == nil {
		return SpecInfo{}, fmt.Errorf("gh issue create: cannot find issue number in output %q", out)
	}
	issue, _ := strconv.Atoi(m[1])
	return SpecInfo{Number: num, Name: fmt.Sprintf("%03d-%s", num, slug), Title: title, Issue: issue}, nil
}

var nonSlugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a free-text spec name into the kebab-case form used in
// SpecInfo.Name ("Drift-safe sync!" → "drift-safe-sync").
func slugify(s string) string {
	return strings.Trim(nonSlugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

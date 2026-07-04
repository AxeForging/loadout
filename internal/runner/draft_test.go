package runner

import (
	"context"
	"strings"
	"testing"
)

// stubRunner returns canned output, standing in for a logged-in agent.
type stubRunner struct{ out string }

func (s stubRunner) ID() string      { return "stub" }
func (s stubRunner) Available() bool { return true }
func (s stubRunner) Run(ctx context.Context, prompt string) (string, error) {
	return s.out, nil
}

const validSkill = `---
name: release-notes
description: Draft release notes from merged PRs. Use when preparing a release.
---

# Release notes

## When to use
Before tagging a release.

## Steps
1. List merged PRs since the last tag.
`

func TestDraftSkill_AcceptsValidOutput(t *testing.T) {
	got, err := DraftSkill(context.Background(), stubRunner{out: validSkill}, "release-notes", "draft release notes")
	if err != nil {
		t.Fatalf("DraftSkill() error = %v", err)
	}
	if !strings.HasPrefix(got, "---\nname: release-notes") {
		t.Errorf("unexpected content:\n%s", got)
	}
}

func TestDraftSkill_StripsCodeFencesTheModelAdded(t *testing.T) {
	fenced := "```markdown\n" + validSkill + "```"
	got, err := DraftSkill(context.Background(), stubRunner{out: fenced}, "release-notes", "x")
	if err != nil {
		t.Fatalf("DraftSkill() error = %v", err)
	}
	if strings.Contains(got, "```") {
		t.Errorf("fences not stripped:\n%s", got)
	}
}

func TestDraftSkill_RejectsInvalidOutput(t *testing.T) {
	cases := map[string]string{
		"no frontmatter": "# just markdown\n",
		"wrong name":     "---\nname: other\ndescription: x\n---\nbody\n",
		"no description": "---\nname: release-notes\n---\nbody\n",
		"empty body":     "---\nname: release-notes\ndescription: x\n---\n",
	}
	for label, out := range cases {
		if _, err := DraftSkill(context.Background(), stubRunner{out: out}, "release-notes", "x"); err == nil {
			t.Errorf("%s: expected validation error", label)
		}
	}
}

func TestEnrichMarkdown_RejectsEmptyOrNonMarkdown(t *testing.T) {
	if _, err := EnrichMarkdown(context.Background(), stubRunner{out: "   "}, "testing.md", "# t", "ctx"); err == nil {
		t.Error("empty output should be rejected")
	}
	if _, err := EnrichMarkdown(context.Background(), stubRunner{out: "sure! here you go"}, "testing.md", "# t", "ctx"); err == nil {
		t.Error("non-markdown output should be rejected")
	}
	got, err := EnrichMarkdown(context.Background(), stubRunner{out: "# Testing\n\n- run go test\n"}, "testing.md", "# t", "ctx")
	if err != nil || !strings.HasPrefix(got, "# Testing") {
		t.Errorf("valid markdown rejected: %v\n%s", err, got)
	}
}

package adapter

import (
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
)

func TestCommandAsSkill_LiftsFrontmatterDescription(t *testing.T) {
	raw := []byte("---\ndescription: Plan a feature before coding.\n---\n\nThink step by step.\n")
	out := string(CommandAsSkill("plan", raw))
	if !strings.Contains(out, "name: plan") {
		t.Errorf("missing name frontmatter:\n%s", out)
	}
	if !strings.Contains(out, "description: Plan a feature before coding.") {
		t.Errorf("description not lifted:\n%s", out)
	}
	if !strings.Contains(out, "Think step by step.") {
		t.Errorf("body lost:\n%s", out)
	}
	if strings.Count(out, "---") != 2 {
		t.Errorf("frontmatter fences wrong:\n%s", out)
	}
}

func TestCommandAsSkill_FallsBackWithoutFrontmatter(t *testing.T) {
	out := string(CommandAsSkill("ship", []byte("Just do the thing.\n")))
	if !strings.Contains(out, "description: Slash command /ship managed by loadout.") {
		t.Errorf("fallback description missing:\n%s", out)
	}
	if !strings.Contains(out, "Just do the thing.") {
		t.Errorf("body lost:\n%s", out)
	}
}

func TestSkillFiles_MarksScriptsExecutableAndSortsDeterministically(t *testing.T) {
	art := domain.Artifact{ID: "demo", Kind: domain.KindSkill}
	content := domain.Content{Files: map[string][]byte{
		"SKILL.md":        []byte("x"),
		"scripts/go.sh":   []byte("#!/bin/sh"),
		"references/a.md": []byte("ref"),
	}}
	out := SkillFiles("skills", art, content)
	if len(out) != 3 {
		t.Fatalf("targets = %d, want 3", len(out))
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].RelPath > out[i].RelPath {
			t.Errorf("not sorted: %s > %s", out[i-1].RelPath, out[i].RelPath)
		}
	}
	for _, tf := range out {
		if tf.RelPath == "skills/demo/scripts/go.sh" && tf.Mode&0o100 == 0 {
			t.Error("script not executable")
		}
	}
}

func TestSettingsFragment_ReturnsNilForOtherAgentsFragment(t *testing.T) {
	content := domain.Content{Files: map[string][]byte{"claude-code.yaml": []byte("a: 1\n")}}
	frag, err := SettingsFragment("gemini-cli.yaml", content)
	if err != nil {
		t.Fatalf("SettingsFragment() error = %v", err)
	}
	if frag != nil {
		t.Errorf("expected nil fragment for a file addressed to another agent, got %s", frag)
	}
}

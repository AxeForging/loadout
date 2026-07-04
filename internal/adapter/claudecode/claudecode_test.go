package claudecode

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
)

func TestDetect_NotInstalledWhenHomeMissing(t *testing.T) {
	a := NewWithHome(filepath.Join(t.TempDir(), ".claude"))
	det, err := a.Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if det.Installed {
		t.Error("Detect() reported installed for a missing home")
	}
}

func TestRender_SkillMapsFilesUnderSkillsDirWithExecutableScripts(t *testing.T) {
	art := domain.Artifact{ID: "code-review", Kind: domain.KindSkill}
	content := domain.Content{Files: map[string][]byte{
		"SKILL.md":       []byte("---\nname: code-review\n---\n"),
		"scripts/run.sh": []byte("#!/bin/sh\n"),
	}}

	targets, err := New().Render(art, content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	byPath := indexTargets(targets)

	md, ok := byPath["skills/code-review/SKILL.md"]
	if !ok {
		t.Fatalf("SKILL.md target missing, got %v", paths(targets))
	}
	if md.Merge != adapter.MergeReplace {
		t.Errorf("SKILL.md merge = %q, want replace", md.Merge)
	}
	script, ok := byPath["skills/code-review/scripts/run.sh"]
	if !ok {
		t.Fatal("script target missing")
	}
	if script.Mode&0o100 == 0 {
		t.Errorf("script mode = %v, want executable", script.Mode)
	}
}

func TestRender_InstructionWrapsManagedBlockIntoClaudeMD(t *testing.T) {
	art := domain.Artifact{ID: "style", Kind: domain.KindInstruction}
	content := domain.Content{Files: map[string][]byte{"style.md": []byte("Use tabs.\n")}}

	targets, err := New().Render(art, content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(targets) != 1 || targets[0].RelPath != "CLAUDE.md" {
		t.Fatalf("expected single CLAUDE.md target, got %v", paths(targets))
	}
	if targets[0].Merge != adapter.MergeBlock {
		t.Errorf("merge = %q, want block", targets[0].Merge)
	}
	s := string(targets[0].Content)
	if !strings.Contains(s, "<!-- loadout:begin style -->") || !strings.Contains(s, "Use tabs.") {
		t.Errorf("managed block malformed:\n%s", s)
	}
}

func TestRender_HookEmitsExecutableScriptAndSettingsFragment(t *testing.T) {
	art := domain.Artifact{ID: "commit-lint", Kind: domain.KindHook}
	content := domain.Content{Files: map[string][]byte{
		"hook.yaml": []byte("event: post-tool\nmatcher: \"Bash\"\ntimeoutSeconds: 10\n"),
		"run.sh":    []byte("#!/bin/sh\nexit 0\n"),
	}}

	targets, err := New().Render(art, content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	byPath := indexTargets(targets)

	script, ok := byPath["hooks/loadout-commit-lint.sh"]
	if !ok {
		t.Fatalf("hook script target missing, got %v", paths(targets))
	}
	if script.Mode&0o100 == 0 {
		t.Error("hook script must be executable")
	}

	settings, ok := byPath["settings.json"]
	if !ok {
		t.Fatal("settings.json fragment missing")
	}
	if settings.Merge != adapter.MergeJSON {
		t.Errorf("settings merge = %q, want json-merge", settings.Merge)
	}
	var frag map[string]any
	if err := json.Unmarshal(settings.Content, &frag); err != nil {
		t.Fatalf("fragment is not valid JSON: %v", err)
	}
	hooks := frag["hooks"].(map[string]any)
	entries, ok := hooks["PostToolUse"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("expected one PostToolUse entry, got %v", hooks)
	}
	entry := entries[0].(map[string]any)
	if entry["matcher"] != "Bash" {
		t.Errorf("matcher = %v, want Bash", entry["matcher"])
	}
	inner := entry["hooks"].([]any)[0].(map[string]any)
	if inner["command"] != "~/.claude/hooks/loadout-commit-lint.sh" {
		t.Errorf("command = %v", inner["command"])
	}
	if inner["timeout"] != float64(10) {
		t.Errorf("timeout = %v, want 10", inner["timeout"])
	}
}

func TestRender_HookRejectsUnknownEvent(t *testing.T) {
	art := domain.Artifact{ID: "bad", Kind: domain.KindHook}
	content := domain.Content{Files: map[string][]byte{
		"hook.yaml": []byte("event: teleport\n"),
		"run.sh":    []byte("#!/bin/sh\n"),
	}}
	if _, err := New().Render(art, content); err == nil {
		t.Fatal("expected error for unknown hook event")
	}
}

func TestRender_SettingsFragmentConvertsYAMLToJSONMerge(t *testing.T) {
	art := domain.Artifact{ID: "prefs", Kind: domain.KindSettings}
	content := domain.Content{Files: map[string][]byte{
		"claude-code.yaml": []byte("env:\n  FOO: bar\n"),
	}}

	targets, err := New().Render(art, content)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if len(targets) != 1 || targets[0].RelPath != "settings.json" || targets[0].Merge != adapter.MergeJSON {
		t.Fatalf("unexpected targets: %v", paths(targets))
	}
	var frag map[string]any
	if err := json.Unmarshal(targets[0].Content, &frag); err != nil {
		t.Fatalf("fragment not valid JSON: %v", err)
	}
	if frag["env"].(map[string]any)["FOO"] != "bar" {
		t.Errorf("fragment = %v", frag)
	}
}

func indexTargets(targets []adapter.TargetFile) map[string]adapter.TargetFile {
	out := map[string]adapter.TargetFile{}
	for _, tf := range targets {
		out[tf.RelPath] = tf
	}
	return out
}

func paths(targets []adapter.TargetFile) []string {
	out := make([]string, 0, len(targets))
	for _, tf := range targets {
		out = append(out, tf.RelPath)
	}
	return out
}

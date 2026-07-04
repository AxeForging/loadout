// Package claudecode adapts canonical artifacts to Claude Code's home layout:
// skills/<name>/SKILL.md, commands/<name>.md, hooks scripts plus a hooks entry
// merged into settings.json, and instructions as a managed block in CLAUDE.md.
package claudecode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/goccy/go-yaml"
)

// hookEvents maps canonical hook events to Claude Code's native names.
var hookEvents = map[string]string{
	"pre-tool":      "PreToolUse",
	"post-tool":     "PostToolUse",
	"prompt-submit": "UserPromptSubmit",
	"stop":          "Stop",
}

type Adapter struct {
	// homeOverride is used by tests; empty means $HOME/.claude.
	homeOverride string
}

func New() *Adapter { return &Adapter{} }

// NewWithHome constructs an adapter rooted at an explicit home (tests).
func NewWithHome(home string) *Adapter { return &Adapter{homeOverride: home} }

func (a *Adapter) ID() domain.AgentID  { return domain.AgentClaudeCode }
func (a *Adapter) DisplayName() string { return "Claude Code" }

func (a *Adapter) home() (string, error) {
	if a.homeOverride != "" {
		return a.homeOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

func (a *Adapter) Detect(ctx context.Context) (adapter.Detection, error) {
	home, err := a.home()
	if err != nil {
		return adapter.Detection{}, err
	}
	info, err := os.Stat(home)
	installed := err == nil && info.IsDir()
	return adapter.Detection{Installed: installed, Home: home}, nil
}

func (a *Adapter) Supports(kind domain.ArtifactKind) bool {
	switch kind {
	case domain.KindSkill, domain.KindInstruction, domain.KindCommand, domain.KindHook, domain.KindSettings:
		return true
	}
	return false
}

func (a *Adapter) Owned(home string) ([]string, error) {
	return []string{"skills/", "commands/", "hooks/loadout-"}, nil
}

func (a *Adapter) Render(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	switch art.Kind {
	case domain.KindSkill:
		return renderSkill(art, content), nil
	case domain.KindInstruction:
		return renderInstruction(art, content)
	case domain.KindCommand:
		return renderCommand(art, content)
	case domain.KindHook:
		return renderHook(art, content)
	case domain.KindSettings:
		return renderSettings(content)
	}
	return nil, fmt.Errorf("claude-code: unsupported artifact kind %q", art.Kind)
}

func renderSkill(art domain.Artifact, content domain.Content) []adapter.TargetFile {
	out := make([]adapter.TargetFile, 0, len(content.Files))
	for rel, raw := range content.Files {
		mode := os.FileMode(0o644)
		if strings.HasPrefix(rel, "scripts/") || strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		out = append(out, adapter.TargetFile{
			RelPath: path.Join("skills", art.ID, rel),
			Content: raw,
			Mode:    mode,
			Merge:   adapter.MergeReplace,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

func renderInstruction(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	body, err := singleFile(art, content)
	if err != nil {
		return nil, err
	}
	return []adapter.TargetFile{{
		RelPath: "CLAUDE.md",
		Content: adapter.WrapBlock(art.ID, body),
		Mode:    0o644,
		Merge:   adapter.MergeBlock,
	}}, nil
}

func renderCommand(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	body, err := singleFile(art, content)
	if err != nil {
		return nil, err
	}
	return []adapter.TargetFile{{
		RelPath: path.Join("commands", art.ID+".md"),
		Content: body,
		Mode:    0o644,
		Merge:   adapter.MergeReplace,
	}}, nil
}

func renderHook(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	rawSpec, ok := content.Files["hook.yaml"]
	if !ok {
		return nil, fmt.Errorf("hook %s: missing hook.yaml", art.ID)
	}
	spec, err := domain.ParseHookSpec(rawSpec)
	if err != nil {
		return nil, fmt.Errorf("hook %s: %w", art.ID, err)
	}
	event, ok := hookEvents[spec.Event]
	if !ok {
		return nil, fmt.Errorf("hook %s: unknown event %q", art.ID, spec.Event)
	}
	script, ok := content.Files[spec.Script]
	if !ok {
		return nil, fmt.Errorf("hook %s: missing script %s", art.ID, spec.Script)
	}

	scriptRel := path.Join("hooks", "loadout-"+art.ID+".sh")
	entry := map[string]any{
		"type":    "command",
		"command": "~/.claude/" + scriptRel,
	}
	if spec.TimeoutSeconds > 0 {
		entry["timeout"] = spec.TimeoutSeconds
	}
	matcherEntry := map[string]any{"hooks": []any{entry}}
	if spec.Matcher != "" {
		matcherEntry["matcher"] = spec.Matcher
	}
	fragment, err := json.Marshal(map[string]any{
		"hooks": map[string]any{event: []any{matcherEntry}},
	})
	if err != nil {
		return nil, err
	}

	return []adapter.TargetFile{
		{RelPath: scriptRel, Content: script, Mode: 0o755, Merge: adapter.MergeReplace},
		{RelPath: "settings.json", Content: fragment, Mode: 0o644, Merge: adapter.MergeJSON},
	}, nil
}

func renderSettings(content domain.Content) ([]adapter.TargetFile, error) {
	raw, ok := content.Files["claude-code.yaml"]
	if !ok {
		return nil, fmt.Errorf("settings fragment: missing claude-code.yaml")
	}
	fragment, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("settings fragment: %w", err)
	}
	return []adapter.TargetFile{{
		RelPath: "settings.json",
		Content: fragment,
		Mode:    0o644,
		Merge:   adapter.MergeJSON,
	}}, nil
}

// singleFile returns the sole file of a single-file artifact.
func singleFile(art domain.Artifact, content domain.Content) ([]byte, error) {
	if len(content.Files) != 1 {
		return nil, fmt.Errorf("artifact %s: expected exactly one file, got %d", art.ID, len(content.Files))
	}
	for _, raw := range content.Files {
		return raw, nil
	}
	return nil, nil
}

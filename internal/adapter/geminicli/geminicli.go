// Package geminicli adapts canonical artifacts to Gemini CLI's home layout:
// skills under ~/.gemini/skills/, instructions as a managed block in
// ~/.gemini/GEMINI.md, commands as skills, and settings fragments merged
// into ~/.gemini/settings.json.
package geminicli

import (
	"context"
	"os"
	"path/filepath"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
)

type Adapter struct {
	homeOverride string
}

func New() *Adapter { return &Adapter{} }

// NewWithHome constructs an adapter rooted at an explicit home (tests).
func NewWithHome(home string) *Adapter { return &Adapter{homeOverride: home} }

func (a *Adapter) ID() domain.AgentID  { return domain.AgentGeminiCLI }
func (a *Adapter) DisplayName() string { return "Gemini CLI" }

func (a *Adapter) home() (string, error) {
	if a.homeOverride != "" {
		return a.homeOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gemini"), nil
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
	case domain.KindSkill, domain.KindInstruction, domain.KindCommand, domain.KindSettings, domain.KindMCP:
		return true
	}
	return false
}

func (a *Adapter) Owned(home string) ([]string, error) {
	return []string{"skills/"}, nil
}

func (a *Adapter) Render(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	switch art.Kind {
	case domain.KindSkill:
		return adapter.SkillFiles("skills", art, content), nil
	case domain.KindInstruction:
		return adapter.InstructionBlock("GEMINI.md", art, content)
	case domain.KindCommand:
		return adapter.CommandSkillFiles("skills", art, content)
	case domain.KindSettings:
		fragment, err := adapter.SettingsFragment("gemini-cli.yaml", content)
		if err != nil || fragment == nil {
			return nil, err
		}
		return []adapter.TargetFile{{
			RelPath: "settings.json",
			Content: fragment,
			Mode:    0o644,
			Merge:   adapter.MergeJSON,
		}}, nil
	case domain.KindMCP:
		return renderMCP(art, content)
	}
	return nil, adapter.ErrUnsupportedKind(domain.AgentGeminiCLI, art.Kind)
}

// renderMCP merges one mcpServers entry into Gemini CLI settings: user scope
// in ~/.gemini/settings.json, project scope in <project>/.gemini/settings.json.
// Gemini names the remote URL field per transport: httpUrl (http) or url (sse).
func renderMCP(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	spec, err := adapter.ParseMCP(art, content)
	if err != nil {
		return nil, err
	}
	server := map[string]any{}
	switch spec.Transport {
	case domain.MCPTransportStdio:
		server["command"] = spec.Command
		if len(spec.Args) > 0 {
			server["args"] = spec.Args
		}
		if len(spec.Env) > 0 {
			server["env"] = spec.Env
		}
	case domain.MCPTransportHTTP:
		server["httpUrl"] = spec.URL
		if len(spec.Headers) > 0 {
			server["headers"] = spec.Headers
		}
	default: // sse
		server["url"] = spec.URL
		if len(spec.Headers) > 0 {
			server["headers"] = spec.Headers
		}
	}
	fragment, err := adapter.MCPServersFragment(art.ID, server)
	if err != nil {
		return nil, err
	}
	target := adapter.TargetFile{
		RelPath: "settings.json",
		Content: fragment,
		Mode:    0o644,
		Merge:   adapter.MergeJSON,
	}
	if spec.Scope == domain.MCPScopeProject {
		target.RelPath = ".gemini/settings.json"
		target.Root = adapter.RootProject
	}
	return []adapter.TargetFile{target}, nil
}

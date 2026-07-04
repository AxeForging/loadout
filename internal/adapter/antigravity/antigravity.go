// Package antigravity adapts canonical artifacts to Google Antigravity's
// global layout. Antigravity shares the ~/.gemini directory with Gemini CLI:
// its skills live under ~/.gemini/antigravity-cli/skills/ and its global
// instructions are the shared ~/.gemini/GEMINI.md. Project-scoped artifacts
// (.agents/rules, .agents/workflows) are on the roadmap.
package antigravity

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

// NewWithHome constructs an adapter rooted at an explicit ~/.gemini-style
// home (tests).
func NewWithHome(home string) *Adapter { return &Adapter{homeOverride: home} }

func (a *Adapter) ID() domain.AgentID  { return domain.AgentAntigravity }
func (a *Adapter) DisplayName() string { return "Antigravity" }

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
	// Antigravity is present when its CLI directory exists inside ~/.gemini.
	info, err := os.Stat(filepath.Join(home, "antigravity-cli"))
	installed := err == nil && info.IsDir()
	return adapter.Detection{Installed: installed, Home: home}, nil
}

func (a *Adapter) Supports(kind domain.ArtifactKind) bool {
	switch kind {
	case domain.KindSkill, domain.KindInstruction:
		return true
	}
	return false
}

func (a *Adapter) Owned(home string) ([]string, error) {
	return []string{"antigravity-cli/skills/"}, nil
}

func (a *Adapter) Render(art domain.Artifact, content domain.Content) ([]adapter.TargetFile, error) {
	switch art.Kind {
	case domain.KindSkill:
		return adapter.SkillFiles("antigravity-cli/skills", art, content), nil
	case domain.KindInstruction:
		// Shared with Gemini CLI: if both agents are installed they manage
		// the same block in the same file, which converges to identical
		// content.
		return adapter.InstructionBlock("GEMINI.md", art, content)
	}
	return nil, adapter.ErrUnsupportedKind(domain.AgentAntigravity, art.Kind)
}

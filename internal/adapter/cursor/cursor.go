// Package cursor adapts canonical artifacts to Cursor's global home layout:
// skills under ~/.cursor/skills/ (Cursor treats skills as slash-capable, so
// commands render as skills too). Rules (.cursor/rules/*.mdc) are
// project-scoped and land with project-scope support — see ROADMAP.
package cursor

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

func (a *Adapter) ID() domain.AgentID  { return domain.AgentCursor }
func (a *Adapter) DisplayName() string { return "Cursor" }

func (a *Adapter) home() (string, error) {
	if a.homeOverride != "" {
		return a.homeOverride, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cursor"), nil
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
	case domain.KindSkill, domain.KindCommand:
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
	case domain.KindCommand:
		return adapter.CommandSkillFiles("skills", art, content)
	}
	return nil, adapter.ErrUnsupportedKind(domain.AgentCursor, art.Kind)
}

// Package tui is the loadout dashboard: five tabs (Overview, Store, Agents,
// Sources, Log) over the same engine and source manager the CLI uses. All
// data loads happen in tea.Cmds; the sync action is gated behind an explicit
// confirmation modal — the TUI never writes without it.
package tui

import (
	"context"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

// Deps are the services the dashboard renders.
type Deps struct {
	Engine  *engine.Engine
	Sources *source.Manager
}

// Run starts the dashboard and blocks until the user quits.
func Run(ctx context.Context, deps Deps) error {
	m := NewModel(deps)
	_, err := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen()).Run()
	return err
}

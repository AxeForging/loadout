// Package app assembles the root CLI command.
package app

import (
	"github.com/AxeForging/loadout/internal/build"
	icli "github.com/AxeForging/loadout/internal/cli"
	"github.com/urfave/cli/v3"
)

// New constructs the root CLI application.
func New() *cli.Command {
	return &cli.Command{
		Name:    "loadout",
		Usage:   "Your agentic gear kit — one canonical store of skills, hooks and instructions, synced into every coding agent",
		Version: build.Version,
		Description: `loadout keeps AI-agent configs (skills, instructions, slash commands,
hooks, settings) in one canonical, git-friendly store and installs them into
each coding agent's native home (claude-code, codex, gemini-cli, antigravity,
cursor) through pluggable adapters. It detects drift, never writes without
confirmation, and user-authored content outside loadout's managed blocks is
never touched.

Workflow:
  1. loadout init
  2. loadout new skill my-workflow
  3. loadout status
  4. loadout sync`,
		Flags:    icli.GlobalFlags(),
		Before:   icli.Before,
		Commands: icli.Commands(),
	}
}

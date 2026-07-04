package cli

import (
	"context"

	"github.com/AxeForging/loadout/internal/tui"
	"github.com/urfave/cli/v3"
)

func tuiCmd() *cli.Command {
	return &cli.Command{
		Name:    "tui",
		Aliases: []string{"dash"},
		Usage:   "Interactive dashboard: drift matrix, store browser, sources and gated sync",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			mgr, err := newSourceManager(cmd)
			if err != nil {
				return err
			}
			// Share the already-opened store so both views stay consistent.
			mgr.Store = eng.Store
			return tui.Run(ctx, tui.Deps{Engine: eng, Sources: mgr})
		},
	}
}

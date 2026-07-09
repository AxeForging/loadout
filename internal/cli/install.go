package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func installCmd() *cli.Command {
	return &cli.Command{
		Name:      "install",
		Usage:     "First-time full install of the store into one agent",
		ArgsUsage: "<agent>",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Apply without the interactive confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show the plan and exit without writing",
			},
			forceFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			agent := cmd.Args().First()
			if agent == "" {
				return fmt.Errorf("usage: loadout install <agent> (see 'loadout agents')")
			}
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			if _, ok := eng.Registry.Get(domain.AgentID(agent)); !ok {
				return fmt.Errorf("unknown agent %q (see 'loadout agents')", agent)
			}
			return runSync(ctx, cmd, eng, engine.Filter{Agents: []domain.AgentID{domain.AgentID(agent)}})
		},
	}
}

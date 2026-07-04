package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func syncCmd() *cli.Command {
	return &cli.Command{
		Name:      "sync",
		Usage:     "Install or update artifacts in agent homes (always asks before writing)",
		ArgsUsage: "[artifact...]",
		Flags: []cli.Flag{
			agentFlag(),
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "Show the plan and exit without writing",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Apply without the interactive confirmation prompt",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			return runSync(ctx, cmd, eng, filterFromFlags(cmd, cmd.Args().Slice()))
		},
	}
}

// runSync is shared by sync and install: plan, print, confirm, apply.
func runSync(ctx context.Context, cmd *cli.Command, eng *engine.Engine, f engine.Filter) error {
	plan, err := eng.Plan(ctx, f)
	if err != nil {
		return err
	}
	printSkips(plan)
	changes := plan.Changes()
	if len(changes) == 0 {
		fmt.Println("Everything in sync — nothing to do.")
		return nil
	}

	fmt.Printf("Plan: %d change(s)\n", len(changes))
	printChanges(plan)
	if cmd.Bool("dry-run") {
		fmt.Println("\nDry run — nothing written.")
		return nil
	}

	n, err := eng.Apply(ctx, plan, confirmApply(cmd))
	if errors.Is(err, engine.ErrNotConfirmed) {
		fmt.Println("Aborted — nothing written.")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Applied %d change(s).\n", n)
	maybeUpdateHint(ctx, cmd)
	return nil
}

package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func diffCmd() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show what sync would change, as unified diffs",
		ArgsUsage: "[artifact...]",
		Flags:     []cli.Flag{agentFlag(), projectFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			plan, err := eng.Plan(ctx, filterFromFlags(cmd, cmd.Args().Slice()))
			if err != nil {
				return err
			}
			changes := plan.Changes()
			if len(changes) == 0 {
				fmt.Println("Everything in sync — nothing to diff.")
				printSkips(plan)
				return nil
			}
			for _, it := range changes {
				text, err := engine.UnifiedDiff(it)
				if err != nil {
					return err
				}
				fmt.Printf("── %s · %s · %s\n%s\n", it.Agent, it.Artifact.ID, it.RelPath, text)
			}
			printSkips(plan)
			return nil
		},
	}
}

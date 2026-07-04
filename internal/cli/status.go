package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

func statusCmd() *cli.Command {
	return &cli.Command{
		Name:      "status",
		Usage:     "Show the drift matrix: every artifact's sync state per agent",
		ArgsUsage: "[artifact...]",
		Flags:     []cli.Flag{agentFlag()},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			rows, plan, err := eng.Status(ctx, filterFromFlags(cmd, cmd.Args().Slice()))
			if err != nil {
				return err
			}

			if len(rows) == 0 {
				fmt.Println("Nothing to report — the store has no artifacts targeting an installed agent.")
				printSkips(plan)
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "ARTIFACT\tKIND\tAGENT\tSTATE")
			for _, r := range rows {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Artifact, r.Kind, r.Agent, r.State)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			printSkips(plan)
			return nil
		},
	}
}

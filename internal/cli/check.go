package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

func checkCmd() *cli.Command {
	return &cli.Command{
		Name:  "check",
		Usage: "Check registered sources for updates (ls-remote only — nothing is fetched or applied)",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			m, err := newSourceManager(cmd)
			if err != nil {
				return err
			}
			results, err := m.Check(ctx)
			if err != nil {
				return err
			}
			if len(results) == 0 {
				fmt.Println("No sources registered. Add one with 'loadout source add <url>'.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "SOURCE\tPINNED\tREMOTE\tSTATE")
			updates := 0
			for _, r := range results {
				state := "up-to-date"
				remote := fmt.Sprintf("%.10s", r.Remote)
				switch {
				case r.Err != nil:
					state = "error: " + r.Err.Error()
					remote = "-"
				case r.HasUpdate:
					state = "update available"
					updates++
				}
				fmt.Fprintf(w, "%s\t%.10s\t%s\t%s\n", r.Name, r.Pinned, remote, state)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			saveUpdateCache(updates)
			if updates > 0 {
				fmt.Printf("\n%d source(s) have updates — run 'loadout update'.\n", updates)
			}
			return nil
		},
	}
}

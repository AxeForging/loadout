package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

func agentsCmd() *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "List supported agents and whether they are installed on this machine",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "AGENT\tNAME\tINSTALLED\tHOME")
			for _, ad := range newRegistry().All() {
				det, err := ad.Detect(ctx)
				if err != nil {
					fmt.Fprintf(w, "%s\t%s\terror: %v\t\n", ad.ID(), ad.DisplayName(), err)
					continue
				}
				installed := "no"
				if det.Installed {
					installed = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ad.ID(), ad.DisplayName(), installed, det.Home)
			}
			return w.Flush()
		},
	}
}

package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/AxeForging/loadout/internal/runner"
	"github.com/urfave/cli/v3"
)

func agentsCmd() *cli.Command {
	return &cli.Command{
		Name:  "agents",
		Usage: "List supported agents: installed homes and headless (--ai) availability",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(w, "AGENT\tNAME\tINSTALLED\tHEADLESS\tHOME")
			headless := map[string]bool{}
			for _, r := range runner.Registry() {
				headless[r.ID()] = r.Available()
			}
			// Adapter IDs map onto runner binaries where a headless mode
			// exists; the gemini ecosystem now ships as agy with the old
			// gemini binary as fallback.
			runnerFor := map[string][]string{
				"claude-code": {"claude"},
				"codex":       {"codex"},
				"gemini-cli":  {"agy", "gemini"},
				"antigravity": {"agy"},
			}
			for _, ad := range newRegistry().All() {
				det, err := ad.Detect(ctx)
				if err != nil {
					fmt.Fprintf(w, "%s\t%s\terror: %v\t\t\n", ad.ID(), ad.DisplayName(), err)
					continue
				}
				installed := "no"
				if det.Installed {
					installed = "yes"
				}
				hl := "-"
				if bins, ok := runnerFor[string(ad.ID())]; ok {
					hl = "no"
					for _, bin := range bins {
						if headless[bin] {
							hl = "yes"
							break
						}
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ad.ID(), ad.DisplayName(), installed, hl, det.Home)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			fmt.Println("\nHEADLESS = usable by 'loadout new skill --ai' and 'loadout wizard --ai' via your logged-in session (no API key).")
			return nil
		},
	}
}

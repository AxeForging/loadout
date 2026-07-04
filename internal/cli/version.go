package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/build"
	"github.com/urfave/cli/v3"
)

func versionCmd() *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "Print build version information",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			fmt.Printf("Version:  %s\n", build.Version)
			fmt.Printf("Commit:   %s\n", build.Commit)
			fmt.Printf("Date:     %s\n", build.Date)
			fmt.Printf("Built by: %s\n", build.BuiltBy)
			return nil
		},
	}
}

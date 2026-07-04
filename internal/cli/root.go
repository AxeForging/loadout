// Package cli defines all loadout commands: one file per command, registered
// in Commands(). Commands orchestrate store/engine/adapters and own all
// terminal I/O; business logic lives in the packages they call.
package cli

import (
	"context"

	"github.com/AxeForging/loadout/internal/logging"
	"github.com/urfave/cli/v3"
)

// Commands returns all subcommands to register on the root app.
func Commands() []*cli.Command {
	return []*cli.Command{
		initCmd(),
		statusCmd(),
		diffCmd(),
		syncCmd(),
		installCmd(),
		newCmd(),
		addCmd(),
		agentsCmd(),
		doctorCmd(),
		versionCmd(),
		completionCmd(),
	}
}

// GlobalFlags returns flags available on every command.
func GlobalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "log-level",
			Usage:   "Log verbosity: debug, info, warn, error",
			Value:   "info",
			Sources: cli.EnvVars("LOADOUT_LOG_LEVEL"),
		},
		&cli.BoolFlag{
			Name:  "no-color",
			Usage: "Disable colored output",
		},
		&cli.StringFlag{
			Name:    "store",
			Usage:   "Path to the canonical store (overrides config)",
			Sources: cli.EnvVars("LOADOUT_STORE"),
		},
	}
}

// Before is the global Before hook — sets up logging before any command runs.
func Before(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	logging.Setup(cmd.String("log-level"), cmd.Bool("no-color"))
	return ctx, nil
}

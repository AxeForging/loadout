package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/urfave/cli/v3"
)

func initCmd() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Create the canonical store (git-friendly directory with a starter manifest)",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "name",
				Usage: "Name recorded in the store manifest",
				Value: "my-loadout",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			path, err := resolveStorePath(cmd)
			if err != nil {
				return err
			}
			s, err := store.Init(path, cmd.String("name"))
			if err != nil {
				return err
			}
			fmt.Printf("Initialized store %q at %s\n\n", s.Manifest.Name, s.Root)
			fmt.Println("Next steps:")
			fmt.Println("  loadout new skill <name>        scaffold a skill")
			fmt.Println("  loadout new instruction <name>  scaffold shared instructions")
			fmt.Println("  loadout status                  see what would sync where")
			fmt.Println("  loadout sync                    install into detected agents")
			return nil
		},
	}
}

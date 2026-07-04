package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/urfave/cli/v3"
)

func profileCmd() *cli.Command {
	return &cli.Command{
		Name:  "profile",
		Usage: "Save and reuse named wizard answer sets (one org, one project type — commit them, share them)",
		Commands: []*cli.Command{
			{
				Name:  "list",
				Usage: "List profiles saved in the store",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					names, err := wizard.ListProfiles(s)
					if err != nil {
						return err
					}
					if len(names) == 0 {
						fmt.Println("No profiles saved. Create one with 'loadout wizard --save-profile <name>'.")
						return nil
					}
					for _, name := range names {
						fmt.Println(name)
					}
					return nil
				},
			},
			{
				Name:      "show",
				Usage:     "Print a saved profile's answers",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout profile show <name>")
					}
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					a, err := wizard.LoadProfile(s, name)
					if err != nil {
						return err
					}
					fmt.Print(a.Summary())
					return nil
				},
			},
			{
				Name:      "rm",
				Usage:     "Delete a saved profile",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout profile rm <name>")
					}
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					if err := wizard.RemoveProfile(s, name); err != nil {
						return err
					}
					fmt.Printf("Removed profile %q\n", name)
					return nil
				},
			},
		},
	}
}

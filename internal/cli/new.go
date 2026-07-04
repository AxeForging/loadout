package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/urfave/cli/v3"
)

func newCmd() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "Scaffold a new artifact in the store",
		ArgsUsage: "<skill|instruction|command|hook> <name>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			kind := domain.ArtifactKind(cmd.Args().Get(0))
			name := cmd.Args().Get(1)
			if kind == "" || name == "" {
				return fmt.Errorf("usage: loadout new <skill|instruction|command|hook> <name>")
			}
			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			a, err := s.New(kind, name)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s %q at %s\n", a.Kind, a.ID, filepath.Join(s.Root, a.Path))
			fmt.Println("Edit it, then run 'loadout sync' to install it.")
			return nil
		},
	}
}

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/AxeForging/loadout/internal/config"
	"github.com/AxeForging/loadout/internal/source"
	"github.com/urfave/cli/v3"
)

func newSourceManager(cmd *cli.Command) (*source.Manager, error) {
	s, err := openStore(cmd)
	if err != nil {
		return nil, err
	}
	data, err := config.DataDir()
	if err != nil {
		return nil, err
	}
	return &source.Manager{Store: s, CacheDir: filepath.Join(data, "sources")}, nil
}

func sourceCmd() *cli.Command {
	return &cli.Command{
		Name:  "source",
		Usage: "Track skills from external git repositories",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Register a repository, import its skills, pin the commit",
				ArgsUsage: "<url>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "name", Usage: "Source name (default: repository basename)"},
					&cli.StringFlag{Name: "ref", Usage: "Branch or tag (default: remote default branch)"},
					&cli.StringFlag{Name: "subdir", Usage: "Only discover skills under this subdirectory"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					url := cmd.Args().First()
					if url == "" {
						return fmt.Errorf("usage: loadout source add <url>")
					}
					name := cmd.String("name")
					if name == "" {
						name = strings.TrimSuffix(filepath.Base(url), ".git")
					}
					m, err := newSourceManager(cmd)
					if err != nil {
						return err
					}
					var res source.AddResult
					if err := withStoreLock(func() error {
						var addErr error
						res, addErr = m.Add(ctx, name, url, cmd.String("ref"), cmd.String("subdir"))
						return addErr
					}); err != nil {
						return err
					}
					fmt.Printf("Added source %q pinned at %.10s\n", name, res.Ref.Commit)
					for _, id := range res.Added {
						fmt.Printf("  + skill %s\n", id)
					}
					for _, id := range res.Skipped {
						fmt.Printf("  ~ skipped %s (an artifact with this name already exists)\n", id)
					}
					fmt.Println("Run 'loadout sync' to install the imported skills.")
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List registered sources and their pins",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					m, err := newSourceManager(cmd)
					if err != nil {
						return err
					}
					refs, err := m.Lock()
					if err != nil {
						return err
					}
					if len(refs) == 0 {
						fmt.Println("No sources registered. Add one with 'loadout source add <url>'.")
						return nil
					}
					w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
					fmt.Fprintln(w, "NAME\tURL\tREF\tPINNED\tSUBDIR")
					for _, r := range refs {
						ref := r.Ref
						if ref == "" {
							ref = "(default)"
						}
						fmt.Fprintf(w, "%s\t%s\t%s\t%.10s\t%s\n", r.Name, r.URL, ref, r.Commit, r.Subdir)
					}
					return w.Flush()
				},
			},
			{
				Name:      "remove",
				Usage:     "Unregister a source (artifacts stay unless --purge)",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "purge", Usage: "Also remove artifacts imported from this source"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout source remove <name>")
					}
					m, err := newSourceManager(cmd)
					if err != nil {
						return err
					}
					var removed []string
					if err := withStoreLock(func() error {
						var rmErr error
						removed, rmErr = m.Remove(name, cmd.Bool("purge"))
						return rmErr
					}); err != nil {
						return err
					}
					fmt.Printf("Removed source %q\n", name)
					for _, id := range removed {
						fmt.Printf("  - purged skill %s (already-installed copies in agent homes are untouched)\n", id)
					}
					if !cmd.Bool("purge") {
						fmt.Println("Imported artifacts were kept. Re-run with --purge to remove them from the store.")
					}
					return nil
				},
			},
		},
	}
}

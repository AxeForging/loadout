package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/AxeForging/loadout/internal/config"
	"github.com/AxeForging/loadout/internal/domain"
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

// parseAliases turns repeated --alias upstream=local flags into a map.
func parseAliases(raw []string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := map[string]string{}
	for _, pair := range raw {
		upstream, local, ok := strings.Cut(pair, "=")
		if !ok || upstream == "" || local == "" {
			return nil, fmt.Errorf("invalid --alias %q: use <upstream-skill>=<local-name>", pair)
		}
		out[upstream] = local
	}
	return out, nil
}

// summarizeStates renders a source's offline skill states for the list
// column: "3 ok" / "2 ok, 1 edited" / "1 edited, 2 legacy" / "-" when the
// source owns nothing.
func summarizeStates(states map[string]source.SkillState) string {
	counts := map[source.SkillState]int{}
	for _, st := range states {
		counts[st]++
	}
	var parts []string
	for _, st := range []source.SkillState{source.SkillOK, source.SkillEdited, source.SkillLegacy} {
		if counts[st] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[st], st))
		}
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, ", ")
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
					&cli.StringSliceFlag{Name: "alias", Usage: "Import a skill under a different local name: --alias <upstream>=<local> (repeatable); updates keep tracking the real upstream skill"},
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
					aliases, err := parseAliases(cmd.StringSlice("alias"))
					if err != nil {
						return err
					}
					m, err := newSourceManager(cmd)
					if err != nil {
						return err
					}
					var res source.AddResult
					if err := withStoreLock(func() error {
						var addErr error
						res, addErr = m.Add(ctx, name, url, cmd.String("ref"), cmd.String("subdir"), aliases)
						return addErr
					}); err != nil {
						return err
					}
					fmt.Printf("Added source %q pinned at %.10s\n", name, res.Ref.Commit)
					for _, id := range res.Added {
						fmt.Printf("  + skill %s\n", id)
					}
					for _, id := range res.Skipped {
						fmt.Printf("  ~ skipped %s (an artifact with this name already exists — import it under another name with 'loadout source import %s %s --as <local-name>')\n", id, name, id)
					}
					fmt.Println("Run 'loadout sync' to install the imported skills.")
					return nil
				},
			},
			{
				Name:      "import",
				Usage:     "Import one skill from a registered source under a local alias (keeps tracking the real upstream skill)",
				ArgsUsage: "<source> <upstream-skill>",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "as", Usage: "Local artifact name (default: the upstream name)"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					src, upstream := cmd.Args().Get(0), cmd.Args().Get(1)
					if src == "" || upstream == "" {
						return fmt.Errorf("usage: loadout source import <source> <upstream-skill> [--as <local-name>]")
					}
					m, err := newSourceManager(cmd)
					if err != nil {
						return err
					}
					var art domain.Artifact
					if err := withStoreLock(func() error {
						var impErr error
						art, impErr = m.ImportSkill(ctx, src, upstream, cmd.String("as"))
						return impErr
					}); err != nil {
						return err
					}
					if art.SourceName != "" {
						fmt.Printf("Imported %s as %q — updates keep tracking upstream %q.\n", upstream, art.ID, art.SourceName)
					} else {
						fmt.Printf("Imported %s from source %q.\n", art.ID, src)
					}
					fmt.Println("Run 'loadout sync' to install it.")
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
					fmt.Fprintln(w, "NAME\tURL\tREF\tPINNED\tSUBDIR\tSTATE")
					for _, r := range refs {
						ref := r.Ref
						if ref == "" {
							ref = "(default)"
						}
						states, err := m.SkillStates(r.Name)
						if err != nil {
							return err
						}
						fmt.Fprintf(w, "%s\t%s\t%s\t%.10s\t%s\t%s\n", r.Name, r.URL, ref, r.Commit, r.Subdir, summarizeStates(states))
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

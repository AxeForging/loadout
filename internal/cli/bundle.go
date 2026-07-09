package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AxeForging/loadout/internal/build"
	"github.com/AxeForging/loadout/internal/bundle"
	"github.com/urfave/cli/v3"
)

func bundleCmd() *cli.Command {
	return &cli.Command{
		Name:  "bundle",
		Usage: "Export/import the whole store as one shareable artifact",
		Commands: []*cli.Command{
			{
				Name:  "export",
				Usage: "Export the store as a bundle directory or a single .loadout.tar.gz",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "out", Aliases: []string{"o"}, Usage: "Destination path (default: derived from the store name)"},
					&cli.BoolFlag{Name: "archive", Usage: "Pack into a single .loadout.tar.gz instead of a directory"},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					createdBy := "loadout " + build.Version
					out := cmd.String("out")
					if cmd.Bool("archive") {
						if out == "" {
							out = s.Manifest.Name + ".loadout.tar.gz"
						}
						meta, err := bundle.ExportArchive(s, out, createdBy)
						if err != nil {
							return err
						}
						fmt.Printf("Exported %q (%d artifact(s)) to %s\n", meta.Name, meta.Artifacts, out)
						return nil
					}
					if out == "" {
						out = s.Manifest.Name + "-loadout"
					}
					meta, err := bundle.Export(s, out, createdBy)
					if err != nil {
						return err
					}
					fmt.Printf("Exported %q (%d artifact(s)) to %s/\n", meta.Name, meta.Artifacts, out)
					fmt.Println("Share the directory (or commit it to a repo); teammates run 'loadout bundle import <path>'.")
					return nil
				},
			},
			{
				Name:      "import",
				Usage:     "Validate a bundle, show the merge plan, apply after confirmation",
				ArgsUsage: "<path>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "on-conflict",
						Usage: "What to do when an artifact ID exists locally with different content: skip or overwrite",
						Value: "skip",
					},
					&cli.BoolFlag{
						Name:    "yes",
						Aliases: []string{"y"},
						Usage:   "Apply without the interactive confirmation prompt",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					src := cmd.Args().First()
					if src == "" {
						return fmt.Errorf("usage: loadout bundle import <path>")
					}
					policy := cmd.String("on-conflict")
					if policy != "skip" && policy != "overwrite" {
						return fmt.Errorf("--on-conflict must be skip or overwrite, got %q", policy)
					}
					overwrite := policy == "overwrite"

					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					plan, err := bundle.PlanImport(s, src)
					if err != nil {
						return err
					}
					defer plan.Close()

					fmt.Printf("Bundle %q by %s (%s)\n", plan.Meta.Name, plan.Meta.CreatedBy, plan.Meta.CreatedAt.Format("2006-01-02"))
					for _, a := range plan.New {
						fmt.Printf("  + %s %s\n", a.Kind, a.ID)
					}
					for _, a := range plan.Conflicts {
						action := "skip (exists locally with different content)"
						if overwrite {
							action = "OVERWRITE local version"
						}
						fmt.Printf("  ! %s %s — %s\n", a.Kind, a.ID, action)
					}
					for _, src := range plan.Sources {
						fmt.Printf("  + source %s (%s)\n", src.Name, src.URL)
					}
					for _, name := range plan.NewProfiles {
						fmt.Printf("  + profile %s\n", name)
					}
					for _, name := range plan.ConflictProfiles {
						action := "skip (exists locally with different content)"
						if overwrite {
							action = "OVERWRITE local version"
						}
						fmt.Printf("  ! profile %s — %s\n", name, action)
					}
					if n := len(plan.NewLibraryEntries); n > 0 {
						noun := "entry"
						if n != 1 {
							noun = "entries"
						}
						fmt.Printf("  + %d custom-instruction library %s\n", n, noun)
					}
					if len(plan.Identical) > 0 {
						fmt.Printf("  = %d artifact(s) identical, nothing to do\n", len(plan.Identical))
					}
					if plan.Changes(overwrite) == 0 {
						fmt.Println("Nothing to import.")
						return nil
					}

					if !cmd.Bool("yes") && !promptYes(fmt.Sprintf("Merge %d item(s) into the store?", plan.Changes(overwrite))) {
						fmt.Println("Aborted — nothing imported.")
						return nil
					}
					applied, err := bundle.Apply(s, plan, overwrite)
					if err != nil {
						return err
					}
					fmt.Printf("Imported: %s\n", strings.Join(applied, ", "))
					fmt.Println("Run 'loadout sync' to install into your agents.")
					return nil
				},
			},
		},
	}
}

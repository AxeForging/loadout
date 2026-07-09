package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/loadouts"
	"github.com/urfave/cli/v3"
)

func loadoutsCmd() *cli.Command {
	return &cli.Command{
		Name:  "loadouts",
		Usage: "Named, switchable sets of active artifacts",
		Commands: []*cli.Command{
			{
				Name:      "save",
				Usage:     "Save an artifact set as a named loadout (no artifacts given = snapshot everything in the manifest)",
				ArgsUsage: "<name> [artifact...]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout loadouts save <name> [artifact...]")
					}
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					if err := loadouts.EnsureDefault(s); err != nil {
						return err
					}
					ids := cmd.Args().Slice()[1:]
					if len(ids) == 0 {
						for _, a := range s.Manifest.Artifacts {
							ids = append(ids, a.ID)
						}
					}
					l, err := loadouts.SaveLoadout(s, name, ids)
					if err != nil {
						return err
					}
					fmt.Printf("Saved loadout %q with %d artifact(s).\n", l.Name, len(l.Artifacts))
					return nil
				},
			},
			{
				Name:  "list",
				Usage: "List saved loadouts",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					if err := loadouts.EnsureDefault(s); err != nil {
						return err
					}
					names, err := loadouts.ListLoadouts(s)
					if err != nil {
						return err
					}
					active, _ := loadouts.GetActive()
					for _, name := range names {
						marker := "  "
						if name == active {
							marker = "* "
						}
						fmt.Printf("%s%s\n", marker, name)
					}
					return nil
				},
			},
			{
				Name:      "show",
				Usage:     "Print a saved loadout's artifact list",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout loadouts show <name>")
					}
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					l, err := loadouts.LoadLoadout(s, name)
					if err != nil {
						return err
					}
					fmt.Print(l.Summary())
					return nil
				},
			},
			{
				Name:      "switch",
				Usage:     "Install a loadout's artifacts and remove whatever isn't in it (always asks before writing)",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Usage: "Show the plan and exit without writing"},
					&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Apply without the interactive confirmation prompt"},
					forceFlag(),
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout loadouts switch <name>")
					}
					eng, err := newEngine(cmd)
					if err != nil {
						return err
					}
					return runLoadoutSwitch(ctx, cmd, eng, name)
				},
			},
			{
				Name:      "rm",
				Usage:     "Delete a saved loadout's definition (never touches installed artifacts — switch to another loadout for that)",
				ArgsUsage: "<name>",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					name := cmd.Args().First()
					if name == "" {
						return fmt.Errorf("usage: loadout loadouts rm <name>")
					}
					s, err := openStore(cmd)
					if err != nil {
						return err
					}
					active, _ := loadouts.GetActive()
					if err := loadouts.RemoveLoadout(s, name); err != nil {
						return err
					}
					fmt.Printf("Removed loadout %q\n", name)
					if name == active {
						fmt.Println("Note: this was your active loadout — its installed artifacts are untouched. Switch to another loadout if you want them removed.")
					}
					return nil
				},
			},
		},
	}
}

// runLoadoutSwitch is the switch orchestration: diff the target loadout's
// artifacts against what's actually installed (per state.yaml, not a
// remembered "active loadout" name), plan both halves, show one combined
// plan, and apply both under one confirmation.
func runLoadoutSwitch(ctx context.Context, cmd *cli.Command, eng *engine.Engine, name string) error {
	s := eng.Store
	if err := loadouts.EnsureDefault(s); err != nil {
		return err
	}
	target, err := loadouts.LoadLoadout(s, name)
	if err != nil {
		return err
	}

	var validIDs []string
	for _, ref := range target.Artifacts {
		if _, found := s.Artifact(ref.ID); found {
			validIDs = append(validIDs, ref.ID)
		} else {
			fmt.Printf("  ! %s: no longer in the store manifest — skipping install\n", ref.ID)
		}
	}

	// An empty Filter.Artifacts means "match everything" (correct for plain
	// sync, wrong here) — build an empty Plan directly instead when the
	// target loadout resolves to nothing installable.
	var installPlan *engine.Plan
	if len(validIDs) == 0 {
		installPlan = &engine.Plan{}
	} else {
		installPlan, err = eng.Plan(ctx, engine.Filter{Artifacts: validIDs})
		if err != nil {
			return err
		}
	}
	installPlan.OverwriteConflicts = cmd.Bool("force")

	installedIDs, err := eng.InstalledArtifactIDs()
	if err != nil {
		return err
	}
	wantSet := make(map[string]bool, len(validIDs))
	for _, id := range validIDs {
		wantSet[id] = true
	}
	var removeIDs []string
	for _, id := range installedIDs {
		if !wantSet[id] {
			removeIDs = append(removeIDs, id)
		}
	}
	removalPlan, err := eng.PlanRemoval(ctx, removeIDs)
	if err != nil {
		return err
	}

	printSkips(installPlan)
	printChanges(installPlan)
	printRemovals(removalPlan)

	if len(installPlan.Changes()) == 0 && len(removalPlan.Items) == 0 {
		if c := installPlan.Conflicts(); len(c) > 0 && !installPlan.OverwriteConflicts {
			fmt.Printf("\n%d conflicting file(s) exist on disk but were never installed by loadout — skipped.\n"+
				"Re-run with --force to overwrite them (a snapshot is taken first).\n", len(c))
			return nil
		}
		fmt.Printf("Already on loadout %q — nothing to do.\n", name)
		return nil
	}
	if cmd.Bool("dry-run") {
		fmt.Println("\nDry run — nothing written.")
		return nil
	}

	installed, removed, err := eng.ApplySwitch(ctx, installPlan, removalPlan, confirmSwitch(cmd, installPlan, removalPlan))
	if errors.Is(err, engine.ErrNotConfirmed) {
		fmt.Println("Aborted — nothing written.")
		return nil
	}
	if err != nil {
		return err
	}

	if err := loadouts.SetActive(name); err != nil {
		return err
	}
	fmt.Printf("Applied %d install change(s), %d removal change(s).\n", installed, removed)
	return nil
}

package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AxeForging/loadout/internal/source"
	"github.com/urfave/cli/v3"
)

func updateCmd() *cli.Command {
	return &cli.Command{
		Name:      "update",
		Usage:     "Fetch source updates, show what would change in the store, apply after confirmation",
		ArgsUsage: "[source]",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Apply without the interactive confirmation prompt (never discards local edits)",
			},
			&cli.BoolFlag{
				Name:  "force-discard",
				Usage: "Also apply conflicting skills, DISCARDING your local edits in favor of upstream",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			m, err := newSourceManager(cmd)
			if err != nil {
				return err
			}
			names := []string{}
			if arg := cmd.Args().First(); arg != "" {
				names = append(names, arg)
			} else {
				refs, err := m.Lock()
				if err != nil {
					return err
				}
				if len(refs) == 0 {
					fmt.Println("No sources registered. Add one with 'loadout source add <url>'.")
					return nil
				}
				for _, r := range refs {
					names = append(names, r.Name)
				}
			}

			force := cmd.Bool("force-discard")
			applied := 0
			for _, name := range names {
				plan, err := m.PlanUpdate(ctx, name)
				if err != nil {
					return err
				}
				writes := plan.Changes() > 0 || (force && len(plan.Conflicts) > 0)
				if plan.UpToDate && !writes && len(plan.Conflicts) == 0 && len(plan.LocallyEdited) == 0 {
					fmt.Printf("%s: up-to-date at %.10s\n", name, plan.Ref.Commit)
					continue
				}

				if plan.UpToDate {
					fmt.Printf("%s: pin up-to-date at %.10s\n", name, plan.Ref.Commit)
				} else {
					fmt.Printf("%s: %.10s → %.10s\n", name, plan.Ref.Commit, plan.NewCommit)
				}
				for _, line := range plan.Log {
					fmt.Printf("    %s\n", line)
				}
				printUpdatePlan(plan, force)

				if !writes && plan.UpToDate {
					continue // report-only: nothing this run may write
				}
				if !writes {
					fmt.Println("  pin moves, no artifact changes")
				}

				question := fmt.Sprintf("Apply update of %q to the store?", name)
				if force && len(plan.Conflicts) > 0 {
					question = fmt.Sprintf("Apply update of %q, DISCARDING local edits to %d skill(s)?", name, len(plan.Conflicts))
				}
				if !cmd.Bool("yes") && !promptYes(question) {
					fmt.Println("  skipped.")
					continue
				}
				if err := withStoreLock(func() error {
					if err := snapshotStorePaths("source-update", m.Store.Root, m.UpdateWritePaths(plan, force)); err != nil {
						return err
					}
					return m.ApplyUpdate(plan, force)
				}); err != nil {
					return err
				}
				applied++
				fmt.Printf("  applied — pinned at %.10s\n", plan.NewCommit)
			}
			if applied > 0 {
				fmt.Println("\nStore updated. Run 'loadout sync' to roll the changes out to your agents.")
			}
			return nil
		},
	}
}

// printUpdatePlan renders the three-way classification. Vocabulary is shared
// with the web plan view: update / new / locally edited / conflict / owned
// elsewhere / removed.
func printUpdatePlan(plan *source.UpdatePlan, force bool) {
	legacy := map[string]bool{}
	for _, n := range plan.LegacyBaseline {
		legacy[n] = true
	}
	for _, sk := range plan.Changed {
		if legacy[sk.Name] {
			fmt.Printf("  ~ update skill %s (no import baseline recorded — local edits, if any, were undetectable)\n", sk.Label())
		} else {
			fmt.Printf("  ~ update skill %s (upstream changed)\n", sk.Label())
		}
	}
	for _, sk := range plan.Added {
		fmt.Printf("  + new skill %s\n", sk.Name)
	}
	for _, sk := range plan.LocallyEdited {
		fmt.Printf("  ! skill %s locally edited — left alone (upstream unchanged)\n", sk.Label())
	}
	for _, sk := range plan.Conflicts {
		if force {
			fmt.Printf("  !! skill %s — conflict: your local edits WILL BE DISCARDED for upstream's version\n", sk.Label())
		} else {
			fmt.Printf("  !! skill %s — conflict: edited locally AND changed upstream; skipped (inspect with 'loadout diff %s --upstream', apply with --force-discard)\n", sk.Label(), sk.LocalID)
		}
	}
	for _, id := range plan.OwnedElsewhere {
		fmt.Printf("  . skill %s exists locally with a different origin — not touched\n", id)
	}
	for _, id := range plan.RemovedIDs {
		fmt.Printf("  - %s no longer upstream (kept in the store, relabeled origin: local)\n", id)
	}
}

// promptYes asks a y/N question on stdin. This is the store-write
// counterpart of the engine's confirmation gate.
func promptYes(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}

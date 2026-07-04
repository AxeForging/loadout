package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

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
				Usage:   "Apply without the interactive confirmation prompt",
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

			applied := 0
			for _, name := range names {
				plan, err := m.PlanUpdate(ctx, name)
				if err != nil {
					return err
				}
				if plan.UpToDate {
					fmt.Printf("%s: up-to-date at %.10s\n", name, plan.Ref.Commit)
					continue
				}
				fmt.Printf("%s: %.10s → %.10s\n", name, plan.Ref.Commit, plan.NewCommit)
				for _, line := range plan.Log {
					fmt.Printf("    %s\n", line)
				}
				for _, sk := range plan.Changed {
					fmt.Printf("  ~ update skill %s\n", sk.Name)
				}
				for _, sk := range plan.Added {
					fmt.Printf("  + new skill %s\n", sk.Name)
				}
				for _, id := range plan.RemovedIDs {
					fmt.Printf("  - %s no longer upstream (kept in the store, relabeled origin: local)\n", id)
				}
				if plan.Changes() == 0 {
					fmt.Println("  pin moves, no artifact changes")
				}

				if !cmd.Bool("yes") && !promptYes(fmt.Sprintf("Apply update of %q to the store?", name)) {
					fmt.Println("  skipped.")
					continue
				}
				if err := m.ApplyUpdate(plan); err != nil {
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

package cli

import (
	"context"
	"fmt"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func diffCmd() *cli.Command {
	return &cli.Command{
		Name:      "diff",
		Usage:     "Show what sync would change, as unified diffs",
		ArgsUsage: "[artifact...]",
		Flags: []cli.Flag{agentFlag(), projectFlag(), &cli.BoolFlag{
			Name:  "upstream",
			Usage: "Diff a source-imported skill's store copy against the source's remote head instead",
		}},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if cmd.Bool("upstream") {
				return upstreamDiff(ctx, cmd)
			}
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			plan, err := eng.Plan(ctx, filterFromFlags(cmd, cmd.Args().Slice()))
			if err != nil {
				return err
			}
			changes, held := plan.Changes(), plan.Conflicts()
			if len(changes) == 0 && len(held) == 0 {
				fmt.Println("Everything in sync — nothing to diff.")
				printSkips(plan)
				return nil
			}
			for _, it := range changes {
				if err := printItemDiff(it, ""); err != nil {
					return err
				}
			}
			// Held-back items are not in Changes, but they are exactly what a
			// user needs to inspect before choosing adopt or --force: '-' lines
			// are what is on disk now and would be lost.
			for _, it := range held {
				note := " (held back: exists on disk, never installed by loadout — sync --force would write this)"
				if it.LocalEdit {
					note = " (held back: local edits — '-' lines are yours; sync --force would discard them, adopt keeps them)"
				}
				if err := printItemDiff(it, note); err != nil {
					return err
				}
			}
			printSkips(plan)
			return nil
		},
	}
}

func printItemDiff(it engine.Item, note string) error {
	text, err := engine.UnifiedDiff(it)
	if err != nil {
		return err
	}
	fmt.Printf("── %s · %s · %s%s\n%s\n", it.Agent, it.Artifact.ID, it.RelPath, note, text)
	return nil
}

// upstreamDiff handles `loadout diff <skill> --upstream`: the store copy vs
// the skill at the source's remote head — the inspection step before
// deciding on an update or a --force-discard.
func upstreamDiff(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args().Slice()
	if len(args) != 1 {
		return fmt.Errorf("usage: loadout diff <skill-id> --upstream")
	}
	m, err := newSourceManager(cmd)
	if err != nil {
		return err
	}
	text, err := m.UpstreamDiff(ctx, args[0])
	if err != nil {
		return err
	}
	if text == "" {
		fmt.Printf("%s matches upstream — no differences.\n", args[0])
		return nil
	}
	fmt.Print(text)
	return nil
}

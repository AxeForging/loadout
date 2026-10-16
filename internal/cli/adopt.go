package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/urfave/cli/v3"
)

func adoptCmd() *cli.Command {
	return &cli.Command{
		Name:      "adopt",
		Usage:     "Copy local edits of an installed artifact from one agent back into the store",
		ArgsUsage: "<artifact>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "agent",
				Aliases:  []string{"a"},
				Usage:    "Agent whose installed copy to adopt (e.g. claude-code)",
				Required: true,
			},
			projectFlag(),
			&cli.BoolFlag{Name: "dry-run", Usage: "Show what would change in the store and exit"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Adopt without the interactive confirmation prompt"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			id := cmd.Args().First()
			if id == "" || cmd.Args().Len() > 1 {
				return fmt.Errorf("usage: loadout adopt <artifact> --agent <agent>")
			}
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			agent := domain.AgentID(cmd.String("agent"))
			ap, err := eng.PlanAdopt(ctx, id, agent)
			if err != nil {
				return err
			}
			return runAdopt(ctx, cmd, eng, ap)
		},
	}
}

func runAdopt(ctx context.Context, cmd *cli.Command, eng *engine.Engine, ap *engine.AdoptPlan) error {
	for _, f := range ap.Ignored {
		fmt.Printf("  ~ ignored %s (only on disk — adopt never adds files; copy it into the store by hand)\n", f)
	}
	if len(ap.Changed) == 0 {
		if cmd.Bool("dry-run") {
			fmt.Printf("%s on %s already matches the store — nothing to adopt.\n", ap.Artifact.ID, ap.Agent)
			return nil
		}
		if _, err := eng.ApplyAdopt(ctx, ap, nil); err != nil {
			return err
		}
		fmt.Printf("%s on %s already matches the store — nothing to adopt.\n", ap.Artifact.ID, ap.Agent)
		return nil
	}

	fmt.Printf("Adopt %s from %s into the store: %d file(s)\n", ap.Artifact.ID, ap.Agent, len(ap.Changed))
	for _, key := range ap.Changed {
		text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(ap.Store.Files[key])),
			B:        difflib.SplitLines(string(ap.Adopted.Files[key])),
			FromFile: "store/" + key,
			ToFile:   string(ap.Agent) + "/" + key,
			Context:  3,
		})
		if err != nil {
			return err
		}
		fmt.Printf("── %s\n%s\n", key, text)
	}
	if cmd.Bool("dry-run") {
		fmt.Println("Dry run — nothing written.")
		return nil
	}

	n, err := eng.ApplyAdopt(ctx, ap, func(*engine.AdoptPlan) bool {
		if cmd.Bool("yes") {
			return true
		}
		fmt.Printf("\nWrite %d file(s) into the store? [y/N]: ", len(ap.Changed))
		answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	})
	if errors.Is(err, engine.ErrNotConfirmed) {
		fmt.Println("Aborted — nothing written.")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Adopted %d file(s) into the store (previous version snapshotted — 'loadout restore' lists it).\n", n)
	fmt.Println("Other agents now show drifted-store; run 'loadout sync' to roll the change out.")
	return nil
}

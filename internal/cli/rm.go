package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func rmCmd() *cli.Command {
	return &cli.Command{
		Name:      "rm",
		Usage:     "Remove artifacts: uninstall from every agent and delete from the store (always asks first)",
		ArgsUsage: "<artifact>...",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "dry-run", Usage: "Show the plan and exit without writing"},
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Apply without the interactive confirmation prompt"},
			&cli.BoolFlag{Name: "force", Usage: "Also remove targets whose content changed since loadout installed them (a snapshot is taken first)"},
			&cli.BoolFlag{Name: "keep-store", Usage: "Only uninstall from agent homes; keep the artifact in the store"},
			projectFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			ids := cmd.Args().Slice()
			if len(ids) == 0 {
				return fmt.Errorf("usage: loadout rm <artifact>... (see 'loadout status' for what's in the store)")
			}
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			return runRemove(ctx, cmd, eng, ids)
		},
	}
}

// runRemove is the rm orchestration: validate IDs, plan the uninstall across
// agent homes (plus the store files about to go), show everything, and apply
// under one confirmation. Store deletion runs after the engine pass — a crash
// in between leaves the artifact in the store and re-running rm converges.
func runRemove(ctx context.Context, cmd *cli.Command, eng *engine.Engine, ids []string) error {
	s := eng.Store
	installedIDs, err := eng.InstalledArtifactIDs()
	if err != nil {
		return err
	}
	installed := make(map[string]bool, len(installedIDs))
	for _, id := range installedIDs {
		installed[id] = true
	}
	for _, id := range ids {
		if _, found := s.Artifact(id); !found && !installed[id] {
			return fmt.Errorf("unknown artifact %q (see 'loadout status')", id)
		}
	}

	keepStore := cmd.Bool("keep-store")
	rp, err := eng.PlanRemoval(ctx, ids, engine.RemovalOptions{
		Force:       cmd.Bool("force"),
		RemoveStore: !keepStore,
	})
	if err != nil {
		return err
	}

	printRemovals(rp)
	for _, sk := range rp.Skips {
		fmt.Printf("  ~ %s skipped: %s\n", sk.Agent, sk.Reason)
	}
	for _, sf := range rp.StoreFiles {
		if sf.Op == engine.OpDelete {
			fmt.Printf("  delete store: %s\n", sf.RelPath)
		}
	}
	for _, id := range ids {
		if art, found := s.Artifact(id); found && strings.HasPrefix(art.Origin, "source:") {
			name := strings.TrimPrefix(art.Origin, "source:")
			fmt.Printf("  ! %s came from source %q — the next 'loadout update' will re-add it; 'loadout source remove %s' drops the whole source\n", id, name, name)
		}
	}

	if len(rp.Items) == 0 && len(rp.StoreFiles) == 0 && len(rp.ClearState) == 0 {
		fmt.Println("Nothing to remove.")
		return nil
	}
	if len(rp.Items) == 0 && len(rp.StoreFiles) == 0 {
		fmt.Println("  (nothing on disk — only stale install records to clear)")
	}
	if cmd.Bool("dry-run") {
		fmt.Println("\nDry run — nothing written.")
		return nil
	}

	removed, err := eng.ApplyRemoval(ctx, rp, confirmRemoval(cmd, rp))
	if errors.Is(err, engine.ErrNotConfirmed) {
		fmt.Println("Aborted — nothing written.")
		return nil
	}
	if err != nil {
		return err
	}

	storeRemoved := 0
	if !keepStore {
		for _, id := range ids {
			if _, found := s.Artifact(id); !found {
				continue
			}
			if err := s.RemoveArtifact(id); err != nil {
				return fmt.Errorf("store entry for %q: %w (agent homes already cleaned — re-run 'loadout rm %s' to retry)", id, err, id)
			}
			storeRemoved++
		}
	}

	fmt.Printf("Removed: %d agent change(s), %d store artifact(s).\n", removed, storeRemoved)
	if len(rp.Warnings) > 0 {
		fmt.Printf("%d warning(s) above — those targets were left in place.\n", len(rp.Warnings))
	}
	fmt.Println("Next: 'loadout status' to verify — 'loadout restore' can undo this.")
	return nil
}

// confirmRemoval prompts on stdin unless --yes was passed. CLI side of
// ApplyRemoval's confirm gate.
func confirmRemoval(cmd *cli.Command, rp *engine.RemovalPlan) func(*engine.RemovalPlan) bool {
	return func(*engine.RemovalPlan) bool {
		if cmd.Bool("yes") {
			return true
		}
		total := len(rp.Items) + len(rp.StoreFiles)
		fmt.Printf("\nRemove %d target(s)? [y/N]: ", total)
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

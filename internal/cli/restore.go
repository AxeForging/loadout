package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/config"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/urfave/cli/v3"
)

func restoreCmd() *cli.Command {
	return &cli.Command{
		Name:      "restore",
		Usage:     "List or restore pre-write snapshots taken before sync/switch overwrote files",
		ArgsUsage: "[snapshot-id]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "yes", Aliases: []string{"y"}, Usage: "Restore without the interactive confirmation prompt"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			backupDir, err := config.BackupsDir()
			if err != nil {
				return err
			}
			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			id := cmd.Args().First()
			if id == "" {
				return listSnapshots(backupDir)
			}
			return restoreSnapshot(ctx, cmd, eng, backupDir, id)
		},
	}
}

func listSnapshots(backupDir string) error {
	snaps, err := engine.ListSnapshots(backupDir)
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		fmt.Println("No snapshots yet — loadout takes one automatically before it overwrites any existing file.")
		return nil
	}
	fmt.Printf("%d snapshot(s), newest first:\n", len(snaps))
	for _, s := range snaps {
		fmt.Printf("  %s  %-9s  %-7s  %d file(s)\n", s.ID, relativeTime(s.CreatedAt), s.Trigger, len(s.Entries))
	}
	fmt.Println("\nRestore one with: loadout restore <snapshot-id>")
	return nil
}

func restoreSnapshot(ctx context.Context, cmd *cli.Command, eng *engine.Engine, backupDir, id string) error {
	snap, err := engine.LoadSnapshot(backupDir, id)
	if err != nil {
		return fmt.Errorf("no snapshot %q (run 'loadout restore' to list): %w", id, err)
	}
	fmt.Printf("Snapshot %s (%s, %s) would restore %d file(s):\n", snap.ID, snap.Trigger, relativeTime(snap.CreatedAt), len(snap.Entries))
	for _, e := range snap.Entries {
		fmt.Printf("  restore %s: %s\n", e.Agent, e.RelPath)
	}

	n, err := eng.ApplyRestore(ctx, snap, confirmRestore(cmd))
	if errors.Is(err, engine.ErrNotConfirmed) {
		fmt.Println("Aborted — nothing restored.")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Printf("Restored %d file(s).\n", n)
	fmt.Println("Note: 'loadout status' may now show drift — run 'loadout sync' to re-converge, or leave it as-is.")
	return nil
}

// confirmRestore is the CLI side of ApplyRestore's confirmation gate; a
// pre-restore snapshot is taken automatically, so a restore is itself undoable.
func confirmRestore(cmd *cli.Command) func(*engine.Snapshot) bool {
	return func(s *engine.Snapshot) bool {
		if cmd.Bool("yes") {
			return true
		}
		fmt.Printf("\nRestore %d file(s)? [y/N]: ", len(s.Entries))
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

// relativeTime renders an approximate "N units ago" for snapshot listings.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

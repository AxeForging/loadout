package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/AxeForging/loadout/internal/config"
	"github.com/urfave/cli/v3"
)

// Opt-in, non-intrusive source-update hint. Off by default; enabled with
// updates.checkOnRun in config.yaml. Failures are always silent, the network
// budget is 2 seconds, results are cached for updates.intervalHours (24 by
// default), and nothing is ever applied — at most one trailing hint line is
// printed. The cache itself lives in internal/config so the TUI dashboard
// can read the same signal.

func saveUpdateCache(updates int) { config.SaveUpdateCache(updates) }

// maybeUpdateHint is called at the end of read-heavy commands (status, sync).
func maybeUpdateHint(ctx context.Context, cmd *cli.Command) {
	cfg, err := config.Load()
	if err != nil || !cfg.Updates.CheckOnRun {
		return
	}
	interval := time.Duration(cfg.Updates.IntervalHours) * time.Hour
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	if cache, ok := config.LoadUpdateCache(); ok && time.Since(cache.CheckedAt) < interval {
		if cache.Updates > 0 {
			fmt.Printf("\nhint: %d source(s) have updates — run 'loadout update' (or 'loadout check' for details)\n", cache.Updates)
		}
		return
	}

	m, err := newSourceManager(cmd)
	if err != nil {
		return
	}
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	results, err := m.Check(checkCtx)
	if err != nil {
		return
	}
	updates := 0
	for _, r := range results {
		if r.HasUpdate {
			updates++
		}
	}
	saveUpdateCache(updates)
	if updates > 0 {
		fmt.Printf("\nhint: %d source(s) have updates — run 'loadout update' (or 'loadout check' for details)\n", updates)
	}
}

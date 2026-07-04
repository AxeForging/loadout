package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/AxeForging/loadout/internal/config"
	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v3"
)

// Opt-in, non-intrusive source-update hint. Off by default; enabled with
// updates.checkOnRun in config.yaml. Failures are always silent, the network
// budget is 2 seconds, results are cached for updates.intervalHours (24 by
// default), and nothing is ever applied — at most one trailing hint line is
// printed.

type updateCache struct {
	CheckedAt time.Time `yaml:"checkedAt"`
	Updates   int       `yaml:"updates"`
}

func updateCachePath() (string, error) {
	data, err := config.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(data, "update-cache.yaml"), nil
}

func loadUpdateCache() (updateCache, bool) {
	var c updateCache
	path, err := updateCachePath()
	if err != nil {
		return c, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, false
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return c, false
	}
	return c, true
}

func saveUpdateCache(updates int) {
	path, err := updateCachePath()
	if err != nil {
		return
	}
	raw, err := yaml.Marshal(updateCache{CheckedAt: time.Now(), Updates: updates})
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644)
}

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

	if cache, ok := loadUpdateCache(); ok && time.Since(cache.CheckedAt) < interval {
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

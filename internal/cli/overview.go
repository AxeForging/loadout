package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/urfave/cli/v3"
)

// Overview is the root action: running bare `loadout` greets with the state
// of the world and contextual next steps instead of a help wall.
func Overview(ctx context.Context, cmd *cli.Command) error {
	s, err := openStore(cmd)
	if err != nil {
		fmt.Println("loadout — your agentic gear kit.")
		fmt.Println()
		fmt.Println("No store yet on this machine. Get started:")
		fmt.Println("  loadout init                      create your canonical store")
		fmt.Println("  loadout wizard                    guided setup (conventions, testing, SDD, handoff)")
		fmt.Println("  loadout bundle import <path>      or start from a teammate's bundle")
		fmt.Println("  loadout --help                    everything else")
		return nil
	}

	eng, err := newEngine(cmd)
	if err != nil {
		return err
	}
	rows, plan, err := eng.Status(ctx, filterFromFlags(cmd, nil))
	if err != nil {
		return err
	}

	installed := 0
	total := len(eng.Registry.All())
	skipped := map[domain.AgentID]bool{}
	for _, sk := range plan.Skips {
		skipped[sk.Agent] = true
	}
	installed = total - len(skipped)

	worst := map[domain.SyncState]int{}
	for _, r := range rows {
		worst[r.State]++
	}

	fmt.Printf("loadout — store %q: %d artifact(s) · %d/%d agent(s) installed\n", s.Manifest.Name, len(s.Manifest.Artifacts), installed, total)

	if len(s.Manifest.Artifacts) == 0 {
		fmt.Println("\nThe store is empty. Fill it:")
		fmt.Println("  loadout wizard                guided setup from your conventions")
		fmt.Println("  loadout new skill <name>      scaffold by hand (--ai \"desc\" to draft with your agent)")
		fmt.Println("  loadout add <path>            adopt something you already have (e.g. ~/.claude/skills/...)")
		fmt.Println("  loadout source add <url>      import a skills repo")
		return nil
	}

	var stateBits []string
	for _, st := range []domain.SyncState{domain.StateInSync, domain.StateMissing, domain.StateDriftedLocal, domain.StateDriftedStore, domain.StateConflict} {
		if worst[st] > 0 {
			stateBits = append(stateBits, fmt.Sprintf("%d %s", worst[st], st))
		}
	}
	if len(stateBits) > 0 {
		fmt.Printf("Sync state: %s\n", strings.Join(stateBits, " · "))
	}

	changes := len(plan.Changes())
	fmt.Println()
	if changes > 0 {
		fmt.Printf("Next: %d pending change(s) — 'loadout diff' to inspect, 'loadout sync' to apply (asks first).\n", changes)
	} else {
		fmt.Println("Everything in sync. 'loadout status' for the full matrix, 'loadout tui' for the dashboard.")
	}
	maybeUpdateHint(ctx, cmd)
	return nil
}

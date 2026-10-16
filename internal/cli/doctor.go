package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/urfave/cli/v3"
)

// instructionFiles are the shared instruction documents doctor inspects for
// managed-block health inside each installed agent home.
var instructionFiles = []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"}

func doctorCmd() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "Verify the store, agent homes, managed blocks and install state on this machine",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			problems := 0
			fail := func(format string, args ...any) {
				problems++
				fmt.Printf("  ✗ "+format+"\n", args...)
			}
			ok := func(format string, args ...any) { fmt.Printf("  ✓ "+format+"\n", args...) }
			note := func(format string, args ...any) { fmt.Printf("  ~ "+format+"\n", args...) }

			eng, err := newEngine(cmd)
			if err != nil {
				return err
			}
			s := eng.Store

			fmt.Println("Store:")
			ok("manifest ok — %d artifact(s) at %s", len(s.Manifest.Artifacts), s.Root)
			if h, found := s.GitHealth(ctx); found {
				switch w := h.Warning(s.Root); {
				case !h.Repo:
					note("store is not a git repository — edits have no history (snapshots still cover overwrites)")
				case w != "":
					note("%s", w)
				default:
					ok("store committed — no uncommitted changes")
				}
			}
			for _, art := range s.Manifest.Artifacts {
				if err := checkArtifact(s, art); err != nil {
					fail("artifact %s: %v", art.ID, err)
				}
				if w := domain.ReservedNameWarning(art.Kind, art.ID); w != "" {
					note("artifact %q: %s", art.ID, w)
				}
			}

			fmt.Println("Agents:")
			homes := map[string]bool{}
			for _, ad := range eng.Registry.All() {
				det, err := ad.Detect(ctx)
				if err != nil {
					fail("%s: detect failed: %v", ad.ID(), err)
					continue
				}
				if !det.Installed {
					note("%s not installed", ad.ID())
					continue
				}
				ok("%s installed at %s", ad.ID(), det.Home)
				homes[det.Home] = true
			}

			fmt.Println("Managed blocks:")
			blocksChecked := 0
			for home := range homes {
				for _, name := range instructionFiles {
					path := filepath.Join(home, name)
					raw, err := os.ReadFile(path)
					if os.IsNotExist(err) {
						continue
					}
					if err != nil {
						fail("%s: %v", path, err)
						continue
					}
					blocksChecked++
					if issues := adapter.CheckBlocks(raw); len(issues) > 0 {
						fail("%s: %s", path, strings.Join(issues, "; "))
					} else {
						ok("%s markers balanced", path)
					}
				}
			}
			if blocksChecked == 0 {
				note("no instruction files found yet")
			}

			fmt.Println("Install state:")
			orphans, err := eng.OrphanedState()
			if err != nil {
				return err
			}
			for _, o := range orphans {
				note("orphaned record: artifact %q on %s (removed from store; files may remain: %s)",
					o.Artifact, o.Agent, strings.Join(o.Files, ", "))
			}
			// Recorded installs whose files vanished from disk (deleted by
			// hand or by the agent) — sync would recreate them.
			plan, err := eng.Plan(ctx, engine.Filter{})
			if err != nil {
				return err
			}
			vanished := 0
			for _, it := range plan.Items {
				if it.StateHash != "" && !it.Exists {
					vanished++
					note("recorded install missing on disk: %s of %q on %s (run 'loadout sync' to restore)",
						it.RelPath, it.Artifact.ID, it.Agent)
				}
			}
			if len(orphans) == 0 && vanished == 0 {
				ok("no orphaned records, all recorded installs present")
			}

			if problems > 0 {
				return fmt.Errorf("doctor found %d problem(s)", problems)
			}
			fmt.Println("\nAll good.")
			return nil
		},
	}
}

// checkArtifact validates one manifest entry: content readable, name valid,
// and kind-specific structure (hook spec + script, settings fragment files).
func checkArtifact(s storeReader, art domain.Artifact) error {
	if err := store.ValidateName(art.ID); err != nil {
		return err
	}
	content, err := s.Content(art)
	if err != nil {
		return err
	}
	switch art.Kind {
	case domain.KindHook:
		raw, exists := content.Files["hook.yaml"]
		if !exists {
			return fmt.Errorf("missing hook.yaml")
		}
		spec, err := domain.ParseHookSpec(raw)
		if err != nil {
			return err
		}
		if _, exists := content.Files[spec.Script]; !exists {
			return fmt.Errorf("missing script %s", spec.Script)
		}
	case domain.KindMCP:
		raw, exists := content.Files["mcp.yaml"]
		if !exists {
			return fmt.Errorf("missing mcp.yaml")
		}
		// ParseMCPSpec re-runs full validation, including the literal-secret
		// gate — doctor catches hand-edits that bypassed `loadout new`.
		if _, err := domain.ParseMCPSpec(raw); err != nil {
			return err
		}
	case domain.KindSettings:
		if len(content.Files) == 0 {
			return fmt.Errorf("empty settings fragment")
		}
	case domain.KindInstruction, domain.KindCommand:
		if len(content.Files) != 1 {
			return fmt.Errorf("expected exactly one file, got %d", len(content.Files))
		}
	}
	return nil
}

// storeReader is the slice of store.Store doctor needs — kept narrow so
// checkArtifact is trivially testable.
type storeReader interface {
	Content(a domain.Artifact) (domain.Content, error)
}

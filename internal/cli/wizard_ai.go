package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/runner"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/urfave/cli/v3"
)

// aiEnrichTargets are the wizard-generated docs worth deepening per stack.
var aiEnrichTargets = []string{"testing", "conventions", "toolchain"}

// enrichWithAI rewrites selected generated instructions with a logged-in
// headless agent, previews the outcome, and overwrites only after
// confirmation. Failures degrade gracefully — templates stay in place.
func enrichWithAI(ctx context.Context, cmd *cli.Command, s *store.Store, answers wizard.Answers, g wizard.Generated) error {
	var r runner.Runner
	var err error
	if id := cmd.String("runner"); id != "" {
		r, err = runner.Get(id)
		if err == nil && !r.Available() {
			err = fmt.Errorf("runner %q is not on PATH", id)
		}
	} else {
		r, err = runner.First()
	}
	if err != nil {
		return err
	}

	created := map[string]string{} // id → store-relative path
	for _, art := range g.Created {
		created[art.ID] = art.Path
	}

	teamContext := answers.Summary()
	type enriched struct {
		id, path, content string
		before, after     int
	}
	var results []enriched
	for _, id := range aiEnrichTargets {
		relPath, ok := created[id]
		if !ok {
			continue
		}
		abs := filepath.Join(s.Root, relPath)
		current, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		fmt.Printf("Enriching %s with %s…\n", id, r.ID())
		out, err := runner.EnrichMarkdown(ctx, r, id+".md", string(current), teamContext)
		if err != nil {
			fmt.Printf("  ~ %s: %v — keeping the template\n", id, err)
			continue
		}
		results = append(results, enriched{
			id: id, path: abs, content: out,
			before: len(strings.Split(string(current), "\n")),
			after:  len(strings.Split(out, "\n")),
		})
	}
	if len(results) == 0 {
		return fmt.Errorf("no documents were enriched")
	}

	fmt.Printf("\nAI enrichment ready for %d doc(s):\n", len(results))
	for _, e := range results {
		fmt.Printf("  ~ %s (%d → %d lines)\n", e.id, e.before, e.after)
	}
	if !cmd.Bool("yes") && !promptYes("Replace the templates with the enriched versions?") {
		fmt.Println("Enrichment discarded — templates kept.")
		return nil
	}
	for _, e := range results {
		if err := os.WriteFile(e.path, []byte(e.content), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("Enriched %d doc(s).\n", len(results))
	return nil
}

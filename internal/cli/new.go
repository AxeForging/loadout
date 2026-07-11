package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/runner"
	"github.com/urfave/cli/v3"
)

func newCmd() *cli.Command {
	return &cli.Command{
		Name:      "new",
		Usage:     "Scaffold a new artifact in the store (or draft one with your logged-in agent via --ai)",
		ArgsUsage: "<skill|instruction|command|hook|mcp> <name>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "ai",
				Usage: "Draft the artifact with a logged-in headless agent from this description (skills only; no API key — uses your existing plan)",
			},
			&cli.StringFlag{
				Name:  "runner",
				Usage: "Which headless agent to use: claude, codex or gemini (default: first available)",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Write the AI draft without the preview confirmation",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			kind := domain.ArtifactKind(cmd.Args().Get(0))
			name := cmd.Args().Get(1)
			if kind == "" || name == "" {
				return fmt.Errorf("usage: loadout new <skill|instruction|command|hook|mcp> <name>")
			}
			s, err := openStore(cmd)
			if err != nil {
				return err
			}

			if description := cmd.String("ai"); description != "" {
				if kind != domain.KindSkill {
					return fmt.Errorf("--ai currently drafts skills only")
				}
				return newAISkill(ctx, cmd, s, name, description)
			}

			a, err := s.New(kind, name)
			if err != nil {
				return err
			}
			fmt.Printf("Created %s %q at %s\n", a.Kind, a.ID, filepath.Join(s.Root, a.Path))
			if w := domain.ReservedNameWarning(a.Kind, a.ID); w != "" {
				fmt.Printf("  ! %s\n", w)
			}
			fmt.Println("Edit it, then run 'loadout sync' to install it.")
			return nil
		},
	}
}

// newAISkill drafts a SKILL.md with a logged-in headless agent, previews it,
// and writes only after confirmation.
func newAISkill(ctx context.Context, cmd *cli.Command, s storeWithSkills, name, description string) error {
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

	fmt.Printf("Drafting skill %q with %s (uses your logged-in session)…\n", name, r.ID())
	content, err := runner.DraftSkill(ctx, r, name, description)
	if err != nil {
		return err
	}

	fmt.Println("\n──── draft preview ────")
	lines := strings.Split(content, "\n")
	preview := lines
	if len(preview) > 25 {
		preview = preview[:25]
	}
	fmt.Println(strings.Join(preview, "\n"))
	if len(lines) > 25 {
		fmt.Printf("… (%d more lines)\n", len(lines)-25)
	}
	fmt.Println("───────────────────────")

	if !cmd.Bool("yes") && !promptYes("Write this skill to the store?") {
		fmt.Println("Aborted — nothing written.")
		return nil
	}
	a, err := s.CreateSkill(name, []byte(content))
	if err != nil {
		return err
	}
	fmt.Printf("Created skill %q — review it, then 'loadout sync' to install.\n", a.ID)
	if w := domain.ReservedNameWarning(a.Kind, a.ID); w != "" {
		fmt.Printf("  ! %s\n", w)
	}
	return nil
}

// storeWithSkills is the slice of store.Store newAISkill needs.
type storeWithSkills interface {
	CreateSkill(name string, skillMD []byte) (domain.Artifact, error)
}

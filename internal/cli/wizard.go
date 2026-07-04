package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/AxeForging/loadout/internal/wizard"
	"github.com/urfave/cli/v3"
)

func wizardCmd() *cli.Command {
	return &cli.Command{
		Name:    "wizard",
		Aliases: []string{"create"},
		Usage:   "Guided builder: turn your stack and conventions into skills, instructions and commands",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "preset",
				Usage: "Start from a preset (" + strings.Join(wizard.Presets(), ", ") + ") instead of the interactive questionnaire",
			},
			&cli.StringFlag{
				Name:  "answers",
				Usage: "Answers YAML file (question ID → value) for non-interactive runs",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Write to the store without the final confirmation prompt",
			},
			&cli.BoolFlag{
				Name:  "ai",
				Usage: "After generating, deepen testing/conventions/toolchain docs for your exact stack using a logged-in headless agent (no API key)",
			},
			&cli.StringFlag{
				Name:  "runner",
				Usage: "Which headless agent to use for --ai: claude, codex or gemini (default: first available)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			s, err := openStore(cmd)
			if err != nil {
				return err
			}

			answers := wizard.Answers{}
			nonInteractive := false
			if preset := cmd.String("preset"); preset != "" {
				loaded, err := wizard.LoadPreset(preset)
				if err != nil {
					return err
				}
				answers = answers.Merge(loaded)
				nonInteractive = true
			}
			if file := cmd.String("answers"); file != "" {
				loaded, err := wizard.LoadAnswers(file)
				if err != nil {
					return err
				}
				answers = answers.Merge(loaded)
				nonInteractive = true
			}
			if !nonInteractive {
				collected, err := collectAnswers()
				if err != nil {
					return err
				}
				answers = answers.Merge(collected)
			}

			create, skip := wizard.Plan(s, answers)
			if len(create) == 0 {
				fmt.Println("Nothing to generate — every artifact these answers produce already exists.")
				return nil
			}
			fmt.Printf("The wizard will create %d artifact(s) in the store:\n", len(create))
			for _, id := range create {
				fmt.Printf("  + %s\n", id)
			}
			for _, id := range skip {
				fmt.Printf("  ~ %s exists — will be left untouched\n", id)
			}

			if !cmd.Bool("yes") && !promptYes("Write these to the store?") {
				fmt.Println("Aborted — nothing written.")
				return nil
			}
			g, err := wizard.Generate(s, answers)
			if err != nil {
				return err
			}
			fmt.Printf("Created %d artifact(s).\n", len(g.Created))

			if cmd.Bool("ai") {
				if err := enrichWithAI(ctx, cmd, s, answers, g); err != nil {
					fmt.Printf("AI enrichment skipped: %v (the generated templates are fully usable as-is)\n", err)
				}
			}
			fmt.Println("Review them in the store, then run 'loadout sync' to install into your agents.")
			return nil
		},
	}
}

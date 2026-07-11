package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/urfave/cli/v3"
)

func addCmd() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Adopt an existing native artifact (skill dir, hook dir, command/instruction file) into the store",
		ArgsUsage: "<path>",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "kind",
				Usage: "Artifact kind (skill, instruction, command, hook, mcp); inferred when possible",
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "Artifact name in the store (default: derived from the path)",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			src := cmd.Args().First()
			if src == "" {
				return fmt.Errorf("usage: loadout add <path>")
			}
			src, err := filepath.Abs(src)
			if err != nil {
				return err
			}
			info, err := os.Stat(src)
			if err != nil {
				return err
			}

			kind, err := inferKind(cmd.String("kind"), src, info.IsDir())
			if err != nil {
				return err
			}
			name := cmd.String("name")
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(src), ".md")
			}

			s, err := openStore(cmd)
			if err != nil {
				return err
			}
			a, err := s.Adopt(kind, name, src)
			if err != nil {
				return err
			}
			fmt.Printf("Adopted %s %q into %s\n", a.Kind, a.ID, filepath.Join(s.Root, a.Path))
			fmt.Println("Run 'loadout status' to see where it will sync.")
			return nil
		},
	}
}

// inferKind resolves the artifact kind from the flag or the path's shape:
// directories with SKILL.md are skills, with hook.yaml hooks, with mcp.yaml
// mcps; files default to commands (pass --kind instruction to override).
func inferKind(flag, src string, isDir bool) (domain.ArtifactKind, error) {
	if flag != "" {
		kind := domain.ArtifactKind(flag)
		switch kind {
		case domain.KindSkill, domain.KindInstruction, domain.KindCommand, domain.KindHook, domain.KindMCP:
			return kind, nil
		}
		return "", fmt.Errorf("unknown kind %q", flag)
	}
	if isDir {
		if _, err := os.Stat(filepath.Join(src, "SKILL.md")); err == nil {
			return domain.KindSkill, nil
		}
		if _, err := os.Stat(filepath.Join(src, "hook.yaml")); err == nil {
			return domain.KindHook, nil
		}
		if _, err := os.Stat(filepath.Join(src, "mcp.yaml")); err == nil {
			return domain.KindMCP, nil
		}
		return "", fmt.Errorf("cannot infer kind of directory %s (no SKILL.md, hook.yaml or mcp.yaml) — pass --kind", src)
	}
	return domain.KindCommand, nil
}

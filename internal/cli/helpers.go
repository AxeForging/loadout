package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/antigravity"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/adapter/codex"
	"github.com/AxeForging/loadout/internal/adapter/cursor"
	"github.com/AxeForging/loadout/internal/adapter/geminicli"
	"github.com/AxeForging/loadout/internal/config"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/urfave/cli/v3"
)

// newRegistry registers every adapter. Adding an agent means adding one
// Register call here.
func newRegistry() *adapter.Registry {
	r := adapter.NewRegistry()
	r.Register(claudecode.New())
	r.Register(codex.New())
	r.Register(geminicli.New())
	r.Register(antigravity.New())
	r.Register(cursor.New())
	return r
}

// resolveStorePath applies precedence: --store flag > config file > default.
func resolveStorePath(cmd *cli.Command) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	return cfg.ResolveStore(cmd.String("store"))
}

func openStore(cmd *cli.Command) (*store.Store, error) {
	path, err := resolveStorePath(cmd)
	if err != nil {
		return nil, err
	}
	return store.Open(path)
}

func newEngine(cmd *cli.Command) (*engine.Engine, error) {
	s, err := openStore(cmd)
	if err != nil {
		return nil, err
	}
	statePath, err := config.StatePath()
	if err != nil {
		return nil, err
	}
	backupDir, err := config.BackupsDir()
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	projectRoot, err := resolveProjectRoot(cmd)
	if err != nil {
		return nil, err
	}
	return &engine.Engine{
		Store:       s,
		Registry:    newRegistry(),
		StatePath:   statePath,
		BackupDir:   backupDir,
		KeepBackups: cfg.BackupsKeep(),
		ProjectRoot: projectRoot,
	}, nil
}

// resolveProjectRoot decides what project-scoped artifacts resolve against:
// an explicit --project wins; otherwise the current directory when it looks
// like a project (has .git). No project means such targets report n/a.
func resolveProjectRoot(cmd *cli.Command) (string, error) {
	if dir := cmd.String("project"); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(abs); err != nil || !info.IsDir() {
			return "", fmt.Errorf("--project %s: not a directory", dir)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil //nolint:nilerr // no cwd just means no project context
	}
	if info, err := os.Stat(filepath.Join(cwd, ".git")); err == nil && info.IsDir() {
		return cwd, nil
	}
	return "", nil
}

func projectFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "project",
		Usage: "Project directory for project-scoped artifacts (default: current directory when it is a git repo)",
	}
}

// filterFromFlags builds an engine filter from --agent flags and positional
// artifact arguments.
func filterFromFlags(cmd *cli.Command, artifacts []string) engine.Filter {
	f := engine.Filter{Artifacts: artifacts}
	for _, a := range cmd.StringSlice("agent") {
		f.Agents = append(f.Agents, domain.AgentID(a))
	}
	return f
}

func agentFlag() cli.Flag {
	return &cli.StringSliceFlag{
		Name:    "agent",
		Aliases: []string{"a"},
		Usage:   "Limit to one or more agents (e.g. claude-code); repeatable",
	}
}

// printSkips reports agents the plan left out.
func printSkips(p *engine.Plan) {
	for _, s := range p.Skips {
		fmt.Printf("  ~ %s skipped: %s\n", s.Agent, s.Reason)
	}
}

// printChanges lists what Apply would write, plus any conflicting files it
// would refuse to overwrite (or, with --force, overwrite after snapshotting).
func printChanges(p *engine.Plan) {
	for _, it := range p.Changes() {
		if it.Conflict { // only present when OverwriteConflicts is set
			fmt.Printf("  ! force  %s: %s  (%s) — overwrites a file loadout never installed (snapshotted first)\n", it.Agent, it.RelPath, it.Artifact.ID)
			continue
		}
		fmt.Printf("  %-6s %s: %s  (%s)\n", it.Op, it.Agent, it.RelPath, it.Artifact.ID)
	}
	if !p.OverwriteConflicts {
		for _, it := range p.Conflicts() {
			fmt.Printf("  ! skip   %s: %s  (%s) — exists on disk, never installed by loadout\n", it.Agent, it.RelPath, it.Artifact.ID)
		}
	}
}

// forceFlag opts into overwriting files loadout has never installed.
func forceFlag() cli.Flag {
	return &cli.BoolFlag{
		Name:  "force",
		Usage: "Overwrite conflicting files loadout never installed (a snapshot is taken first)",
	}
}

// confirmApply prompts on stdin unless --yes was passed. It is the CLI side
// of the engine's confirmation gate.
func confirmApply(cmd *cli.Command) func(*engine.Plan) bool {
	return func(p *engine.Plan) bool {
		if cmd.Bool("yes") {
			return true
		}
		fmt.Printf("\nApply %d change(s)? [y/N]: ", len(p.Changes()))
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

// printRemovals lists what a loadout switch would remove, plus any
// artifact×file pairs the engine could not safely remove automatically.
func printRemovals(rp *engine.RemovalPlan) {
	for _, it := range rp.Items {
		fmt.Printf("  %-6s %s: %s  (%s)\n", it.Op, it.Agent, it.RelPath, it.Artifact.ID)
	}
	for _, w := range rp.Warnings {
		fmt.Printf("  ! %s %s: %s — %s\n", w.Agent, w.Artifact, w.RelPath, w.Reason)
	}
}

// confirmSwitch prompts on stdin unless --yes was passed, summing the
// install and removal change counts. CLI side of ApplySwitch's confirm gate.
func confirmSwitch(cmd *cli.Command, install *engine.Plan, removal *engine.RemovalPlan) func(*engine.Plan, *engine.RemovalPlan) bool {
	return func(*engine.Plan, *engine.RemovalPlan) bool {
		if cmd.Bool("yes") {
			return true
		}
		total := len(install.Changes()) + len(removal.Items)
		fmt.Printf("\nApply %d change(s)? [y/N]: ", total)
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return false
		}
		answer = strings.ToLower(strings.TrimSpace(answer))
		return answer == "y" || answer == "yes"
	}
}

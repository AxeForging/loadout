package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
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
	return &engine.Engine{Store: s, Registry: newRegistry(), StatePath: statePath}, nil
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

// printChanges lists what Apply would write.
func printChanges(p *engine.Plan) {
	for _, it := range p.Changes() {
		fmt.Printf("  %-6s %s: %s  (%s)\n", it.Op, it.Agent, it.RelPath, it.Artifact.ID)
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

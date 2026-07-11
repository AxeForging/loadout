package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/AxeForging/loadout/internal/sdd"
	"github.com/urfave/cli/v3"
)

func specCmd() *cli.Command {
	dirFlag := &cli.StringFlag{
		Name:  "dir",
		Usage: "Specs directory relative to the project root",
		Value: sdd.DefaultDir,
	}
	return &cli.Command{
		Name:      "spec",
		Usage:     "Spec-driven development: numbered specs with checklists in files or GitHub issues",
		ArgsUsage: "<subcommand>",
		Commands: []*cli.Command{
			{
				Name:      "init",
				Usage:     "Scaffold the next free NNN-<name>.md spec (and its tracking issue on the github backend)",
				ArgsUsage: "<short-name>",
				Flags: []cli.Flag{
					dirFlag,
					&cli.StringFlag{Name: "backend", Usage: "Persist the checklist backend for this project: local or github"},
					&cli.StringSliceFlag{Name: "label", Usage: "Label for created issues (github backend); repeatable"},
				},
				Action: specInitAction,
			},
			{
				Name:   "list",
				Usage:  "List specs with checklist progress",
				Flags:  []cli.Flag{dirFlag},
				Action: specListAction,
			},
			{
				Name:      "status",
				Usage:     "Show checklist progress (all specs, or one)",
				ArgsUsage: "[spec]",
				Flags: []cli.Flag{
					dirFlag,
					&cli.BoolFlag{Name: "json", Usage: "Machine-readable output"},
					&cli.BoolFlag{Name: "require-complete", Usage: "Exit non-zero when any step is open (CI gate)"},
				},
				Action: specStatusAction,
			},
			{
				Name:      "check",
				Usage:     "Mark a step done (idempotent)",
				ArgsUsage: "<spec> <step-id>",
				Flags:     []cli.Flag{dirFlag},
				Action:    func(ctx context.Context, cmd *cli.Command) error { return specSetAction(ctx, cmd, true) },
			},
			{
				Name:      "uncheck",
				Usage:     "Mark a step open again (idempotent)",
				ArgsUsage: "<spec> <step-id>",
				Flags:     []cli.Flag{dirFlag},
				Action:    func(ctx context.Context, cmd *cli.Command) error { return specSetAction(ctx, cmd, false) },
			},
			{
				Name:      "sync",
				Usage:     "Reconcile a spec's checklist with its GitHub issue (github backend)",
				ArgsUsage: "<spec>",
				Flags: []cli.Flag{
					dirFlag,
					&cli.BoolFlag{Name: "from-local", Usage: "Resolve check-state divergence from the local file"},
					&cli.BoolFlag{Name: "from-github", Usage: "Resolve check-state divergence from the issue"},
				},
				Action: specSyncAction,
			},
		},
	}
}

// specEnv is the resolved project context every spec subcommand needs.
type specEnv struct {
	root    string // project root (git walk-up)
	dir     string // absolute specs dir
	cfg     sdd.Config
	backend sdd.Backend
}

func specSetup(cmd *cli.Command) (*specEnv, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	root, err := sdd.FindProjectRoot(cwd)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, cmd.String("dir"))
	cfg, err := sdd.LoadConfig(dir)
	if err != nil {
		return nil, err
	}
	env := &specEnv{root: root, dir: dir, cfg: cfg}
	if cfg.Backend == sdd.BackendGitHub {
		env.backend = &sdd.GitHubBackend{Dir: dir, ProjectRoot: root, Config: &env.cfg}
	} else {
		env.backend = &sdd.LocalBackend{Dir: dir}
	}
	return env, nil
}

func (e *specEnv) specs() ([]sdd.SpecInfo, error) { return sdd.ListSpecs(e.dir, e.cfg) }

func (e *specEnv) resolve(ref string) (sdd.SpecInfo, error) {
	if ref == "" {
		return sdd.SpecInfo{}, fmt.Errorf("which spec? pass a number (007) or name fragment (sdd-workflow)")
	}
	specs, err := e.specs()
	if err != nil {
		return sdd.SpecInfo{}, err
	}
	return sdd.Resolve(specs, ref)
}

func specInitAction(ctx context.Context, cmd *cli.Command) error {
	name := cmd.Args().First()
	if name == "" {
		return fmt.Errorf("usage: loadout spec init <short-name>")
	}
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	if b := cmd.String("backend"); b != "" {
		env.cfg.Backend = b
		if labels := cmd.StringSlice("label"); len(labels) > 0 {
			env.cfg.Labels = labels
		}
		if b != sdd.BackendLocal && b != sdd.BackendGitHub {
			return fmt.Errorf("unknown backend %q (local or github)", b)
		}
		if err := os.MkdirAll(env.dir, 0o755); err != nil {
			return err
		}
		if err := env.cfg.Save(env.dir); err != nil {
			return err
		}
	}

	spec, err := sdd.CreateSpec(env.dir, name)
	if err != nil {
		return err
	}
	fmt.Printf("Created %s\n", spec.Path)

	if env.cfg.Backend == sdd.BackendGitHub {
		body, err := os.ReadFile(spec.Path)
		if err != nil {
			return err
		}
		gb := &sdd.GitHubBackend{Dir: env.dir, ProjectRoot: env.root, Config: &env.cfg}
		issue, err := gb.CreateIssue(ctx, spec, body)
		if err != nil {
			return fmt.Errorf("spec file created, but issue creation failed: %w", err)
		}
		fmt.Printf("Opened tracking issue #%d (%s)\n", issue, env.cfg.Repo)
	}
	fmt.Println("Fill in the sections, then track steps with 'loadout spec check'.")
	return nil
}

func specListAction(ctx context.Context, cmd *cli.Command) error {
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	specs, err := env.specs()
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		fmt.Printf("No specs in %s yet — start one with 'loadout spec init <short-name>'.\n", env.dir)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SPEC\tSTEPS\tBACKEND")
	for _, s := range specs {
		done, total, err := stepCounts(ctx, env, s)
		progress := fmt.Sprintf("%d/%d", done, total)
		if err != nil {
			progress = "?"
		}
		where := env.cfg.Backend
		if s.Issue > 0 {
			where = fmt.Sprintf("github #%d", s.Issue)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", s.Name, progress, where)
	}
	return w.Flush()
}

func stepCounts(ctx context.Context, env *specEnv, s sdd.SpecInfo) (done, total int, err error) {
	steps, err := env.backend.Steps(ctx, s)
	if err != nil {
		return 0, 0, err
	}
	for _, st := range steps {
		if st.Done {
			done++
		}
	}
	return done, len(steps), nil
}

func specStatusAction(ctx context.Context, cmd *cli.Command) error {
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	var specs []sdd.SpecInfo
	if ref := cmd.Args().First(); ref != "" {
		s, err := env.resolve(ref)
		if err != nil {
			return err
		}
		specs = []sdd.SpecInfo{s}
	} else if specs, err = env.specs(); err != nil {
		return err
	}

	type row struct {
		Spec  string     `json:"spec"`
		Done  int        `json:"done"`
		Total int        `json:"total"`
		Steps []sdd.Step `json:"steps"`
	}
	var rows []row
	incomplete := 0
	for _, s := range specs {
		steps, err := env.backend.Steps(ctx, s)
		if err != nil {
			return fmt.Errorf("%s: %w", s.Name, err)
		}
		r := row{Spec: s.Name, Total: len(steps), Steps: steps}
		for _, st := range steps {
			if st.Done {
				r.Done++
			}
		}
		incomplete += r.Total - r.Done
		rows = append(rows, r)
	}

	if cmd.Bool("json") {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			return err
		}
	} else {
		for _, r := range rows {
			fmt.Printf("%s — %d/%d done\n", r.Spec, r.Done, r.Total)
			for _, st := range r.Steps {
				box := "[ ]"
				if st.Done {
					box = "[x]"
				}
				fmt.Printf("  %s %s  (%s)\n", box, st.Text, st.ID)
			}
		}
	}
	if cmd.Bool("require-complete") && incomplete > 0 {
		return fmt.Errorf("%d step(s) still open", incomplete)
	}
	return nil
}

func specSetAction(ctx context.Context, cmd *cli.Command, done bool) error {
	ref, stepID := cmd.Args().Get(0), cmd.Args().Get(1)
	if ref == "" || stepID == "" {
		return fmt.Errorf("usage: loadout spec %s <spec> <step-id>", map[bool]string{true: "check", false: "uncheck"}[done])
	}
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	spec, err := env.resolve(ref)
	if err != nil {
		return err
	}
	changed, err := env.backend.SetStep(ctx, spec, stepID, done)
	if err != nil {
		return err
	}
	state := "open"
	if done {
		state = "done"
	}
	if changed {
		fmt.Printf("%s: %s → %s\n", spec.Name, stepID, state)
	} else {
		fmt.Printf("%s: %s already %s\n", spec.Name, stepID, state)
	}
	return nil
}

func specSyncAction(ctx context.Context, cmd *cli.Command) error {
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	if env.cfg.Backend != sdd.BackendGitHub {
		return fmt.Errorf("spec sync only applies to the github backend (this project uses %q)", env.cfg.Backend)
	}
	spec, err := env.resolve(cmd.Args().First())
	if err != nil {
		return err
	}
	fromLocal, fromGitHub := cmd.Bool("from-local"), cmd.Bool("from-github")
	if fromLocal && fromGitHub {
		return fmt.Errorf("pick one direction: --from-local or --from-github")
	}

	body, err := os.ReadFile(spec.Path)
	if err != nil {
		return err
	}
	localSteps, err := sdd.ParseSteps(body)
	if err != nil {
		return err
	}
	gb := &sdd.GitHubBackend{Dir: env.dir, ProjectRoot: env.root, Config: &env.cfg}

	if spec.Issue == 0 {
		issue, err := gb.CreateIssue(ctx, spec, body)
		if err != nil {
			return err
		}
		fmt.Printf("Linked %s to new tracking issue #%d\n", spec.Name, issue)
		return nil
	}

	remoteSteps, err := gb.Steps(ctx, spec)
	if err != nil {
		return err
	}
	rep := sdd.Compare(localSteps, remoteSteps)
	if rep.InSync() {
		fmt.Printf("%s is in sync with issue #%d\n", spec.Name, spec.Issue)
		return nil
	}

	switch {
	case fromLocal:
		if _, err := gb.SetBody(ctx, spec, body); err != nil {
			return err
		}
		fmt.Printf("Pushed local spec (structure + state) to issue #%d\n", spec.Issue)
	case fromGitHub:
		lb := &sdd.LocalBackend{Dir: env.dir}
		for _, d := range rep.StateDiffs {
			if _, err := lb.SetStep(ctx, spec, d.ID, d.RemoteDone); err != nil {
				return err
			}
		}
		fmt.Printf("Pulled %d step state(s) from issue #%d into %s\n", len(rep.StateDiffs), spec.Issue, spec.Path)
	case len(rep.StateDiffs) > 0:
		fmt.Println("Check-state divergence — loadout will not guess a direction:")
		for _, d := range rep.StateDiffs {
			fmt.Printf("  %s\n", d.Describe())
		}
		return fmt.Errorf("re-run with --from-local or --from-github")
	default: // structure changed, states agree: structure always flows local → github
		if _, err := gb.SetBody(ctx, spec, body); err != nil {
			return err
		}
		fmt.Printf("Pushed checklist structure to issue #%d\n", spec.Issue)
	}
	return nil
}

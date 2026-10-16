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
	return &cli.Command{
		Name:      "spec",
		Usage:     "Spec-driven development: numbered specs with checklists in files or GitHub issues (--backend issues: spec + plan entirely in issues)",
		ArgsUsage: "<subcommand>",
		Commands: []*cli.Command{
			specSubCmd("init", "Scaffold the next free NNN-<name>.md spec (and its tracking issue on the github backend), or open spec(NNN): <name> as an issue (issues backend)", "<short-name>", specInitAction,
				&cli.StringSliceFlag{Name: "label", Usage: "Label for created issues (github/issues backends); repeatable"},
				&cli.StringFlag{Name: "style", Usage: "Issue body template for the issues backend: full or light", Value: "full"}),
			specSubCmd("list", "List specs with checklist progress", "", specListAction),
			specSubCmd("status", "Show checklist progress (all specs, or one)", "[spec]", specStatusAction,
				&cli.BoolFlag{Name: "json", Usage: "Machine-readable output"},
				&cli.BoolFlag{Name: "require-complete", Usage: "Exit non-zero when any step is open (CI gate)"}),
			specSubCmd("check", "Mark a step done (idempotent)", "<spec> <step-id>",
				func(ctx context.Context, cmd *cli.Command) error { return specSetAction(ctx, cmd, true) }),
			specSubCmd("uncheck", "Mark a step open again (idempotent)", "<spec> <step-id>",
				func(ctx context.Context, cmd *cli.Command) error { return specSetAction(ctx, cmd, false) }),
			specSubCmd("sync", "Reconcile a spec's checklist with its GitHub issue (github backend only)", "<spec>", specSyncAction,
				&cli.BoolFlag{Name: "from-local", Usage: "Resolve check-state divergence from the local file"},
				&cli.BoolFlag{Name: "from-github", Usage: "Resolve check-state divergence from the issue"}),
		},
	}
}

// specSubCmd builds one spec subcommand: every one takes --dir and
// --backend plus its own extra flags.
func specSubCmd(name, usage, argsUsage string, action cli.ActionFunc, extra ...cli.Flag) *cli.Command {
	flags := append([]cli.Flag{
		&cli.StringFlag{
			Name:  "dir",
			Usage: "Specs directory relative to the project root",
			Value: sdd.DefaultDir,
		},
		&cli.StringFlag{
			Name:    "backend",
			Usage:   "local, github or issues (overrides sdd.yaml; on init, local/github are persisted to sdd.yaml — issues never writes a repo file)",
			Sources: cli.EnvVars("LOADOUT_SDD_BACKEND"),
		},
	}, extra...)
	return &cli.Command{Name: name, Usage: usage, ArgsUsage: argsUsage, Flags: flags, Action: action}
}

// specEnv is the resolved project context every spec subcommand needs.
type specEnv struct {
	root    string // project root (git walk-up)
	dir     string // absolute specs dir
	cfg     sdd.Config
	backend sdd.Backend
	issues  *sdd.IssuesBackend // set when the backend is "issues"
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
	if b := cmd.String("backend"); b != "" {
		if !sdd.ValidBackend(b) {
			return nil, fmt.Errorf("unknown backend %q (local, github or issues)", b)
		}
		cfg.Backend = b
	}
	if labels := cmd.StringSlice("label"); len(labels) > 0 {
		cfg.Labels = labels
	}
	env := &specEnv{root: root, dir: dir, cfg: cfg}
	switch cfg.Backend {
	case sdd.BackendGitHub:
		env.backend = &sdd.GitHubBackend{Dir: dir, ProjectRoot: root, Config: &env.cfg}
	case sdd.BackendIssues:
		env.issues = &sdd.IssuesBackend{ProjectRoot: root, Config: &env.cfg}
		env.backend = env.issues
	default:
		env.backend = &sdd.LocalBackend{Dir: dir}
	}
	return env, nil
}

func (e *specEnv) specs(ctx context.Context) ([]sdd.SpecInfo, error) {
	if e.issues != nil {
		return e.issues.List(ctx)
	}
	return sdd.ListSpecs(e.dir, e.cfg)
}

func (e *specEnv) resolve(ctx context.Context, ref string) (sdd.SpecInfo, error) {
	if ref == "" {
		return sdd.SpecInfo{}, fmt.Errorf("which spec? pass a number (007), an issue (#25) or a name fragment (sdd-workflow)")
	}
	specs, err := e.specs(ctx)
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
	if env.issues != nil {
		return specInitIssue(ctx, cmd, env, name)
	}
	if cmd.IsSet("backend") {
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

// specInitIssue opens a new spec issue on the issues backend. Nothing is
// written into the repository.
func specInitIssue(ctx context.Context, cmd *cli.Command, env *specEnv, name string) error {
	style := cmd.String("style")
	if style != "full" && style != "light" {
		return fmt.Errorf("unknown --style %q (full or light)", style)
	}
	// Local spec files (if any) still reserve their numbers.
	local, err := sdd.ListSpecs(env.dir, env.cfg)
	if err != nil {
		return err
	}
	spec, err := env.issues.Create(ctx, name, local, style == "full")
	if err != nil {
		return err
	}
	fmt.Printf("Opened spec issue #%d: %s (%s)\n", spec.Issue, spec.Title, env.cfg.Repo)
	fmt.Println("Fill in the sections in the issue body; the plan goes between <!-- plan:start --> and <!-- plan:end -->.")
	fmt.Printf("Track steps with 'loadout spec check %03d <step> --backend issues' (it re-fetches the body before editing).\n", spec.Number)
	return nil
}

func specListAction(ctx context.Context, cmd *cli.Command) error {
	env, err := specSetup(cmd)
	if err != nil {
		return err
	}
	specs, err := env.specs(ctx)
	if err != nil {
		return err
	}
	if len(specs) == 0 {
		where := env.dir
		if env.issues != nil {
			where = fmt.Sprintf("issues labelled %q", env.issues.Label())
		}
		fmt.Printf("No specs in %s yet — start one with 'loadout spec init <short-name>'.\n", where)
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
		switch {
		case env.issues != nil:
			where = fmt.Sprintf("issue #%d", s.Issue)
		case s.Issue > 0:
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
		s, err := env.resolve(ctx, ref)
		if err != nil {
			return err
		}
		specs = []sdd.SpecInfo{s}
	} else if specs, err = env.specs(ctx); err != nil {
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
	spec, err := env.resolve(ctx, ref)
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
	if env.cfg.Backend == sdd.BackendIssues {
		return fmt.Errorf("spec sync is not applicable to the issues backend: the issue is the only copy, so there is nothing to reconcile")
	}
	if env.cfg.Backend != sdd.BackendGitHub {
		return fmt.Errorf("spec sync only applies to the github backend (this project uses %q)", env.cfg.Backend)
	}
	spec, err := env.resolve(ctx, cmd.Args().First())
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

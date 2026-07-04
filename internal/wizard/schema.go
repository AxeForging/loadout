// Package wizard is the builder playground: a guided questionnaire that
// turns answers about your stack and engineering conventions into canonical
// artifacts in the store. The question schema defined here is the single
// source of truth for every surface (CLI today; TUI and web reuse it).
package wizard

// QuestionType is how a question is asked and answered.
type QuestionType string

const (
	TypeSelect QuestionType = "select" // one of Options
	TypeMulti  QuestionType = "multi"  // any of Options
	TypeText   QuestionType = "text"   // free text
	TypeBool   QuestionType = "bool"   // yes/no
)

type Option struct {
	Value string `json:"value" yaml:"value"`
	Label string `json:"label" yaml:"label"`
}

type Question struct {
	ID      string       `json:"id" yaml:"id"`
	Prompt  string       `json:"prompt" yaml:"prompt"`
	Type    QuestionType `json:"type" yaml:"type"`
	Options []Option     `json:"options,omitempty" yaml:"options,omitempty"`
	Default any          `json:"default,omitempty" yaml:"default,omitempty"`
}

type Section struct {
	ID        string     `json:"id" yaml:"id"`
	Title     string     `json:"title" yaml:"title"`
	Questions []Question `json:"questions" yaml:"questions"`
}

// Sections returns the full questionnaire.
func Sections() []Section {
	return []Section{
		{
			ID:    "profile",
			Title: "Profile — what do you build?",
			Questions: []Question{
				{
					ID: "profile.appTypes", Prompt: "Which kinds of apps do you work on?", Type: TypeMulti,
					Options: opts("cli", "CLI tools", "api", "APIs / backends", "web", "Web apps (SPA/SSR)", "mobile", "Mobile apps", "library", "Libraries / SDKs"),
					Default: []string{"cli", "api"},
				},
				{
					ID: "profile.languages", Prompt: "Primary languages?", Type: TypeMulti,
					Options: opts("go", "Go", "typescript", "TypeScript", "python", "Python", "other", "Other"),
					Default: []string{"go", "typescript"},
				},
			},
		},
		{
			ID:    "toolchain",
			Title: "Toolchain — what do you use?",
			Questions: []Question{
				{ID: "toolchain.tools", Prompt: "Tools and versions (free text, e.g. \"Go 1.24, Node 22, pnpm 9\")", Type: TypeText, Default: ""},
				{
					ID: "toolchain.packageManagers", Prompt: "Package managers?", Type: TypeMulti,
					Options: opts("go-modules", "Go modules", "pnpm", "pnpm", "npm", "npm", "uv", "uv", "pip", "pip"),
					Default: []string{"go-modules", "pnpm"},
				},
				{ID: "toolchain.docker", Prompt: "Do you use Docker?", Type: TypeBool, Default: true},
				{ID: "toolchain.compose", Prompt: "Do you use docker compose for local dev?", Type: TypeBool, Default: true},
			},
		},
		{
			ID:    "standards",
			Title: "Standards — write or reference?",
			Questions: []Question{
				{
					ID: "standards.mode", Prompt: "Standards and style guides:", Type: TypeSelect,
					Options: opts("inline", "Write the essentials into the instructions", "referenced", "Reference external documents/links"),
					Default: "inline",
				},
				{ID: "standards.refs", Prompt: "Links to style guides / standards (comma-separated, optional)", Type: TypeText, Default: ""},
			},
		},
		{
			ID:    "conventions",
			Title: "Engineering conventions",
			Questions: []Question{
				{ID: "conv.lefthook", Prompt: "Use lefthook-managed git hooks?", Type: TypeBool, Default: true},
				{
					ID: "conv.hooks", Prompt: "Which git hooks?", Type: TypeMulti,
					Options: opts("fmt", "pre-commit: format staged files", "lint", "pre-commit: lint", "test", "pre-push: tests", "commit-msg", "commit-msg: enforce commit convention"),
					Default: []string{"fmt", "lint", "commit-msg"},
				},
				{
					ID: "conv.linters", Prompt: "Which linters/formatters?", Type: TypeMulti,
					Options: opts("golangci-lint", "golangci-lint (Go)", "gofumpt", "gofumpt (Go)", "eslint", "ESLint (JS/TS)", "prettier", "Prettier", "ruff", "Ruff (Python)"),
					Default: []string{"golangci-lint", "gofumpt", "eslint", "prettier"},
				},
				{
					ID: "conv.branching", Prompt: "Branching model?", Type: TypeSelect,
					Options: opts("trunk", "Trunk-based (short-lived branches off main)", "github-flow", "GitHub flow (feature branches + PRs)", "git-flow", "git-flow (develop/release branches)"),
					Default: "github-flow",
				},
				{
					ID: "conv.commits", Prompt: "Commit message convention?", Type: TypeSelect,
					Options: opts("conventional", "Conventional Commits", "conventional-scopes", "Conventional Commits with required scopes", "freeform", "Freeform"),
					Default: "conventional",
				},
				{
					ID: "conv.prTitles", Prompt: "PR title convention?", Type: TypeSelect,
					Options: opts("conventional", "Conventional Commits (squash-merge ready)", "ticket", "Ticket prefix (e.g. ABC-123: ...)", "freeform", "Freeform"),
					Default: "conventional",
				},
			},
		},
		{
			ID:    "sdd",
			Title: "Spec-driven development",
			Questions: []Question{
				{ID: "sdd.enable", Prompt: "Enable spec-driven development workflow (/plan and /spec commands)?", Type: TypeBool, Default: true},
				{ID: "sdd.folder", Prompt: "Where do specs live?", Type: TypeText, Default: "docs/specs"},
				{
					ID: "sdd.style", Prompt: "Spec template style?", Type: TypeSelect,
					Options: opts("light", "Light — problem, approach, checklist", "full", "Full — context, requirements, design, test plan, rollout"),
					Default: "light",
				},
			},
		},
		{
			ID:    "targets",
			Title: "Targets",
			Questions: []Question{
				{
					ID: "targets.agents", Prompt: "Which agents should the generated artifacts target?", Type: TypeMulti,
					Options: opts("all", "All agents", "claude-code", "Claude Code", "codex", "Codex", "gemini-cli", "Gemini CLI", "antigravity", "Antigravity", "cursor", "Cursor"),
					Default: []string{"all"},
				},
			},
		},
	}
}

// Questions flattens all sections.
func Questions() []Question {
	var out []Question
	for _, s := range Sections() {
		out = append(out, s.Questions...)
	}
	return out
}

func opts(pairs ...string) []Option {
	out := make([]Option, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, Option{Value: pairs[i], Label: pairs[i+1]})
	}
	return out
}

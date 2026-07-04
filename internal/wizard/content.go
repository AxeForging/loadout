package wizard

import (
	"fmt"
	"strings"
)

// Content builders: answers → markdown. Kept as plain Go so every branch is
// unit-testable and the output stays deterministic.

const conventionalTypes = "feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert"

func labelOf(questionID, value string) string {
	for _, q := range Questions() {
		if q.ID != questionID {
			continue
		}
		for _, o := range q.Options {
			if o.Value == value {
				return o.Label
			}
		}
	}
	return value
}

func labelsOf(a Answers, questionID string) []string {
	values := a.List(questionID)
	out := make([]string, 0, len(values))
	for _, v := range values {
		out = append(out, labelOf(questionID, v))
	}
	return out
}

func coreInstruction(a Answers) string {
	var b strings.Builder
	b.WriteString("# Working context\n\n")
	fmt.Fprintf(&b, "I primarily build: %s.\n", strings.Join(labelsOf(a, "profile.appTypes"), ", "))
	fmt.Fprintf(&b, "Primary languages: %s.\n\n", strings.Join(labelsOf(a, "profile.languages"), ", "))
	b.WriteString("General expectations:\n\n")
	b.WriteString("- Follow the conventions in the `conventions` and `toolchain` sections of this file — they are the project's source of truth.\n")
	b.WriteString("- Prefer existing project patterns over introducing new ones; check neighboring code first.\n")
	b.WriteString("- Write tests for behavior you add or change. No coverage theater — test what can break.\n")
	return b.String()
}

func toolchainInstruction(a Answers) string {
	var b strings.Builder
	b.WriteString("# Toolchain\n\n")
	if tools := strings.TrimSpace(a.Str("toolchain.tools")); tools != "" {
		fmt.Fprintf(&b, "Pinned tools and versions: %s. Use these versions — do not assume newer ones.\n\n", tools)
	}
	if pms := labelsOf(a, "toolchain.packageManagers"); len(pms) > 0 {
		fmt.Fprintf(&b, "Package managers: %s. Never mix package managers within one project.\n", strings.Join(pms, ", "))
	}
	if a.Bool("toolchain.docker") {
		b.WriteString("\nDocker is part of the workflow")
		if a.Bool("toolchain.compose") {
			b.WriteString("; local dependencies run via `docker compose up`")
		}
		b.WriteString(". See the `docker-workflows` skill.\n")
	}
	return b.String()
}

func conventionsInstruction(a Answers) string {
	var b strings.Builder
	b.WriteString("# Engineering conventions\n\n")

	fmt.Fprintf(&b, "## Branching\n\n%s\n\n", branchingText(a.Str("conv.branching")))

	b.WriteString("## Commits\n\n")
	switch a.Str("conv.commits") {
	case "conventional":
		fmt.Fprintf(&b, "Commit messages follow Conventional Commits: `<type>[optional scope][!]: <description>` with types `%s`.\n\n", conventionalTypes)
	case "conventional-scopes":
		fmt.Fprintf(&b, "Commit messages follow Conventional Commits **with a required scope**: `<type>(<scope>)[!]: <description>`, types `%s`.\n\n", conventionalTypes)
	default:
		b.WriteString("Commit messages are freeform — make them descriptive (what + why).\n\n")
	}

	b.WriteString("## Pull requests\n\n")
	switch a.Str("conv.prTitles") {
	case "conventional":
		b.WriteString("PR titles follow Conventional Commits (repos squash-merge, so the title becomes the commit message).\n\n")
	case "ticket":
		b.WriteString("PR titles start with the ticket ID, e.g. `ABC-123: short description`.\n\n")
	default:
		b.WriteString("PR titles are freeform but must describe the change, not the activity.\n\n")
	}

	if linters := labelsOf(a, "conv.linters"); len(linters) > 0 {
		fmt.Fprintf(&b, "## Linting\n\nLinters in use: %s. Run them before pushing; CI treats warnings as failures. See the `linting` skill for commands.\n\n", strings.Join(linters, ", "))
	}
	if a.Bool("conv.lefthook") {
		fmt.Fprintf(&b, "## Git hooks\n\nGit hooks are managed with lefthook (%s). Never bypass hooks with `--no-verify` unless explicitly asked.\n\n", strings.Join(labelsOf(a, "conv.hooks"), "; "))
	}
	if refs := strings.TrimSpace(a.Str("standards.refs")); refs != "" {
		fmt.Fprintf(&b, "## Standards\n\nAuthoritative style guides / standards:\n\n")
		for _, ref := range strings.Split(refs, ",") {
			fmt.Fprintf(&b, "- %s\n", strings.TrimSpace(ref))
		}
	}
	return b.String()
}

func branchingText(model string) string {
	switch model {
	case "trunk":
		return "Trunk-based development: short-lived branches off `main`, merged within a day or two. `main` is always releasable."
	case "git-flow":
		return "git-flow: feature branches off `develop`, releases via `release/*` branches, hotfixes off `main`."
	default:
		return "GitHub flow: every change is a feature branch off `main` plus a PR. `main` is always deployable; no direct pushes."
	}
}

func commitAndPRSkill(a Answers) string {
	var b strings.Builder
	b.WriteString("---\nname: commit-and-pr\ndescription: How to write commit messages, branch names and PR titles for this team's conventions. Use when committing, opening PRs, or naming branches.\n---\n\n")
	b.WriteString("# Commits, branches and PRs\n\n")
	fmt.Fprintf(&b, "## Branching\n\n%s\n\n", branchingText(a.Str("conv.branching")))

	commits := a.Str("conv.commits")
	if commits != "freeform" {
		b.WriteString("## Commit format\n\n```\n<type>")
		if commits == "conventional-scopes" {
			b.WriteString("(<scope>)")
		} else {
			b.WriteString("[optional scope]")
		}
		b.WriteString("[!]: <description>\n```\n\n")
		fmt.Fprintf(&b, "Allowed types: `%s`.\n\n", conventionalTypes)
		b.WriteString("Validation regex:\n\n```\n^(" + conventionalTypes + ")")
		if commits == "conventional-scopes" {
			b.WriteString(`\(.+\)`)
		} else {
			b.WriteString(`(\(.+\))?`)
		}
		b.WriteString("(!)?: .+\n```\n\n")
		b.WriteString("Examples:\n\n- `feat: add sync command`\n- `fix(engine): classify block drift correctly`\n- `feat!: breaking change`\n\n")
	}

	switch a.Str("conv.prTitles") {
	case "conventional":
		b.WriteString("## PR titles\n\nSame Conventional Commits format as commit messages — the repo squash-merges, so the PR title becomes the final commit message on main.\n")
	case "ticket":
		b.WriteString("## PR titles\n\nStart with the ticket ID: `ABC-123: short description`.\n")
	}
	if a.Bool("conv.lefthook") {
		b.WriteString("\n## Git hooks\n\nHooks are managed by lefthook — a ready-to-use config is in `references/lefthook.yml`. Install with `lefthook install`. Never suggest `--no-verify`.\n")
	}
	return b.String()
}

func lefthookSnippet(a Answers) string {
	var b strings.Builder
	b.WriteString("# lefthook.yml — generated by loadout wizard\n")
	b.WriteString("# Install: go install github.com/evilmartians/lefthook@latest && lefthook install\n\n")

	hooks := a.List("conv.hooks")
	linters := a.List("conv.linters")
	pre := []string{}
	if contains(hooks, "fmt") {
		if contains(linters, "gofumpt") {
			pre = append(pre, "    gofumpt:\n      glob: \"*.go\"\n      run: gofumpt -l -w {staged_files}\n      stage_fixed: true")
		}
		if contains(linters, "prettier") {
			pre = append(pre, "    prettier:\n      glob: \"*.{ts,tsx,js,jsx,json,css,md}\"\n      run: npx prettier --write {staged_files}\n      stage_fixed: true")
		}
	}
	if contains(hooks, "lint") {
		if contains(linters, "golangci-lint") {
			pre = append(pre, "    golangci:\n      glob: \"*.go\"\n      run: golangci-lint run --timeout=5m")
		}
		if contains(linters, "eslint") {
			pre = append(pre, "    eslint:\n      glob: \"*.{ts,tsx,js,jsx}\"\n      run: npx eslint {staged_files}")
		}
		if contains(linters, "ruff") {
			pre = append(pre, "    ruff:\n      glob: \"*.py\"\n      run: ruff check {staged_files}")
		}
	}
	if len(pre) > 0 {
		b.WriteString("pre-commit:\n  parallel: true\n  commands:\n")
		b.WriteString(strings.Join(pre, "\n"))
		b.WriteString("\n")
	}
	if contains(hooks, "test") {
		b.WriteString("\npre-push:\n  commands:\n    tests:\n      run: make test\n")
	}
	if contains(hooks, "commit-msg") && a.Str("conv.commits") != "freeform" {
		scope := `(\(.+\))?`
		if a.Str("conv.commits") == "conventional-scopes" {
			scope = `\(.+\)`
		}
		fmt.Fprintf(&b, `
commit-msg:
  commands:
    commitlint:
      run: |
        msg=$(cat {1})
        if ! echo "$msg" | grep -qE "^(%s)%s(!)?: .+"; then
          echo "Error: commit message must follow the team convention"
          exit 1
        fi
`, conventionalTypes, scope)
	}
	return b.String()
}

func lintingSkill(a Answers) string {
	var b strings.Builder
	b.WriteString("---\nname: linting\ndescription: Which linters this team uses and how to run and fix them. Use before pushing or when lint errors appear.\n---\n\n# Linting\n\n")
	for _, l := range a.List("conv.linters") {
		switch l {
		case "golangci-lint":
			b.WriteString("## golangci-lint (Go)\n\n- Run: `golangci-lint run --timeout=5m`\n- Config lives in `.golangci.yml`; treat every finding as a failure.\n\n")
		case "gofumpt":
			b.WriteString("## gofumpt (Go)\n\n- Run: `gofumpt -w .` (stricter than gofmt — modern octal literals, no blank line before `}`)\n\n")
		case "eslint":
			b.WriteString("## ESLint (JS/TS)\n\n- Run: `npx eslint .` — fix: `npx eslint . --fix`\n\n")
		case "prettier":
			b.WriteString("## Prettier\n\n- Check: `npx prettier --check .` — write: `npx prettier --write .`\n\n")
		case "ruff":
			b.WriteString("## Ruff (Python)\n\n- Run: `ruff check .` — fix: `ruff check . --fix`; format: `ruff format .`\n\n")
		}
	}
	b.WriteString("Run the relevant linters before every push; CI runs them all and fails on any finding.\n")
	return b.String()
}

func dockerSkill(a Answers) string {
	var b strings.Builder
	b.WriteString("---\nname: docker-workflows\ndescription: How this team uses Docker for local development. Use when starting services, debugging containers, or writing Dockerfiles.\n---\n\n# Docker workflows\n\n")
	if a.Bool("toolchain.compose") {
		b.WriteString("## Local development\n\n- Start dependencies: `docker compose up -d`\n- Tail logs: `docker compose logs -f <service>`\n- Reset state: `docker compose down -v` (destroys volumes — ask before running)\n\n")
	}
	b.WriteString("## Conventions\n\n- Multi-stage builds; final images run as a non-root user.\n- Pin base image tags (no `latest`).\n- `.dockerignore` mirrors `.gitignore` plus build artifacts.\n")
	return b.String()
}

func sddInstruction(a Answers) string {
	folder := a.Str("sdd.folder")
	var b strings.Builder
	b.WriteString("# Spec-driven development\n\n")
	fmt.Fprintf(&b, "Non-trivial features start with a spec in `%s/` before any code. Use `/spec` to draft one and `/plan` to derive the implementation plan.\n\n", folder)
	fmt.Fprintf(&b, "- Specs are numbered: `%s/NNN-short-name.md`.\n", folder)
	b.WriteString("- A spec must be reviewed (or explicitly waived) before implementation.\n")
	b.WriteString("- The plan references the spec; the PR references both.\n")
	return b.String()
}

func planCommand(a Answers) string {
	folder := a.Str("sdd.folder")
	return fmt.Sprintf(`---
description: Derive a step-by-step implementation plan from a spec.
---

Read the spec the user points at (default: the most recent file in %s/).
Produce an implementation plan:

1. List the concrete steps in dependency order, each small enough to verify independently.
2. Name the files to create or modify per step.
3. Call out risks, migrations, and rollback strategy.
4. Define "done": tests to add, how to verify end-to-end.

Keep the plan reviewable — terse bullets, no prose padding. Ask before deviating from the spec.
`, folder)
}

func specCommand(a Answers) string {
	folder := a.Str("sdd.folder")
	if a.Str("sdd.style") == "full" {
		return fmt.Sprintf(`---
description: Draft a full spec (context, requirements, design, test plan, rollout) for a feature.
---

Create %s/NNN-<short-name>.md (next free number) with sections:

## Context — why now, what problem, who is affected
## Requirements — numbered, testable statements (MUST/SHOULD)
## Non-goals — explicitly out of scope
## Design — approach, alternatives considered and why rejected
## Data & API changes — schemas, endpoints, migrations
## Test plan — per requirement, how it is verified
## Rollout — flags, migration order, rollback

Interview me for anything ambiguous before writing. Keep requirements independently verifiable.
`, folder)
	}
	return fmt.Sprintf(`---
description: Draft a light spec (problem, approach, checklist) for a feature.
---

Create %s/NNN-<short-name>.md (next free number) with sections:

## Problem — one paragraph, user-visible framing
## Approach — the chosen solution in a few bullets, plus what was ruled out
## Checklist — verifiable steps to done, including tests

Ask clarifying questions first if the problem statement is ambiguous.
`, folder)
}

func sddWorkflowSkill(a Answers) string {
	folder := a.Str("sdd.folder")
	return fmt.Sprintf(`---
name: sdd-workflow
description: The team's spec-driven development flow — when specs are required, where they live, and how /spec and /plan chain together. Use when starting any non-trivial feature.
---

# Spec-driven development workflow

1. **Spec first** — for any feature beyond a small fix, run /spec to draft %s/NNN-<name>.md.
2. **Review** — the spec is reviewed (or waived explicitly in the PR description).
3. **Plan** — run /plan against the spec to derive an ordered implementation plan.
4. **Implement** — follow the plan; if reality diverges from the spec, update the spec in the same PR.
5. **PR** — reference the spec file in the PR body.

Never start implementation of a large feature without a spec — if asked to, suggest drafting one with /spec first.
`, folder)
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

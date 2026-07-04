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
description: Derive (or continue) a persistent, checkbox-tracked implementation plan from a spec.
---

Target spec: the one the user points at, else the most recent file in %[1]s/.
The plan LIVES ON DISK next to its spec as %[1]s/NNN-<name>.plan.md — it is the
durable progress tracker that survives restarts, crashes and agent switches.

1. If the plan file already exists: READ it, report progress (checked vs
   unchecked), and CONTINUE from the first unchecked step — do not regenerate.
2. Otherwise derive the plan from the spec and WRITE the file:
   - Concrete steps in dependency order as "- [ ]" checkboxes, each small
     enough to verify independently.
   - Per step: the files to create or modify.
   - A "Risks" section (migrations, rollback) and a "Done means" section
     (tests to add, how to verify end-to-end).

While implementing, KEEP THE FILE CURRENT: check off each step ("- [x]") the
moment it is done and verified, and append newly discovered steps instead of
holding them in memory. Ask before deviating from the spec.
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
description: The team's spec-driven development flow — when specs are required, where specs and their persistent plans live, and how /spec, /plan, /handoff and /resume chain together. Use when starting any non-trivial feature or resuming one.
---

# Spec-driven development workflow

1. **Spec first** — for any feature beyond a small fix, run /spec to draft %[1]s/NNN-<name>.md.
2. **Review** — the spec is reviewed (or waived explicitly in the PR description).
3. **Plan on disk** — run /plan: it writes %[1]s/NNN-<name>.plan.md, an ordered
   "- [ ]" checklist. The plan file — not the conversation — is the source of
   truth for progress.
4. **Implement with the checklist** — check off steps as they complete and
   append discovered steps. If reality diverges from the spec, update the spec
   in the same PR.
5. **PR** — reference the spec and plan files in the PR body.

## Crash / restart / agent-switch safety

Because progress lives in the plan file, work is resumable by design:
- After a restart or crash, or in a different agent: run /resume — it reads
  the context handoff (if any) AND the active plan file, then continues at the
  first unchecked step.
- Before ending a session mid-feature, run /handoff and make sure the handoff
  references the active spec and plan paths.
- Never keep plan state only in your head or the chat — if a step completed,
  its checkbox must be checked in the file before moving on.

Never start implementation of a large feature without a spec — if asked to, suggest drafting one with /spec first.
`, folder)
}

func testingInstruction(a Answers) string {
	var b strings.Builder
	b.WriteString("# Testing strategy\n\n")
	b.WriteString("General rules:\n\n")
	b.WriteString("- Tests are mandatory for behavior you add or change. Behavior-named tests (`TestSync_DetectsLocalDrift`, `renders empty state when list is empty`), no coverage theater.\n")
	b.WriteString("- Never assert the mock — test observable behavior.\n\n")

	levels := a.List("quality.levels")
	if contains(levels, "unit") {
		b.WriteString("## Unit tests\n\n")
		for _, tool := range a.List("quality.unitTools") {
			switch tool {
			case "go-testing":
				b.WriteString("- Go: stdlib `testing` only (no assertion frameworks); table tests where natural; `t.TempDir()` for filesystem; run `go test -race ./...`.\n")
			case "vitest":
				b.WriteString("- TS/JS: Vitest — colocate `*.test.ts` next to source; run `pnpm test`.\n")
			case "jest":
				b.WriteString("- TS/JS: Jest — colocate `*.test.ts`; run `npm test`.\n")
			case "pytest":
				b.WriteString("- Python: pytest — `tests/` mirrors package layout; run `pytest -q`.\n")
			}
		}
		b.WriteString("\n")
	}
	if contains(levels, "component") && a.Str("quality.feFramework") != "none" {
		fe := labelOf("quality.feFramework", a.Str("quality.feFramework"))
		fmt.Fprintf(&b, "## Component tests (%s)\n\n- Use Testing Library idioms: query by role/label like a user would, never by implementation details (class names, internals).\n- Cover: renders with real props, user interaction paths, loading/empty/error states.\n\n", fe)
	}
	if contains(levels, "integration") {
		b.WriteString("## Integration tests\n\n- Exercise the real wiring (real DB in a container or temp dir, real HTTP handlers), not unit tests with more mocks.\n- Keep them deterministic: no sleeps, no shared state between tests, no network beyond local containers.\n\n")
	}
	if contains(levels, "e2e") && a.Str("quality.e2eTool") != "none" {
		tool := labelOf("quality.e2eTool", a.Str("quality.e2eTool"))
		fmt.Fprintf(&b, "## End-to-end tests (%s)\n\n- A handful of critical user journeys, not a parallel test pyramid.\n- Stable selectors (`data-testid` / roles), auto-waiting assertions, no fixed sleeps.\n\n", tool)
	}

	b.WriteString("## Mocking policy\n\n")
	switch a.Str("quality.mocking") {
	case "minimal":
		b.WriteString("Minimal mocking: prefer real implementations and in-memory fakes. A mock is a last resort and needs a justification.\n")
	case "liberal":
		b.WriteString("Mocks are acceptable wherever they meaningfully speed tests up — but the critical paths still need at least one unmocked test.\n")
	default:
		b.WriteString("Mock external boundaries only (third-party HTTP APIs, payment providers, clocks). Everything inside the codebase runs real in tests.\n")
	}
	return b.String()
}

func handoffCommand() string {
	return `---
description: Dump the current session context to .agents/context/handoff.md so any agent (or a later session) can pick up exactly where this one left off.
---

Write a context handoff to ` + "`.agents/context/handoff.md`" + ` (create the directory if needed, overwrite the previous handoff). Structure it exactly like this:

# Handoff — <date> <time>

## Goal
What we are ultimately trying to achieve, in one or two sentences.

## Current state
What is DONE and verified, what is in progress, what is untouched. Reference concrete files and branches. If a spec/plan is active (docs/specs/*.md + *.plan.md), name both files — the plan's checkboxes carry the step-level progress.

## Decisions & constraints
Choices already made (and why), approaches ruled out, constraints to respect.

## Next steps
Ordered, concrete, resumable steps. The first one should be actionable immediately.

## Gotchas
Anything surprising discovered this session that would waste the next agent's time to rediscover.

Keep it dense and factual — this is for another AI agent, not a status report for humans. After writing, confirm the path so the user can commit it.
`
}

func resumeCommand() string {
	return `---
description: Resume work from the last context handoff written by /handoff (from any agent or a previous session).
---

1. Read ` + "`.agents/context/handoff.md`" + `. If it does not exist, say so and ask what to work on instead.
2. If the handoff references a spec/plan (or ` + "`docs/specs/*.plan.md`" + ` files exist), read the active plan file — its unchecked "- [ ]" items are the authoritative remaining work.
3. Cross-check against reality: run ` + "`git status`" + ` and ` + "`git log --oneline -5`" + `, and spot-check any file the handoff claims was changed.
4. Summarize in a few lines: the goal, where things stand, and the first next step. Flag any drift between the handoff/plan and the actual repo state.
5. Continue with the first unchecked plan step (or the handoff's first next step). Respect recorded decisions — do not relitigate them without new information. Keep checking off plan items as you complete them.
`
}

func contextHandoffSkill() string {
	return `---
name: context-handoff
description: The team's cross-agent context handoff convention — when and how to dump session state so another agent, another tool, or a later session can continue seamlessly. Use when ending a session, switching agents, or before context gets compacted.
---

# Context handoff

Session context dies with the session — unless it is written down in a place every agent reads. This team's convention:

- **Location**: ` + "`.agents/context/handoff.md`" + ` in the project root (agent-agnostic — Claude Code, Codex, Gemini CLI, Antigravity and Cursor can all read and write it).
- **Write** a handoff (via /handoff or manually, same format) whenever: the session is ending mid-task, the user says they will continue later or in another tool, or the conversation is about to be compacted and important context is only in the chat.
- **Read** it (via /resume) at the start of a session when it exists and is recent; verify it against git state before trusting it.
- **Format**: Goal · Current state · Decisions & constraints · Next steps · Gotchas. Dense, factual, written for an AI agent.
- **Lifecycle**: one live handoff per project; it is safe to commit. Delete or rewrite it when the task completes — a stale handoff is worse than none.
`
}

func contains(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

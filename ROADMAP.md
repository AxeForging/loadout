# loadout Roadmap

loadout is built in shippable milestones — every milestone leaves `main` releasable. Status legend: ✅ done · 🚧 in progress · ⏳ planned.

## Where we are

| | Milestone | Status |
|---|-----------|--------|
| M1 | Core store + sync engine + Claude Code adapter | ✅ shipped |
| M2 | All five agent adapters + `doctor` + `add` | ✅ shipped |
| M3 | External git skill sources (pinned, gated updates) | ✅ shipped |
| M4 | Shareable bundles (export/import) | ✅ shipped — **released as v0.4.0** |
| M5 | Builder wizard (conventions + SDD presets) | ✅ shipped — released as v0.5.0 |
| M6 | TUI dashboard | ✅ shipped — released as v0.6.0 |
| M6.5 | Continuity (context handoff) + testing strategy + AI drafting via logged-in agents + no-args overview | ✅ shipped |
| M7 | Embedded web UI (React SPA) | ✅ shipped — released as v0.7.0 |
| M7.5 | Crash-safe SDD plans + EXAMPLES walkthrough (real-session proof) | ✅ shipped |
| M8 | v1.0 hardening | ✅ shipped — **released as v1.0.0** |

**The v1 roadmap is complete.** Post-1.0 ideas live under "Open questions / parked" below.

## M1 — Core + Claude Code adapter ✅ (v0.1.0)

- ✅ Canonical git-friendly store: `loadout.yaml` manifest + `skills/`, `instructions/`, `commands/`, `hooks/`, `settings/`
- ✅ Sync engine: plan/diff/status/apply, three-way drift classification (`in-sync`, `missing`, `drifted-local`, `drifted-store`, `conflict`), machine-local state in the platform data dir
- ✅ Merge strategies: `replace`, sentinel-marked managed blocks, JSON deep-merge (semantic no-op keeps user formatting)
- ✅ Hard confirmation gate — nothing is written without it (permanent invariant test)
- ✅ Claude Code adapter: skills, `CLAUDE.md` managed blocks, commands, hooks (script + `settings.json` merge), settings fragments
- ✅ CLI: `init` · `status` · `diff` · `sync` · `install` · `new` · `agents` · `version` · `completion`
- ✅ Full CI: conventional-commit gate, reviewforge, structlint, golangci-lint v2, race tests, integration suite, goreleaser + releaseforge

## M2 — All adapters + doctor + adopt ✅ (v0.2.0)

- ✅ Codex adapter: skills → `~/.codex/skills/`, instructions → managed block in `~/.codex/AGENTS.md`, commands rendered as skills (native prompts are deprecated)
- ✅ Gemini CLI adapter: skills → `~/.gemini/skills/`, instructions → `~/.gemini/GEMINI.md` block, commands as skills, settings → `settings.json` JSON-merge
- ✅ Antigravity adapter: skills → `~/.gemini/antigravity-cli/skills/`, instructions via the shared `GEMINI.md`
- ✅ Cursor adapter: skills → `~/.cursor/skills/`, commands as skills (Cursor treats skills as slash-capable)
- ✅ `loadout doctor`: verify agent homes on this machine, artifact paths and hook specs in the store, balanced managed-block markers, orphaned state records
- ✅ `loadout add <path>`: adopt an existing native artifact (skill dir, command/instruction file, hook dir) into the store
- Deferred from this milestone (tracked below): Codex `config.toml` merge (a TOML rewrite would destroy user comments — needs a comment-preserving strategy), Cursor `.mdc` rules and Antigravity workflows (project-scoped; loadout manages global homes until project-scope support lands)

## M3 — External sources ✅ (v0.3.0)

- ✅ `loadout source add|list|remove <url>` — track skills from external git repos
- ✅ `sources.lock.yaml` committed in the store: url, ref, pinned commit, subdir — reproducible and shareable
- ✅ `loadout check` — non-intrusive update check (`git ls-remote`, no clone); opt-in `updates.checkOnRun` with cached results and a single hint line, never auto-apply
- ✅ `loadout update [source]` — fetch, show commit log + artifact diff, permission-gated apply; user-owned (origin `local`) artifacts are never overwritten, upstream-removed skills are kept and relabeled
- ✅ Design change vs. the original plan: git operations shell out to the **system git** instead of go-git — private repos authenticate through the user's existing SSH agent / credential helpers, so the keyring secrets package became unnecessary
- ⏳ `skillsStrategy: shared-agents-dir` — install skills once into `~/.agents/skills/` for agents that read it (Cursor, Gemini CLI) — moved to M4

## M4 — Bundles ✅ (v0.4.0)

- ✅ `loadout bundle export [--archive]` — one shareable artifact of the whole setup (git-friendly dir by default, `.loadout.tar.gz` optional)
- ✅ `bundle.yaml` metadata + the store tree (manifest, artifacts, sources.lock) — sources not registered locally are merged on import
- ✅ `loadout bundle import <path>` — schema validation, merge plan summary, permission-gated apply with conflict policy (`skip` default, `overwrite` opt-in); archive extraction rejects path traversal
- ⏳ import from URL and a `rename` conflict policy — deferred
- ⏳ `skillsStrategy: shared-agents-dir` (carried from M3) — install skills once into `~/.agents/skills/` for Cursor + Gemini CLI

## M5 — Builder wizard ✅ (v0.5.0)

- ✅ `loadout wizard` (alias `create`) — guided questionnaire → generated artifacts; single question schema (`internal/wizard/schema.go`) shared by CLI (charmbracelet/huh) and, later, TUI and web
- ✅ Sections: profile (app types, languages) · toolchain (tools + versions, package managers, docker) · standards (inline vs referenced) · targets (which agents)
- ✅ **Engineering conventions**: githooks (lefthook + which hooks), linters (golangci-lint/gofumpt/eslint/prettier/ruff), branching model (trunk / GitHub flow / git-flow), commit convention (Conventional Commits ± required scopes), PR title convention → generates `instructions/conventions.md`, a `commit-and-pr` skill with the validation regex + a ready `lefthook.yml` in `references/`, and a `linting` skill
- ✅ **Spec-driven development**: `/plan` + `/spec` commands (light or full template), `sdd-workflow` skill, `instructions/sdd.md`
- ✅ Non-interactive: `--preset go-cli|web-app`, `--answers file.yaml` (validated against the schema), `--yes`; store writes are confirmation-gated; existing artifacts skipped, never overwritten
- ✅ Engine fix uncovered by M5: multiple managed blocks merging into the same file within one sync now compose instead of clobbering each other (permanent regression test)
- ⏳ Custom skill builder (freeform task → agentskills.io-valid SKILL.md) — lands with the web `/builder` where the UX fits; `loadout new skill` covers scaffolding meanwhile

## M6 — TUI ✅ (v0.6.0)

- ✅ Bubble Tea dashboard (`loadout tui`, alias `dash`): Overview (agent cards + drift counts + pending changes) · Store (artifact browser + content preview) · Agents (drift rows, `d` diff view, `s` sync behind a confirm modal) · Sources (pins, `c` on-demand remote check) · Log (session activity)
- ✅ The confirmation gate holds in the TUI: declining the modal dispatches nothing (unit-tested via model messages, no PTY needed)

## M6.5 — Continuity, testing strategy, AI drafting ✅

- ✅ **Context handoff**: wizard continuity section generates `/handoff` (dump structured session state to `.agents/context/handoff.md`) + `/resume` (verify against git, continue) + a `context-handoff` skill — cross-agent and restart-proof by convention
- ✅ **Testing strategy** wizard section: mandatory levels (unit/component/integration/e2e), FE framework, unit runners, e2e tool, mocking policy → `instructions/testing.md` tailored per answer
- ✅ **Headless runner layer** (`internal/runner`): drives already-logged-in agents (`claude -p --output-format json`, `codex exec` read-only sandbox, `gemini -p`) — no API keys, rides the user's plan. `loadout new skill --ai "desc"` drafts validated SKILL.md; `loadout wizard --ai` deepens testing/conventions/toolchain for the exact stack. Templates stay the deterministic base; AI output is validated + confirmation-gated; `loadout agents` shows a HEADLESS column
- ✅ Bare `loadout` shows a friendly overview (store, agents, pending changes, contextual next steps) instead of a help wall

## M7 — Web UI ✅ (v0.7.0)

- ✅ Embedded React SPA (React 19 + Vite + Tailwind v4 + TanStack Router/Query) built into the binary via `embed.FS`; **dist/ is committed** so `go install` ships the real UI. Zustand dropped — TanStack Query + local state covers everything
- ✅ `loadout serve [--dev]` on localhost — REST JSON API; **two-phase sync** (POST plan → single-use token → POST apply) is the permission gate over HTTP; wizard schema served as JSON so the Builder reuses the exact CLI questionnaire
- ✅ Routes: overview (drift counts, agents, bundle export) · store browser + file viewer · drift matrix with inline colored diffs · sources with read-only remote check · Builder (multi-step form, preset chips, review-before-write)
- ✅ Design via the impeccable craft flow (`.impeccable.md`): warm-steel dark armory theme, Chakra Petch / Schibsted Grotesk / Red Hat Mono, docked pending-changes bar instead of modals, square checklist state markers; verified live with Playwright
- ⏳ Deferred: SSE push (the server is request-driven; refetch-on-action covers it), bundle import upload (CLI covers it), settings page

## M7.5 — Continuity hardening + docs ✅

- ✅ **Crash-safe SDD**: `/plan` writes `docs/specs/NNN-<name>.plan.md` as a checked-off-as-you-go checklist — the plan file, not the conversation, carries progress; `/resume` continues at the first unchecked item; `/handoff` references the active spec/plan pair
- ✅ **EXAMPLES.md**: hands-on walkthrough with a real dogfood transcript — a live Claude Code session (Haiku) driven via yoink executed the loadout-installed `/spec` and `/handoff`; AI drafting shown with a real logged-in `agy` run

## M8 — v1.0 hardening ✅ (v1.0.0)

- ✅ Full-matrix e2e: the complete wizard kit + a hook synced across all five fake agent homes ends fully in-sync with a clean `doctor`; deleting an installed file by hand is detected and `sync` restores it
- ✅ `doctor` completeness: new vanished-install check (recorded installs missing from disk) alongside store validation, agent detection, managed-block balance and orphaned-state reporting
- ✅ Docs: EXAMPLES.md linked from the README; README current-release line kept honest

## Open questions / parked

- Codex `config.toml` settings merge without destroying user comments (comment-preserving TOML editor or managed `# loadout:begin/end` line block)
- Project-scoped artifacts (`.cursor/rules/*.mdc`, `.agents/{rules,workflows,skills}`, per-repo CLAUDE.md) — per-project loadout manifests
- Self-update check (GitHub Releases + ETag, opt-in, print-only)
- Adapter path re-verification: agent home layouts move fast; `doctor` is the runtime source of truth, docs are point-in-time (verified 2026-07-04)

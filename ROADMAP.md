# loadout Roadmap

loadout is built in shippable milestones — every milestone leaves `main` releasable. Status legend: ✅ done · 🚧 in progress · ⏳ planned.

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

## M5 — Builder wizard ⏳ (v0.5.0)

- `loadout wizard` (alias `create`) — guided questionnaire → generated artifacts; single question schema shared by CLI (charmbracelet/huh), TUI and web
- Sections: profile (app types, languages) · toolchain (tools + versions, package managers, docker) · standards (inline vs referenced)
- **Engineering conventions**: githooks (lefthook + which hooks), linters (golangci-lint/eslint/ruff), branching model (trunk / git-flow / GitHub flow), commit convention (Conventional Commits + scopes), PR title convention → generates `instructions/conventions.md`, a `commit-and-pr` skill, a `commit-msg-lint` hook + emitted `lefthook.yml` snippet, a `linting` skill
- **Spec-driven development**: `/plan` + `/spec` commands, `sdd-workflow` skill, folder conventions; presets `sdd-light` / `sdd-full`
- Custom skill builder producing agentskills.io-valid `SKILL.md`

## M6 — TUI ⏳ (v0.6.0)

- Bubble Tea dashboard (`loadout tui`), per-tab sub-models: Overview (agent cards + drift counts) · Store (artifact browser + preview) · Agents (status, diff view, gated sync modal) · Sources (pin vs remote) · Log (engine events)

## M7 — Web UI ⏳ (v0.7.0)

- Embedded React SPA (React 19 + Vite + Tailwind v4 + TanStack Router/Query + Zustand) built into the binary via `embed.FS` — single-binary distribution preserved
- `loadout serve [--dev]` — REST + SSE; **two-phase sync** (plan → token → apply) as the permission gate over HTTP
- Routes: dashboard · store browser with markdown preview · per-agent status + diff + gated sync · sources · bundles (export download / import upload) · `/builder` (the wizard as a multi-step form) · settings

## M8 — v1.0 hardening ⏳

- End-to-end matrix across all five fake agent homes; `doctor` completeness; error-message polish; docs/EXAMPLES; v1.0.0

## Open questions / parked

- Codex `config.toml` settings merge without destroying user comments (comment-preserving TOML editor or managed `# loadout:begin/end` line block)
- Project-scoped artifacts (`.cursor/rules/*.mdc`, `.agents/{rules,workflows,skills}`, per-repo CLAUDE.md) — per-project loadout manifests
- Self-update check (GitHub Releases + ETag, opt-in, print-only)
- Adapter path re-verification: agent home layouts move fast; `doctor` is the runtime source of truth, docs are point-in-time (verified 2026-07-04)

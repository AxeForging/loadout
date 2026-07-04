# CLAUDE.md — loadout

AI assistant instructions for working with the loadout codebase.

## Project Overview

loadout is a Go CLI tool ("dotfiles manager + package manager for AI-agent configs"). It keeps skills, instructions, slash commands, hooks and settings in one canonical, git-friendly store and installs them into each coding agent's native home (claude-code today; codex, gemini-cli, antigravity, cursor planned) through pluggable adapters, with drift detection and a hard confirmation gate before any write.

**Repository:** https://github.com/AxeForging/loadout
**Organization:** https://github.com/AxeForging

---

## Safety Invariants (NON-NEGOTIABLE)

1. **The engine is the ONLY package that writes into agent homes.** Adapters are pure translators (artifact in → target files out); CLI/TUI/web orchestrate.
2. **Nothing is written without confirmation.** `engine.Apply` requires a confirm callback returning true; nil or false writes nothing. The integration suite has a permanent invariant test for this — never weaken it.
3. **User content is never touched.** Instructions go into sentinel-marked managed blocks (`<!-- loadout:begin <id> -->`); settings and hooks deep-merge into JSON, never replace files. Content outside markers is preserved byte-for-byte.
4. **The store stays shareable.** Machine-local state (install hashes) lives in the platform data dir, never inside the store repo. No secrets in the store.

---

## Directory Structure

```
loadout/
├── cmd/loadout/main.go         # Binary entry point
├── internal/
│   ├── app/app.go              # Root CLI command wiring
│   ├── cli/                    # One file per command; root.go registers all; helpers.go builds the adapter registry
│   ├── domain/                 # Pure shared types (Artifact, Manifest, SyncState, HookSpec) — imports nothing local
│   ├── store/                  # Canonical store: manifest IO, artifact content, hashing, scaffolds
│   ├── adapter/                # Adapter interface + registry + managed-block helpers
│   │   └── claudecode/         # One package per agent
│   ├── engine/                 # Plan/Status/Apply, merge strategies, drift classification, state lockfile
│   ├── source/                 # External git skill repos: sources.lock.yaml, discovery, check/update (system git via exec)
│   ├── bundle/                 # Shareable bundles: export (dir/tar.gz) + validated, gated import
│   ├── config/                 # loadout's own config + platform paths
│   ├── build/info.go           # Version info injected at build time
│   └── logging/logging.go      # Zerolog setup
├── integration/                # Integration tests (build tag: integration, temp HOME)
└── Makefile
```

Import direction: `domain` imports nothing local; `store` and `adapter/*` import `domain` only; adapters never import `engine`, `store` or each other; `engine` imports `domain`/`store`/`adapter`; `source` imports `domain`/`store`; `cli` orchestrates everything; nothing imports `cli`.

Git rule: all git invocations shell out to the system `git` (never go-git) so user credentials (SSH agent, credential helpers) just work and loadout never stores secrets. Always pass `GIT_TERMINAL_PROMPT=0`.

---

## Development Rules

1. **README stays in sync** — any flag/command/output change updates README.md in the same PR.
2. **Tests are mandatory** — behavior-named unit tests alongside every file with logic; integration tests (build tag `integration`) drive the real binary against a temp HOME with fake agent homes. No coverage theater.
3. **Adding a new command**: `internal/cli/<name>.go` with `<name>Cmd() *cli.Command`, register in root.go, README section, update completion scripts and `.structlint.yaml` if structurally required.
4. **Adding a new agent adapter**: `internal/adapter/<agent>/`, implement the `adapter.Adapter` interface with agent paths kept in data (not scattered logic), register in `internal/cli/helpers.go`, add render unit tests (golden files for tricky formats) and an integration scenario with a fake home.
5. **PR titles** — Conventional Commits (`feat|fix|docs|style|refactor|test|chore|build|ci|perf|revert`), enforced by CI.
6. **Run before pushing:** `make ci` and `make test-integration`.

---

## Key Dependencies

| Package | Role |
|---------|------|
| `github.com/urfave/cli/v3` | CLI framework |
| `github.com/rs/zerolog` | Structured logging |
| `github.com/goccy/go-yaml` | Manifest/state/config YAML (+ YAMLToJSON for settings fragments) |
| `github.com/pmezard/go-difflib` | Unified diffs for `loadout diff` |

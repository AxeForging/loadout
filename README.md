# loadout

Your agentic gear kit. loadout keeps AI-agent configs — skills, instructions, slash commands, hooks, settings — in one canonical, git-friendly store and installs them into each coding agent's native home through pluggable adapters.

Skills use the open [Agent Skills](https://agentskills.io) standard (`SKILL.md`), so the canonical format is the native format: loadout places them, tracks them, and keeps them in sync everywhere.

## Why

Agent configs are fragmented across `~/.claude`, `~/.codex`, `~/.gemini`, `~/.cursor` and project `.agents/` directories: no sync, no versioning, no team-sharing, no reinstall story. loadout treats them like dotfiles with a package manager on top:

- **One canonical store** — a plain directory (and git repo) holding your whole setup.
- **Pluggable adapters** — install into Claude Code, Codex, Gemini CLI, Antigravity and Cursor.
- **Drift detection** — see per artifact and per agent what is in sync, edited on disk, updated in the store, or missing.
- **Never writes without asking** — every apply goes through a confirmation gate; user-authored content outside loadout's managed blocks is never touched.

## Install

```bash
go install github.com/AxeForging/loadout/cmd/loadout@latest
```

Or grab a binary from [Releases](https://github.com/AxeForging/loadout/releases).

## Quick start

```bash
loadout init                      # create the canonical store
loadout new skill code-review    # scaffold a skill (SKILL.md standard)
loadout new instruction style    # shared instructions for every agent
loadout status                   # drift matrix: artifact × agent
loadout sync                     # install into detected agents (asks first)
```

## Commands

| Command | Purpose |
|---------|---------|
| `loadout init` | Create the canonical store (git-friendly directory with a starter manifest) |
| `loadout status [artifact...]` | Drift matrix: every artifact's sync state per agent |
| `loadout diff [artifact...]` | Unified diffs of what sync would change |
| `loadout sync [artifact...]` | Install/update artifacts in agent homes — always asks before writing (`--dry-run`, `--yes`, `--agent`) |
| `loadout install <agent>` | First-time full install into one agent |
| `loadout new <kind> <name>` | Scaffold a `skill`, `instruction`, `command` or `hook` in the store |
| `loadout add <path>` | Adopt an existing native artifact (skill/hook dir, command/instruction file) into the store |
| `loadout source add\|list\|remove <url>` | Track skills from external git repos (pinned in `sources.lock.yaml`) |
| `loadout check` | Check sources for updates — `git ls-remote` only, nothing fetched or applied |
| `loadout update [source]` | Fetch updates, show changelog + artifact diff, apply to the store after confirmation |
| `loadout agents` | List supported agents and whether they are installed |
| `loadout doctor` | Verify the store, agent homes, managed-block markers and install state |
| `loadout version` | Print build version information |
| `loadout completion <shell>` | Shell completion scripts (bash, zsh, fish) |

Global flags: `--store <path>` (env `LOADOUT_STORE`), `--log-level` (env `LOADOUT_LOG_LEVEL`), `--no-color`.

## The canonical store

By default at `~/.local/share/loadout/store` (override with `storePath` in `~/.config/loadout/config.yaml` or `--store`). It is a plain directory — point it at your dotfiles repo if you like:

```
store/
├── loadout.yaml            # manifest: artifacts, kinds, targets
├── skills/<name>/SKILL.md  # Agent Skills standard, verbatim
├── instructions/*.md       # concatenated into managed blocks in CLAUDE.md etc.
├── commands/<name>.md      # slash-command prompts
├── hooks/<name>/           # hook.yaml (event/matcher/timeout) + run.sh
└── settings/<agent>.yaml   # per-agent settings fragments (merged, never replaced)
```

Machine-local install state (content hashes per artifact × agent) lives separately in the platform data dir — the store repo stays shareable.

### Sync states

| State | Meaning |
|-------|---------|
| `in-sync` | Agent file matches the store |
| `missing` | Not installed yet |
| `drifted-local` | The agent file was edited after install |
| `drifted-store` | The store was updated after install |
| `conflict` | Both changed, or an unmanaged file is in the way |
| `n/a` | The agent does not support this artifact kind |

### Managed blocks

Instructions render into agents' global instruction files (e.g. `~/.claude/CLAUDE.md`) between sentinel markers:

```markdown
<!-- loadout:begin style -->
...your instruction content...
<!-- loadout:end style -->
```

Everything outside the markers is preserved byte-for-byte. Hooks and settings fragments deep-merge into `settings.json` instead of replacing it.

## External sources

Track skill repos (yours, your team's, public collections) and stay pinned:

```bash
loadout source add git@github.com:acme/team-skills.git
loadout check            # ls-remote only — never fetches or applies
loadout update           # shows changelog + per-skill diff, asks, applies to the STORE
loadout sync             # then rolls the store out to your agents (asks again)
```

Pins live in `sources.lock.yaml` inside the store, so a shared store reproduces the exact same skill versions everywhere. Git operations shell out to your system `git` — private repos work through your existing SSH agent or credential helper, and loadout never stores credentials. Skills you edited locally (origin `local`) are never overwritten by updates; skills that disappear upstream are kept and relabeled `local`, never deleted.

Optional passive checking (off by default) in `~/.config/loadout/config.yaml`:

```yaml
updates:
  checkOnRun: true     # after status/sync, print at most one hint line
  intervalHours: 24    # cache window; failures are always silent
```

## Supported agents

| Agent | Skills | Instructions | Commands | Hooks | Settings |
|-------|--------|--------------|----------|-------|----------|
| Claude Code | ✔ `~/.claude/skills/` | ✔ `CLAUDE.md` block | ✔ `commands/` | ✔ script + `settings.json` merge | ✔ JSON merge |
| Codex | ✔ `~/.codex/skills/` | ✔ `AGENTS.md` block | ✔ as skill | — | — (`config.toml` merge parked: comment-preserving TOML needed) |
| Gemini CLI | ✔ `~/.gemini/skills/` | ✔ `GEMINI.md` block | ✔ as skill | — | ✔ JSON merge |
| Antigravity | ✔ `~/.gemini/antigravity-cli/skills/` | ✔ shared `GEMINI.md` | project-scoped (roadmap) | — | — |
| Cursor | ✔ `~/.cursor/skills/` | project-scoped `.mdc` rules (roadmap) | ✔ as skill | — | — |

Commands render "as skill" on agents whose slash commands are backed by the Agent Skills standard. Unsupported cells show as `n/a` in `loadout status` — nothing is silently skipped.

## Roadmap

See [ROADMAP.md](ROADMAP.md) — all adapters, external skill sources, shareable bundles, a guided builder wizard (including engineering-conventions and spec-driven-development presets), TUI and an embedded web UI.

## Development

```bash
make build              # build bin/loadout
make test               # unit tests (race)
make test-integration   # end-to-end CLI tests against a temp HOME
make ci                 # tidy + lint + test + build
```

## License

MIT

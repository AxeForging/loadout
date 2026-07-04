# loadout by example

A hands-on walkthrough of the full flow. Everything below was executed for real — including the final section, where a live Claude Code session runs commands that loadout installed.

## 1. First contact

Running bare `loadout` always tells you where you are and what to do next:

```text
$ loadout
loadout — your agentic gear kit.

No store yet on this machine. Get started:
  loadout init                      create your canonical store
  loadout wizard                    guided setup (conventions, testing, SDD, handoff)
  loadout bundle import <path>      or start from a teammate's bundle
```

## 2. Build your kit with the wizard

```bash
loadout init
loadout wizard              # interactive questionnaire
# or non-interactive:
loadout wizard --preset go-cli --yes
```

The `go-cli` preset generates 14 artifacts from one command:

| Artifact | Kind | What it encodes |
|----------|------|-----------------|
| `core`, `toolchain`, `conventions`, `testing`, `sdd` | instructions | Stack, branching (GitHub flow), Conventional Commits, PR titles, linters, mandatory test levels, mocking policy |
| `commit-and-pr` | skill | Commit/branch/PR rules + validation regex + a ready `lefthook.yml` in `references/` |
| `linting`, `docker-workflows`, `sdd-workflow`, `context-handoff` | skills | Run/fix commands per linter, docker conventions, the SDD flow, the handoff convention |
| `plan`, `spec`, `handoff`, `resume` | commands | `/spec`, `/plan`, `/handoff`, `/resume` in every agent |

Add `--ai` and your logged-in agent (claude, codex, or agy) deepens `testing`/`conventions`/`toolchain` for your exact stack — previewed and confirmed before anything is replaced.

## 3. Sync — the gated ritual

```text
$ loadout status
ARTIFACT        KIND         AGENT        STATE
commit-and-pr   skill        claude-code  missing
conventions     instruction  claude-code  missing
...

$ loadout sync
Plan: 15 change(s)
  create claude-code: skills/commit-and-pr/SKILL.md  (commit-and-pr)
  update claude-code: CLAUDE.md  (conventions)
  ...
Apply 15 change(s)? [y/N]:
```

Nothing is ever written without that question (or an explicit `--yes`). Instructions land inside sentinel-marked blocks — everything you wrote yourself in `CLAUDE.md`/`AGENTS.md`/`GEMINI.md` is preserved byte-for-byte. `loadout diff` shows exact changes beforehand; `loadout doctor` verifies the whole setup.

The same gate holds in the TUI (`loadout tui` — confirm modal) and the web UI (`loadout serve` — a docked review bar; applying requires a token minted by the plan you just reviewed).

## 4. Proof: a real agent session using the installed kit

We synced the SDD + continuity artifacts into `~/.claude`, spawned a real Claude Code session (on Haiku) in an empty project, and drove it:

```text
> /spec a tiny CLI that prints the current moon phase as an emoji
```

The session used the loadout-installed `/spec` command and wrote `docs/specs/001-moon-phase-emoji.md`:

```markdown
## Problem
Users need a quick, simple way to check the current moon phase from the
command line ...

## Approach
- Build a Go CLI that reads the current system date, calculates the moon
  phase using a standard astronomical algorithm, and outputs the emoji ...

## Checklist
- [ ] Set up Go module and CLI structure following golang-cli-dev conventions
- [ ] Implement moon phase calculation algorithm ...
```

Then:

```text
> /handoff
```

produced `.agents/context/handoff.md` in the canonical cross-agent format:

```markdown
# Handoff — 2026-07-04 20:00

## Goal
Build a tiny Go CLI that reads the current system date, calculates the
lunar phase, and outputs the corresponding emoji to stdout.

## Current state
**DONE:**
- Spec drafted and committed to `docs/specs/001-moon-phase-emoji.md` ...
```

Any agent — Codex, Gemini CLI, or a fresh Claude session after a restart — can now run `/resume` and continue from that file.

### Crash-safe plans

`/plan` writes `docs/specs/NNN-<name>.plan.md` as a `- [ ]` checklist and keeps it checked-off as steps complete. The plan file, not the conversation, is the source of truth — after a crash or agent switch, `/resume` continues at the first unchecked item.

## 5. Team distribution

```bash
# Share skills from a repo (pinned; private repos use your own git credentials):
loadout source add git@github.com:acme/team-skills.git
loadout check                 # ls-remote only — never applies
loadout update                # changelog + per-skill diff, asks, updates the STORE
loadout sync                  # then rolls out to agents (asks again)

# Or ship the whole setup as one artifact:
loadout bundle export --archive          # → team-kit.loadout.tar.gz
loadout bundle import team-kit.loadout.tar.gz   # on the new machine: plan → confirm → sync
```

## 6. AI drafting with the agents you already pay for

```text
$ loadout new skill pr-readiness --ai "Verify a PR is ready for review: tests green, \
    lint clean, conventional title, no leftover debug code" --runner agy
Drafting skill "pr-readiness" with agy (uses your logged-in session)…

──── draft preview ────
---
name: pr-readiness
description: Verifies that a pull request is ready for review by checking
tests, linting, title conventions, ... and should be used before submitting
any code for review.
---
# PR Readiness Verification
...
───────────────────────
Write this skill to the store? [y/N]:
```

That output came from a real logged-in `agy` session — no API key involved. Output is validated against the Agent Skills contract before you're even asked.

## 7. Everyday loop

```bash
loadout            # where am I? anything pending?
loadout status     # the full drift matrix
loadout sync       # apply (asks first)
loadout tui        # the same, as a dashboard
loadout serve      # the same, in the browser
```

# 007 — Spec-driven development workflow: `loadout spec` + shipped SDD skills

**Status:** Draft — pending review
**Owner:** Lucas Machado
**Related:** spec 006 (first consumer of shipped presets alongside it), wizard embedded presets

## Context

The team already works spec-first (numbered specs in `docs/specs/`, checkbox plans, skills
like `/spec-af` / `/plan-af`), but the machinery is duct tape:

- The skills are copied per agent and drift; each agent re-implements "how to edit the
  checklist" in prose, so completion state gets missed or mangled.
- Checklists only live in local markdown. Teams that track work in GitHub issues have no
  option, and there is no way to see spec progress outside the editor.
- Nothing guarantees the completion loop: an agent finishes a step, and whether the checklist
  is updated depends on prompt luck.

loadout is the natural home: it already distributes skills/commands to every agent, and a
small `loadout spec` CLI gives all agents **one tested code path** to create specs, manage
checklists, and record completion — whether the spec lives in `docs/specs/` or in a GitHub
issue. Affected: this team first, then any loadout user who opts into the shipped SDD preset.

## Requirements

- **R1.** A `loadout spec` command group MUST exist with subcommands:
  `init <short-name>`, `list`, `status <spec>`, `check <spec> <step-id>`,
  `uncheck <spec> <step-id>`, `sync`. `<spec>` accepts number (`007`) or slug prefix.
- **R2.** `spec init` MUST scaffold the next free `docs/specs/NNN-<short-name>.md` from a
  template (Context / Requirements / Non-goals / Design / Data & API / Test plan / Rollout)
  and, for the github backend, create the paired issue. It MUST refuse to overwrite an
  existing spec file.
- **R3.** Checklist steps MUST have stable IDs: `- [ ] <text> <!-- sdd:<step-id> -->`.
  The parser MUST ignore checkboxes inside fenced code blocks and MUST tolerate arbitrary
  surrounding markdown (steps can live in any section, at any list depth). `check`/`uncheck`
  MUST modify only the `[ ]`/`[x]` of the targeted step — every other byte of the document is
  preserved (same discipline as safety invariant 3).
- **R4.** Two backends MUST exist behind one interface: `local` (checklist lives in the spec
  file) and `github` (checklist lives in the body of one GitHub issue per spec).
  Per-project config lives in `docs/specs/sdd.yaml`: `backend`, optional `repo`
  (auto-detected from `origin` when omitted), optional issue `labels`.
- **R5.** The github backend MUST shell out to the `gh` CLI (never a Go GitHub client), with
  `GIT_TERMINAL_PROMPT=0` discipline, and fail with an actionable message when `gh` is
  missing or unauthenticated. loadout never stores GitHub credentials.
- **R6.** `check`/`uncheck` MUST be idempotent (checking a checked step is a no-op success)
  and MUST exit non-zero with the list of valid IDs when the step ID is unknown.
- **R7.** `spec status` MUST print done/total per spec; `--json` for tooling; a
  `--require-complete` flag exits non-zero when any step is open (CI gate for "spec finished
  before merge").
- **R8.** `spec sync` (github backend only) MUST reconcile the issue against the local spec
  file: new/edited steps push to the issue; check-state differences are reported and resolved
  with an explicit direction flag (`--from-local` / `--from-github`) — never silently.
- **R9.** *(Amended during implementation — see plan.)* loadout MUST ship an embedded `sdd`
  preset (`loadout wizard --preset sdd`). The wizard **already generates** SDD artifacts
  behind `sdd.enable` (`spec` + `plan` commands, `sdd-workflow` skill, `sdd` instruction),
  so instead of introducing parallel `spec-draft`/`spec-plan`/`sdd-loop` artifacts, those
  existing generators are rewired: `/spec` scaffolds via `loadout spec init`, `/plan`
  derives checklist steps with `sdd:` IDs, and `sdd-workflow` instructs the agent to run
  `loadout spec check` immediately after completing each step and `loadout spec status`
  when resuming. Installed to every agent through the normal artifact pipeline — the
  completion loop is identical on claude-code, codex, gemini-cli and cursor.
- **R10.** All `loadout spec` subcommands operate on the **current project** (walk up to the
  git root). They MUST NOT touch the canonical store, agent homes, or the engine — specs are
  project files; the confirm-gate machinery is not involved and no new writer into agent
  homes is introduced (safety invariant 1 untouched).
- **R11.** README, completion scripts and `.structlint.yaml` updated in the same PR (dev
  rules). This repo's own `docs/specs/` MUST adopt `sdd.yaml` + step IDs as the first user
  (dogfood).

## Checklist

Tracked via `loadout spec check 007 <step-id>` (dogfood — R11):

- [x] markdown parser/mutator with fence-aware, byte-preserving SetStep <!-- sdd:parser -->
- [x] local backend: list/resolve/create/check <!-- sdd:local-backend -->
- [x] github backend via gh CLI: issue create/view/edit, repo autodetect <!-- sdd:github-backend -->
- [x] `loadout spec` command group + completions <!-- sdd:cli -->
- [x] wizard generators rewired to the CLI + `sdd` preset <!-- sdd:preset -->
- [x] unit + integration tests green (`make ci`, `make test-integration`) <!-- sdd:tests -->
- [x] README section + ROADMAP note <!-- sdd:docs -->
- [ ] PR open and CI green <!-- sdd:pr -->


## Non-goals

- Not a general issue tracker: no Jira/Linear/GitLab backends, no assignees, milestones,
  or cross-repo specs in v1.
- No automatic PR ↔ spec linking or auto-closing issues on merge.
- No web UI / TUI surface for specs in v1.
- No enforcement that code matches the spec — the CLI manages checklist state, the shipped
  skills manage agent behavior; judgment stays human.
- Not migrating existing specs 001–006 to step-ID format retroactively (new work only).

## Design

### Package layout

New `internal/sdd` package (name avoids collision with the store's future artifact kinds and
with `docs/specs` the content): `sdd.module.go` (backend interface + config),
`sdd.markdown.go` (checklist parser/mutator), `sdd.local.go`, `sdd.github.go`,
`sdd.template.go`. CLI wiring in `internal/cli/spec.go`, registered in `root.go`.

```go
type Backend interface {
    List(ctx) ([]SpecInfo, error)
    Steps(ctx, spec) ([]Step, error)              // Step{ID, Text, Done}
    SetStep(ctx, spec, stepID string, done bool) error
    Create(ctx, spec SpecInfo, body []byte) error
}
```

The markdown checklist mutator is shared by both backends — the github backend fetches the
issue body (`gh issue view --json body`), applies the same byte-preserving mutation, and
writes it back (`gh issue edit --body-file -`). One parser, two storage locations.

### Why a CLI backend instead of prompt-only skills

Considered and rejected: **skills/prompts only** (each agent re-implements checklist editing;
unparseable drift; untestable) and **hybrid** (local via prose, gh via CLI — two behaviors for
one concept, and the local path stays untested). A single Go code path is boring, testable
with the real binary, and makes the shipped skills one-liners ("run `loadout spec check …`"),
which is exactly what keeps the completion loop reliable across four different agents.

### Why `gh` and not a GitHub library

Same reasoning as the system-git rule: user credentials (gh auth, SSO, enterprise hosts)
just work, and loadout never holds a token. `gh` becomes a documented optional dependency,
required only when `backend: github`.

### Step IDs

HTML comments (`<!-- sdd:setup-db -->`) render invisibly on GitHub and in most markdown
viewers, survive reformatting better than positional matching, and make `check` immune to
step reordering/renaming. `spec init`/`spec-plan` generate slugs; collisions within a spec
are a validation error. **Alternative rejected:** matching steps by text (breaks on the
first wording tweak) or by ordinal (breaks on insertion).

### `sync` conflict stance

Checklist *structure* flows local → github (the file is authored there); check *state* can
legitimately change on either side (someone ticks a box in the issue UI). loadout refuses to
guess: state divergence is printed and the user picks a direction. Mirrors the engine's
drift-classification philosophy without involving the engine.

## Data & API changes

- New `docs/specs/sdd.yaml` (per project, committed):

  ```yaml
  backend: github        # local | github
  repo: AxeForging/loadout   # optional; auto-detected from origin
  labels: [spec]         # optional, github only
  issues:                # written by loadout: spec → issue mapping
    007-sdd-workflow: 123
  ```

- Spec markdown grammar: checklist lines gain trailing `<!-- sdd:<id> -->` markers.
- New CLI command group `spec` (+ completion scripts).
- New embedded preset `sdd` under `internal/wizard/presets/`.
- No manifest, store, engine, adapter, or web API changes. No migrations.

## Test plan

Unit tests colocated; integration tests build and drive the real binary (team convention),
`gh` stubbed with a fake binary on `PATH` recording invocations (external boundary — the only
thing mocked).

- **R3 (unit, the load-bearing one):** table tests for the parser/mutator — step in nested
  list; checkbox inside a fenced code block (must be ignored); `[x]`→`[x]` idempotent
  re-check; unknown ID; duplicate ID rejected; document byte-identical outside the toggled
  character; CRLF input preserved.
- **R1/R2 (integration):** `spec init` in a temp git repo creates `docs/specs/001-…` with
  template sections; second `init` picks `002`; `init` onto an existing name refuses,
  exit non-zero.
- **R4/R5 (integration):** `backend: github` + stub `gh` — `init` calls `issue create` with
  labels; missing stub (`PATH` without gh) yields the actionable error; repo auto-detected
  from the temp repo's origin remote.
- **R6/R7 (integration):** `check` twice → same file hash, exit 0; unknown ID lists valid
  IDs, exit non-zero; `status --require-complete` exits non-zero with one open step, zero
  when all checked; `--json` output shape asserted.
- **R8 (integration):** diverged state (issue body checked, local unchecked via stubbed
  `gh issue view`) → `sync` reports and exits non-zero without flags; `--from-github`
  updates the local file only.
- **R9 (integration):** `wizard --preset sdd` installs the three artifacts into a fake
  claude-code home through the normal confirm-gated pipeline; skill bodies reference
  `loadout spec check` (guards against the preset drifting from the CLI surface).
- **R10 (unit):** `internal/sdd` imports neither `engine` nor `adapter` (structlint rule).

## Rollout

1. Order: parser + local backend → CLI group → github backend (stub-tested) → `sdd` preset →
   dogfood (`docs/specs/sdd.yaml` in this repo, this spec's own plan gets step IDs).
2. Additive only — no flag needed; nothing changes for users who never run `loadout spec`.
3. README section + completion scripts + `.structlint.yaml` in the implementation PRs.
4. Rollback: delete the command group and preset; `sdd.yaml` and step-ID comments in spec
   files are inert markdown, harmless to leave behind.
5. Follow-ups (ROADMAP): additional backends, PR linking, spec progress in web UI/TUI.

# 009 — `loadout rm`: artifact removal with clean uninstall

**Status:** Approved (review waived) — implemented
**Owner:** Lucas Machado
**Related:** spec 003 (loadouts switch introduced `PlanRemoval`/`ApplySwitch`), spec 004 (sync safety: snapshots, conflict semantics), spec 006 (MCP configs — deferred uninstall), ROADMAP "Open questions" item on MCP uninstall

## Context

The store has no delete verb. `internal/store` exposes `AddArtifact` but no removal, so
dropping a hand-made artifact means editing `manifest.yaml` by hand — the single most
surprising gap in the CLI for a manager tool. A failed bulk import or an experiment can't be
cleanly undone.

Half the machinery already exists: `loadouts switch` ships `engine.PlanRemoval` +
`ApplySwitch`, which correctly delete `MergeReplace` targets and strip `MergeBlock` managed
blocks. But `MergeJSON` targets fall through to a warning
(`internal/engine/removal.go:202`): removing an MCP artifact leaves the merged entry in
`~/.claude.json` / `.mcp.json`, and removing a hook leaves its entry in `settings.json`
forever. This is the parked ROADMAP item ("json-merge removal is still unsupported") and it
bites both MCP and hooks.

Affected: every user who adopts, imports or generates an artifact and later wants it gone —
which is everyone, eventually. Also a prerequisite for the spec-kit adoption story (bulk
adopt is only safe when a bad import can be reversed).

## Requirements

- **R1.** A `loadout rm <artifact>...` command MUST exist accepting one or more artifact
  IDs. Unknown IDs fail with a non-zero exit and a pointer to `loadout status`. Flags:
  `--dry-run`, `--yes`, `--force`, `--keep-store`.
- **R2.** Default behavior is **full removal** in one confirmation-gated pass: uninstall
  from every agent home where state records an install (files deleted, blocks stripped,
  JSON keys removed) **and** delete the artifact from the store (manifest entry + content
  files). `--keep-store` uninstalls from agent homes but keeps the store artifact.
- **R3.** The engine MUST support **keyed JSON removal**: for `MergeJSON` targets,
  `PlanRemoval` produces an `OpUpdate` item whose desired content is the current file with
  exactly the fragment's contribution removed — fragment object leaf keys deleted, array
  items removed only when deep-equal to a fragment item, parent objects pruned when the
  removal empties them. Bytes outside the removed keys are preserved byte-for-byte
  (surgical `sjson` delete, same discipline as `jsonMerge`). A semantic no-op (nothing of
  the fragment present) produces no write. The shared file itself is NEVER deleted, even if
  removal empties it.
- **R4.** **Drift safety**: when the on-disk value no longer matches what loadout installed
  — a `MergeJSON` value at a fragment path differing from the fragment, a `MergeReplace`
  file whose hash differs from the state record, a `MergeBlock` block whose content hash
  differs — the target is skipped with a warning naming the path and reason. `--force`
  removes it anyway. *(Amended during implementation.)* A drift-skipped target KEEPS its
  install record (the state group shrinks to the retained subset instead of vanishing) —
  clearing it would make a later `rm --force` treat the file as never-installed and
  refuse. Fully removed targets clear their records as normal. (Mirrors sync's
  conflict/`--force` semantics.)
- **R5.** Removing an artifact with `origin: source` is allowed, but the plan output MUST
  warn: next `loadout update` of that source will re-add it, and `loadout source remove`
  drops the whole source. No exclusion mechanism in this spec (non-goal).
- **R6.** **Ordering & crash safety**: agent-home uninstall happens first (while the
  artifact still renders, so `PlanRemoval` can compute merge strategies), store deletion
  last, state saved once per phase. A crash mid-way leaves the artifact in the store and
  re-running `loadout rm` is safe (plans re-diff from disk + state, all steps idempotent).
- **R7.** Removing a hook artifact MUST remove both its script file
  (`hooks/loadout-<id>.sh`) and its `settings.json` hook entries (via R3 array removal).
  Removing an MCP artifact MUST remove its server entry from the user- or project-scope
  config per its `scope`. Closes the ROADMAP MCP-uninstall item for claude-code, cursor
  and gemini-cli.
- **R8.** All four safety invariants hold: only the engine writes; `rm` writes nothing
  without a confirm callback returning true (covered by the permanent invariant suite); a
  pre-write snapshot with trigger `rm` captures every file about to be modified or deleted;
  content outside managed keys/blocks is untouched.
- **R9.** The pre-write snapshot SHOULD also capture the artifact's **store content**
  (manifest entry + content files) so `loadout restore` can resurrect a regretted `rm`
  even in a store that isn't git-tracked. If store capture is infeasible in the snapshot
  format, `rm` MUST print where the store lives and recommend git — never silently
  unrecoverable.
- **R10.** UX: the plan output lists per-agent actions, warnings and skips before the
  prompt; success prints artifact/agent counts and the next step (`loadout status`).
  README section, shell completions and `.structlint.yaml` updated in the same PR.

## Non-goals

- Per-agent uninstall (`rm --agent X` / a separate `loadout uninstall` verb) — full
  removal or `--keep-store` only; per-agent granularity stays with `loadouts switch`.
- Source exclusion lists (rm'd artifact re-appearing on `update` is warned, not prevented).
- Codex `config.toml` removal — codex has no MCP/settings support yet (parked separately).
- `loadout undo` / `restore --last` ergonomics — next spec.
- Web UI / TUI removal surfaces — CLI first; the web `DELETE /api/artifacts/<id>` route
  can reuse the same engine path in a follow-up.
- Removing profiles / library entries / loadouts (they have their own `rm` verbs already).

## Design

### Inverse merge (`jsonRemove`)

Mirror of `applyFragment` in `internal/engine/merge.go`, walking the fragment against the
current bytes:

- **Object keys**: recurse when both sides are objects; at leaf paths, compare the current
  value to the fragment value. Equal → `sjson.DeleteBytes` at that path. Different →
  drift: record a warning (or delete under force). After deletions, prune now-empty
  ancestor objects that the fragment introduced (`mcpServers: {}` left behind is noise,
  but never prune the root document).
- **Arrays**: inverse of the union in `setLeaf` — remove elements deep-equal to fragment
  items; user-added or user-edited elements are untouched (an edited element simply no
  longer deep-equals, which *is* the drift case: warn, force-remove removes nothing here
  because we cannot identify the edited element — documented behavior).
- **Scalars set wholesale** (fragment set the whole subtree on install): deep-equal →
  delete subtree; different → drift.

Returns `(newBytes, changed, warnings)`; `changed == false` keeps existing bytes so user
formatting never causes phantom writes (same guarantee as `jsonMerge`).

### Engine changes

`PlanRemoval` gains an options struct (`Force bool`) and fills the `MergeJSON` case with
`jsonRemove`, emitting `OpUpdate` items like the `MergeBlock` case does today. The
`MergeReplace`/`MergeBlock` cases gain the R4 drift check (hash vs state record).
`loadouts switch` inherits the drift-skip behavior — a deliberate improvement (today
switch deletes drifted files without asking); its `--force` flag threads through.

A new `Engine.ApplyRemoval(ctx, plan, confirm)` wraps the existing write pass (same shape
as `ApplySwitch` with a nil install plan) so `rm` doesn't fake an empty install plan.

### Store changes

`Store.RemoveArtifact(id)`: drop the manifest entry, save, then delete the artifact's
content path (`skills/<id>/`, `commands/<id>.md`, …). Refuses unknown IDs. Pure store
concern — no engine or adapter knowledge.

### CLI

`internal/cli/rm.go` with `rmCmd()`, registered in `root.go`. Sequence: resolve IDs →
`PlanRemoval` → print plan + warnings (incl. R5 source-origin warning) → confirm →
`ApplyRemoval` → unless `--keep-store`, `RemoveArtifact` per ID → summary.

### Alternatives considered

- **Track installed JSON keys in `state.yaml`** (key list per target instead of re-render):
  rejected — schema migration for existing users, and redundant while the artifact is
  still in the store (render is deterministic). The one case it would help — artifact
  already gone from the manifest — already has a defined warning fallback in `PlanRemoval`.
- **Remove the fragment's top-level keys wholesale, ignoring drift**: rejected — deletes
  user edits inside the subtree (e.g. a hand-added `"disabled": true` on an MCP entry),
  violating invariant 3 and the team's newcomer-safety bar.
- **Sentinel comments in JSON files**: rejected — comments are invalid JSON; agents parse
  these files.

## Data & API changes

- No manifest, config, or `state.yaml` schema changes.
- Snapshot meta: entries MAY gain a `root: store` variant for R9 store-content capture
  (backward-compatible addition; old snapshots restore unchanged).
- New: `Store.RemoveArtifact`, `engine.ApplyRemoval`, `jsonRemove` in merge.go,
  `internal/cli/rm.go`.
- No HTTP API changes (web parity is a non-goal).

## Test plan

Per team policy: TDD for the ROADMAP:104 bug class (write the failing MCP/hook-leak test
first), behavior-named tests, integration tests drive the **built binary** against a temp
HOME.

- **R3 (unit, `merge_test.go` style tables)**: removing an object leaf preserves sibling
  bytes exactly; array item removed only on deep-equality; user-added array elements
  survive; emptied `mcpServers` pruned; root never pruned; fragment absent → no-op, bytes
  identical; invalid existing JSON → loud error.
- **R4 (unit, `removal_test.go`)**: drifted JSON value / edited replace-file / edited
  block → skipped with warning, state cleared; same plan with `Force` → removed; snapshot
  taken before the forced write.
- **R1/R2/R6/R7 (integration)**: build the binary; init store; add a skill + hook + MCP
  artifact; sync into a fake claude-code home; `loadout rm` each: files gone, `settings.json`
  hook entry and `.claude.json` server entry removed while user-authored sibling keys keep
  their exact bytes; manifest entry and store content gone; state has no record;
  `--keep-store` leaves the store entry; second `rm` of the same ID errors; `rm` with no
  confirmation (pipe `n`) writes nothing (invariant suite extension).
- **R5 (integration)**: source-origin artifact rm prints the re-add warning; subsequent
  `update` re-adds it (documents the behavior).
- **R8 (invariant suite)**: extend the permanent no-confirm-no-write test to cover `rm`.
- **R9 (integration)**: `rm` then `loadout restore <id>` brings the agent-home files back;
  store content restored (or, if capture was descoped, the printed guidance is asserted).
- **R10**: README/completions/structlint checked by existing CI gates.

## Rollout

- No feature flags, no migrations. Ships in one PR: `feat: loadout rm — artifact removal
  with clean uninstall (json-merge removal)`.
- Behavior change to note in the PR body: `loadouts switch` now skips drifted targets
  instead of deleting them (strictly safer; `--force` restores old behavior).
- ROADMAP: mark the MCP-uninstall open question closed for claude-code/cursor/gemini-cli;
  codex TOML remains parked.
- Rollback: revert the PR — no persisted formats change, old binaries read the same
  state/store.

## Checklist

Tracked via `loadout spec check 009 <step-id>`:

- [x] failing TDD tests reproducing the MCP + hook JSON-leak (ROADMAP:104) <!-- sdd:tdd-repro -->
- [x] `jsonRemove` inverse-merge with drift detection + pruning <!-- sdd:json-remove -->
- [x] `PlanRemoval` MergeJSON case + drift checks + `Force` option <!-- sdd:plan-removal -->
- [x] `engine.ApplyRemoval` (confirm-gated, snapshot trigger `rm`) <!-- sdd:apply-removal -->
- [x] store-content capture in snapshots (R9) or explicit descope note <!-- sdd:store-snapshot -->
- [x] `Store.RemoveArtifact` <!-- sdd:store-remove -->
- [x] `loadout rm` CLI + source-origin warning + completions <!-- sdd:cli -->
- [x] invariant suite extended to `rm` <!-- sdd:invariant -->
- [x] integration scenarios (full removal, keep-store, force, restore) <!-- sdd:integration -->
- [x] README + ROADMAP + structlint updated <!-- sdd:docs -->

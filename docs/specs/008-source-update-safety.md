# 008 — Source update safety: three-way classification, store snapshots, atomic writes

## Context

Spec 004 made **sync** (store → agent homes) safe: conflicts are blocked by default, every overwrite is snapshotted, and `classify()` distinguishes "the store moved" from "the user moved" from "both moved". The **store side never inherited any of that.** `loadout update` (internal/source/update.go) compares store content only against the *new* upstream commit — a two-way diff. It cannot tell "upstream changed this skill" from "the user edited this skill locally", so both are listed as `~ update skill X` and `ApplyUpdate` clobbers them with `os.RemoveAll` + copy. Concretely: a user imports a skill from a source, tweaks its SKILL.md in the store, and any upstream commit — even one touching a *different* skill — causes `loadout update` (or `update --yes` in a script, with no prompt at all) to silently revert their edit. There is no backup and no undo, because store mutations are never snapshotted.

Three adjacent gaps compound this:

- `PlanUpdate`'s "owned by the user or another source" branch is a silent no-op — the plan never mentions those skills, so users can't see that a source *has* a skill they own under the same name.
- Store metadata writes (`sources.lock.yaml`, `manifest.yaml`, engine state) are plain `os.WriteFile` — a crash mid-write corrupts the file — and CLI, TUI and `loadout serve` can all mutate the store concurrently with no coordination.
- The web UI can *check* for source updates (`POST /api/sources/check`) but cannot plan or apply them, and the TUI never surfaces the update-available count the CLI hint system already caches.

Who is affected: anyone who customizes an imported skill (the moment loadout's own docs encourage "adopt then adapt", this is everyone), anyone scripting `update --yes`, and web/TUI-first users who today must drop to the CLI to apply source updates.

## Requirements

### Three-way classification

1. **MUST** record a content hash (`sourceHash`, existing `store.Hash` format) on every source-origin artifact in `manifest.yaml`, stamped at import (`source.Add`) and refreshed whenever an update writes that artifact (`ApplyUpdate`). Local-origin artifacts carry no `sourceHash`.
2. **MUST** classify each source-owned skill in `PlanUpdate` by comparing three hashes — baseline (`sourceHash`), store content, new upstream content — into exactly one of: `identical` (store == upstream), `upstream-changed` (baseline == store ≠ upstream), `locally-edited` (baseline ≠ store, baseline == upstream), `conflict` (all three differ).
3. **MUST** backfill a missing `sourceHash` (stores created before this spec) during `PlanUpdate` by reconstructing the skill's content at the pinned commit from the full clone (`git archive <pin>`); if the pin is unreachable (force-pushed away), **MUST** fall back to legacy behavior for that skill (treat as `upstream-changed`) and say so in the plan output.
4. **MUST NOT** write `locally-edited` skills during `ApplyUpdate`: they keep their origin, keep their original `sourceHash` (so detection persists), and appear in the plan as informational lines.
5. **MUST** skip `conflict` skills by default. A new `loadout update --force-discard` flag opts in per-run; `--yes` alone **MUST NOT** discard local edits. When `--force-discard` is set, the plan output names each skill whose local edits will be discarded before the confirmation.
6. **MUST** list skills the source provides but that exist locally with a different origin as explicit plan lines ("exists locally with origin X — not touched") instead of today's silent skip. Advisory only, never blocks.
7. **MUST** expose the local-only half of the classification (`ok` / `edited` / `legacy` for missing hash) as a state column in `loadout source list`, computed offline from `sourceHash` vs store content — no network.

### Upstream diffing

8. **MUST** add `loadout diff <skill-id> --upstream`: a unified diff (existing go-difflib) of the store copy against the skill in the source's clone at the remote ref, fetching the clone if the cache is missing. Errors clearly when the artifact has no source origin.

### Store write reliability

9. **MUST** write `sources.lock.yaml`, `manifest.yaml`, and engine state (`state.yaml`) atomically (temp file in the same directory + `os.Rename`) via one shared helper.
10. **MUST** snapshot the store files an `ApplyUpdate` or bundle `Apply` (import with overwrite) is about to overwrite or delete — artifact trees plus `manifest.yaml`/`sources.lock.yaml` — into the existing machine-local backups dir (`<data>/loadout/backups/<id>/`, triggers `source-update` / `bundle-import`), before the first write, reusing the spec-004 snapshot machinery so `loadout restore` lists and restores them with the same confirm gate. Fail closed: no snapshot, no write. Pure additions (nothing overwritten) produce no snapshot.
11. **MUST** guard store mutations with a machine-local advisory lock (`<data>/loadout/store.lock`, `O_CREATE|O_EXCL` with PID + stale detection) so concurrent CLI/TUI/serve writers fail fast with a clear "store is locked by pid N" error instead of interleaving writes. Read paths take no lock.

### Web and TUI parity

12. **MUST** add `POST /api/sources/update/plan` (returns per-skill classification + a one-shot plan token) and `POST /api/sources/update/apply` (consumes the token, optional `forceDiscard`), mirroring the existing two-phase sync gate; apply without a valid token is rejected. The SPA sources page gains the corresponding update flow (plan modal with classification, apply button, discard requiring an explicit checkbox).
13. **MUST** show the cached update-available count (the CLI hint cache) on the TUI dashboard as a read-only hint line; the TUI gains no apply affordance.

### Skill aliasing

15. **MUST** support importing a source skill under a different local name (alias): `loadout source add --alias <upstream>=<local>` (repeatable) at registration, and `loadout source import <source> <upstream> --as <local>` for a skill previously skipped due to a name collision. The artifact records its real upstream name (`sourceName` in the manifest, empty when identical to the ID), and update classification, `diff --upstream` and removal-relabeling all resolve through it — an aliased skill still tracks the REAL upstream skill's changes.
16. **MUST** keep collision behavior safe: an alias targeting an existing artifact ID errors; a skill skipped at `source add` time is reported with a hint to import it under an alias.

### Documentation

14. **MUST** update README in the same PR: `update` classification vocabulary and `--force-discard`, `diff --upstream`, `source list` state column, store snapshots in the `restore` section, the lock error, and the new web endpoints — with a worked example of the locally-edited → conflict → force-discard flow. Completion scripts updated for new flags.

## Non-goals

- Non-git sources (tarballs, raw URLs) — sources stay git-only; bundles already cover hand-me-a-file sharing.
- A `source detach <skill>` command (explicit forking) — decided against for now; `locally-edited` keeps tracking upstream indefinitely.
- Interactive per-skill conflict resolution during `update` — the unit is the run (`--force-discard`), not the skill.
- TUI apply flow for source updates (hint only, requirement 13).
- Merging upstream and local edits (three-way *content* merge) — classification only; the diff command is the resolution aid.
- Locking reads or remote checks — only mutations take the store lock.

## Design

### Approach

**Classification** lives in `PlanUpdate`, which already walks discovered skills against the store. `UpdatePlan` grows `LocallyEdited []SkillDir`, `Conflicts []SkillDir`, `OwnedElsewhere []string`, and `LegacyBaseline []string`; `Changed` keeps meaning "safe to write". The decision table is a pure function `classifyUpdate(baseline, store, upstream string) updateState` with exhaustive unit tests — the engine's `classify` pattern transplanted. `ApplyUpdate` gains a `ForceDiscard bool`; it writes `Changed` always, `Conflicts` only when forced, stamps `sourceHash` on everything it writes, and never touches `LocallyEdited`.

**Hash stamping**: `store.AdoptFrom` computes the content hash it already has the bytes for and records it when the origin is a source; `Artifact` gains `SourceHash string yaml:"sourceHash,omitempty"`. goccy/go-yaml ignores unknown fields, so older loadout binaries read newer manifests untouched, and older manifests simply have empty hashes → the backfill path (req 3): reconstruct the pinned content with `git archive <pin> -- <subdir>` into a temp dir (the update clone is full, so the pin is normally reachable), hash it, proceed three-way.

**Atomic writes + lock**: `store.WriteFileAtomic(path, data, perm)` (temp + fsync + rename in the same dir) replaces the three metadata `os.WriteFile` call sites. `store.Lock(dataDir)` returns an unlock func; taken by the CLI/web/TUI entry points that mutate the store (update apply, bundle import apply, source add/remove, wizard writes). Stale detection: if the lock file's PID is dead, steal it.

**Store snapshots**: reuse spec-004's snapshot format. A small adapter builds `SnapshotEntry` values from store paths (agent id `store`) for the artifact trees and metadata files an apply will touch, then the existing `takeSnapshot`/`ApplyRestore`/prune machinery does the rest — `loadout restore` needs only trigger-label awareness, no new subcommand.

**Web**: the plan-token pattern is copied from `handleSyncPlan`/`handleSyncApply` (one-shot token map, same expiry). The plan response serializes the classification lists verbatim so the SPA renders the same vocabulary as the CLI. `make build-web`, commit dist.

### Alternatives considered

- **Hashes in `sources.lock.yaml` (per-source artifact map) instead of the manifest.** Rejected: two files must then agree about the same artifact, and the manifest entry is already the single record of an artifact's identity/origin. One optional field beats a parallel registry.
- **Git-only three-way (no stored hash, always `git archive <pin>`).** Rejected as the primary mechanism: the clone cache is machine-local and disposable — on a second machine sharing the store there is no cache, so local-edit detection would need network. The stored hash travels with the store and works offline; git reconstruction remains as backfill.
- **Auto-relabel `locally-edited` skills to `origin: local` (treat edit as fork).** Rejected: silent detachment surprises users and permanently stops update tracking for what may be a one-line tweak. Reporting keeps both options open.
- **Interactive per-skill prompts for conflicts.** Rejected for v1: skip-by-default plus an explicit run-level `--force-discard` is the same asymmetry spec 004 chose (refusing to write loses nothing); per-skill interaction can layer on later without changing the model.
- **flock/gofrs for locking.** Rejected: `O_EXCL` + stale-PID is dependency-free and sufficient for "two processes on one machine" — we are not protecting against NFS or byzantine failures.
- **A new snapshot format for store backups.** Rejected: `restore` should be one mental model. The 004 machinery is path-agnostic; only entry construction differs.

## Data & API changes

### `internal/domain`

```go
type Artifact struct { /* … */ SourceHash string `yaml:"sourceHash,omitempty" json:"sourceHash,omitempty"` }
```

### `internal/source` (update.go)

```go
type UpdatePlan struct {
    /* existing: Ref, NewCommit, Log, Added, Changed, RemovedIDs, UpToDate */
    LocallyEdited  []SkillDir // baseline ≠ store, upstream unchanged — never written
    Conflicts      []SkillDir // all three differ — written only with ForceDiscard
    OwnedElsewhere []string   // upstream skill name exists locally with another origin
    LegacyBaseline []string   // sourceHash missing and pin unreachable — legacy two-way for these
}
func (m *Manager) ApplyUpdate(p *UpdatePlan, forceDiscard bool) error
func classifyUpdate(baseline, store, upstream string) updateState // pure
```

### `internal/store`

```go
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error
func Lock(dataDir string) (unlock func(), err error) // store.lock, O_EXCL + stale PID
```

### CLI

- `loadout update [--yes] [--force-discard]` — new flag; plan output gains `!` (locally edited), `!!` (conflict), `.` (owned elsewhere) lines.
- `loadout diff <skill-id> --upstream` — new flag on the existing diff command.
- `loadout source list` — new STATE column (`ok`/`edited`/`legacy`).
- `loadout restore` — lists store-trigger snapshots alongside sync snapshots (label only).

### Web

- `POST /api/sources/update/plan` → `{plans: [{source, classification lists, log}], token}`
- `POST /api/sources/update/apply` `{token, forceDiscard}` → applied summary; invalid/reused token → 400.

### Migration

`sourceHash` is optional; no store-format version bump. Old stores backfill lazily on first `update` per requirement 3. Rollback = revert the PR; stray `sourceHash` fields are ignored by older binaries.

## Test plan

Per requirement (unit unless noted; integration = build tag `integration`, real binary, temp HOME, local git fixture repos as in `source_test.go`):

1. `AdoptFrom` from a source records `sourceHash`; local adopt records none. `ApplyUpdate` re-stamps written skills.
2. Table test for `classifyUpdate` covering all hash permutations (the four states, exhaustive).
3. Backfill: fixture store without hashes + reachable pin → correct three-way; force-pushed pin → skill lands in `LegacyBaseline` and plan says so.
4. **The regression test this spec exists for (integration):** import skill, edit it locally, push an unrelated upstream commit, run `loadout update --yes` — the edit survives, plan shows `locally edited`, exit 0.
5. Conflict skill + `--yes` → skipped, content untouched; `--force-discard --yes` → overwritten, `sourceHash` re-stamped; plan named the skill before confirm (golden output).
6. Upstream adds a skill whose name the user owns locally → plan contains the `owned elsewhere` line; store untouched (integration).
7. `source list` state column: pristine → `ok`, edited → `edited`, hashless → `legacy` — offline (no network in test).
8. `diff --upstream`: golden unified diff against a fixture clone; artifact without source origin → clear error; missing cache → clone then diff (integration).
9. Kill-during-write simulation: `WriteFileAtomic` leaves either old or new content, never partial (write to a full/failing target in unit test); all three metadata writers go through it (grep-level structural test or direct unit coverage).
10. Update apply with overwrites produces a snapshot restorable by `loadout restore` (integration, round-trip content equality); pure-addition update produces no snapshot; snapshot write failure aborts before any store write.
11. Second process (or second lock call) while locked → immediate "locked by pid" error; stale lock (dead PID) is stolen; unlock releases.
12. Web handler tests: plan token round-trip, token reuse → 400, apply without token → 400, `forceDiscard:false` skips conflicts / `true` writes them (extends `server_test.go` fixtures).
13. TUI model test: hint cache present → dashboard line rendered; absent → no line.
14. README sections exist for every new flag/command (checked in review, not automated); completion scripts include `--force-discard` / `--upstream`.

All new tests are behavior-named; the binary-driven integration tests satisfy the "test the built bin" convention.

## Rollout

Single PR (`feat!:` — `update --yes` no longer discards local edits, an intentional behavior break). No feature flags: the safe behavior *is* the feature. Order inside the PR: domain field + hash stamping → classification → apply gating → atomic writes/lock/snapshots → CLI surface → web/TUI → docs. Rollback is a clean revert; data written by the new version (`sourceHash`, snapshots) is inert to older binaries.

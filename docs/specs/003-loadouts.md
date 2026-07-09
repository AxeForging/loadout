# 003 — Loadouts: named, switchable artifact-set snapshots

## Context

The store's manifest is a flat list of artifacts; every sync installs whatever the manifest and target filters say, and nothing is ever removed — `engine.Apply` (`internal/engine/engine.go`) only ever creates or updates files. There is no way to say "install exactly this named subset of artifacts, and remove whatever was installed before that isn't in it." Users who want different artifact sets for different contexts (e.g. a lean personal setup vs. a full team kit) have no way to switch between them without manually tracking and deleting files by hand.

This spec adds **loadouts**: named, persisted snapshots of "the active set of artifacts." `loadout loadouts switch <name>` installs the new set's artifacts *and* removes artifacts that were previously installed but aren't in the new set — a real switch, not an additive-only sync.

This is the first feature to give loadout a delete capability over files in agent homes (`~/.claude`, etc.), which the tool has never had. The design here is deliberately conservative: it reuses the existing confirm-gate and state-tracking machinery as-is, and scopes out the one case (JSON-merged `settings.json` fragments) where safe removal isn't provably solvable with the data the tool currently keeps.

Who is affected: users who want more than one named artifact configuration and want to switch between them without hand-editing agent homes.

## Requirements

1. **MUST** add a `loadouts` package/CLI command group persisting named artifact-ID lists at `<store>/loadouts/<name>.yaml` — store-shareable like wizard profiles, distinct from them (profiles are wizard *answers*; loadouts are *artifact sets*).
2. **MUST** auto-create a special, reserved loadout named `default` the first time the feature is used (first `loadouts save` or first `loadouts switch`), capturing every artifact ID currently in the store manifest at that moment as a frozen snapshot — a safety-net rollback point the user never has to think to create.
3. **MUST** reject any attempt to save over or remove the `default` loadout, enforced by a reserved-name check (not solely a data flag a user could hand-edit away).
4. **MUST** let a user freely save, list, show, and remove any non-`default` ("custom") loadout.
5. **MUST** make `loadouts switch <name>` a real switch: install every artifact in the target loadout that's still in the store manifest, **and** remove every artifact that's currently installed (per `state.yaml`) but not in the target loadout.
6. **MUST** gate `switch`'s writes behind the exact same confirm-then-apply invariant every other write path in the tool already has (`engine.Apply`'s `confirm func(*Plan) bool` contract) — nothing is removed or installed without an explicit yes.
7. **MUST** treat removal of `MergeReplace` targets (whole files/directories that are 100% loadout-owned: `commands/<id>.md`, `hooks/loadout-<id>.sh`, `skills/<id>/`) as real deletion (`os.Remove`/`os.RemoveAll`), guarded by checking the target path against the adapter's `Owned()` allowlist before any delete.
8. **MUST** treat removal of `MergeBlock` targets (instructions in CLAUDE.md/AGENTS.md/etc.) as stripping the sentinel-marked block from the shared file, never deleting the file itself, and never leaving a double blank line behind.
9. **MUST NOT** attempt to remove `MergeJSON` targets (hooks/settings fragments merged into `settings.json`) — v1 leaves the file untouched and only drops the `state.yaml` tracking record, surfacing the artifact as "installed but unmanaged" via `status`/`doctor` going forward.
10. **MUST** clear `state.yaml` records for every artifact×agent pair being removed, in the same write pass as the install half, so a crash mid-switch can't leave `state.yaml` reflecting only half the operation.
11. **MUST NOT** guess a merge strategy for a `state.yaml` record whose artifact ID no longer resolves in the current manifest, or whose recorded relative path no longer appears in a fresh render — warn and leave the file untouched instead.
12. **MUST** force-remove a `MergeBlock` target regardless of whether its current content has drifted from what loadout would render (matching `UpsertBlock`'s existing unconditional-overwrite behavior on install) — but **MUST NOT** touch a structurally broken block (missing end marker); warn instead.
13. **MUST** make `loadouts rm <name>` delete only the loadout's saved definition, never installed artifacts or `state.yaml` records — real removal only ever happens through `switch`.
14. **SHOULD** treat switching to the already-active loadout as a no-op that still prints a clear message and touches no files.

## Non-goals

- Safe removal of `MergeJSON` (hooks/settings) targets — tracked as a future spec; would require per-key/per-array-element provenance tracking this release doesn't add.
- Per-artifact target/agent overrides within a loadout (the schema has a `Targets` field reserved for this; v1 ignores it and always uses `Manifest.Targeted()`).
- Web UI or TUI surfaces for loadouts — CLI-first for this release; a two-phase `/api/loadouts/switch/plan`+`/apply` pair reusing the existing token pattern is a natural fast-follow once the CLI path is battle-tested.
- CLI commands for editing a loadout's artifact list in place (`save` always writes a fresh full list; there's no `add`/`remove` sub-editing in v1).
- Migrating existing installs into a loadout automatically beyond the one-time `default` baseline snapshot.

## Design

### Approach

A loadout is `{name, protected, createdAt, artifacts: [{id, targets}]}`, persisted at `<store>/loadouts/<name>.yaml`, package `internal/loadouts` mirroring `internal/wizard/profiles.go`'s exact CRUD shape and store-I/O pattern. `EnsureDefault` lazily bootstraps the reserved `default` loadout from the current manifest the first time `save` or `switch` runs; `list`/`show` stay side-effect-free.

Switching diffs two artifact-ID sets: the target loadout's IDs (filtered to ones still resolvable in the manifest) versus `Engine.InstalledArtifactIDs()` — the real set derived from `state.yaml` install records, not a remembered "which loadout is active" name, so a switch is always self-healing even if the machine drifted since the last one. The "install" half reuses `Engine.Plan`/`Item`/`merged()` exactly as `sync` does. The "removal" half is a new `Engine.PlanRemoval` that reads `state.yaml`'s recorded relative paths per artifact×agent and dispatches by merge strategy: `MergeReplace` → queue a delete (skill artifacts collapse to one directory-delete item, not one per file); `MergeBlock` → strip the block via a new `adapter.RemoveBlock`, reusing `UpsertBlock`'s exact marker-scanning approach; `MergeJSON` → warn, clear state, leave the file. Both halves apply through a single new `Engine.ApplySwitch`, which writes everything to agent homes first (mirroring `Apply`'s own crash-safety shape) then saves `state.yaml` once, gated by one confirm call covering both plans.

### Alternatives considered

- **Attempt a real JSON un-merge for `MergeJSON` targets.** Rejected for v1: `internal/engine/merge.go`'s `jsonMerge`/`deepMerge` have no per-key/per-array-element provenance, and `state.yaml` only hashes the whole rendered fragment per file — not per JSON-pointer. Two different artifact kinds (hooks and settings) can write into the same `settings.json` additively, so subtracting exactly one artifact's contribution without a provenance model this release doesn't have risks silently corrupting or deleting unrelated hand-edited content. The failure mode of "stays installed longer than intended" (skip-only) is annoying but fully recoverable; the failure mode of a buggy JSON subtraction is destructive and hard to recover — an asymmetric risk not worth taking for the first delete-capable release.
- **Store the loadout's `Protected` flag as the sole deletion guard.** Rejected: a flag in a YAML file a user can hand-edit is not a real guard. The reserved name `default` is checked directly in `SaveLoadout`/`RemoveLoadout` before any file is touched; the flag is kept only for display/tooling clarity.
- **Compute the "previously active set" from a remembered loadout name (e.g. "you were on `frontend`, diff against its saved list") instead of `state.yaml`.** Rejected: this would be wrong the moment the machine drifted from that saved list (a manual `sync`, a crash, a hand edit) — leaving artifacts installed with no loadout ever knowing about them, or removing artifacts a plain `sync` had legitimately added since. Diffing against real `state.yaml` install records is always correct relative to what's actually on disk, matching the tool's existing drift-detection philosophy.
- **Have `loadouts rm <name>` also uninstall everything that loadout had installed.** Rejected: this hides a scary combined "delete recipe + everything it installed" operation behind a small, easy-to-fumble command with no plan/review step. Real removal only ever happens through `switch`, which always shows a plan and asks first — `rm` stays a pure, always-safe, always-reversible bookkeeping operation, consistent with `profile rm`'s existing "definitions only" semantics.
- **Ship web UI in the same PR.** Rejected for v1 given this is the first delete-capable feature; CLI-first lets it get battle-tested via real usage before a second, more casually-clickable surface is added. The existing two-phase token pattern (`internal/web/server.go`) is a straightforward fast-follow once the CLI path is proven.

## Data & API changes

### `internal/loadouts` package (new)

```go
type ArtifactRef struct {
    ID      string   `yaml:"id"`
    Targets []string `yaml:"targets,omitempty"` // reserved; v1 ignores it
}
type Loadout struct {
    Name      string        `yaml:"name"`
    Protected bool          `yaml:"protected,omitempty"`
    CreatedAt time.Time     `yaml:"createdAt"`
    Artifacts []ArtifactRef `yaml:"artifacts"`
}
const DefaultName = "default"

func SaveLoadout(s *store.Store, name string, artifactIDs []string) (Loadout, error) // rejects DefaultName
func LoadLoadout(s *store.Store, name string) (Loadout, error)
func ListLoadouts(s *store.Store) ([]string, error)
func RemoveLoadout(s *store.Store, name string) error // rejects DefaultName
func EnsureDefault(s *store.Store) error
func (l Loadout) Summary() string
```
Plus a machine-local, cosmetic-only active-loadout marker (`config.DataDir()/active-loadout`) for `list`/`show` display — never used to compute what `switch` removes.

### `internal/engine` additions

```go
const OpDelete Op = "delete"     // Item.IsDir picks os.Remove vs os.RemoveAll
// Item gains: IsDir bool

func (e *Engine) InstalledArtifactIDs() ([]string, error)
func (e *Engine) PlanRemoval(ctx context.Context, artifactIDs []string) (*RemovalPlan, error)
func (e *Engine) ApplySwitch(ctx context.Context, install *Plan, removal *RemovalPlan, confirm func(*Plan, *RemovalPlan) bool) (installed, removed int, err error)

type RemovalPlan struct {
    Items      []Item
    ClearState []StateKey
    Warnings   []RemovalWarning
    Skips      []Skip
}
type StateKey struct{ Artifact string; Agent domain.AgentID }
type RemovalWarning struct{ Artifact string; Agent domain.AgentID; RelPath string; Reason string }
```

### `internal/adapter` additions

```go
func RemoveBlock(doc []byte, id string) (result []byte, found bool, err error)
```

### CLI

```
loadout loadouts save   <name> [artifact...]   # no artifacts = snapshot everything in the manifest; rejects "default"
loadout loadouts list                           # annotates the active one
loadout loadouts show   <name>
loadout loadouts switch <name> [--dry-run] [--yes]
loadout loadouts rm     <name>                  # rejects "default"; definition only
```

## Test plan

| Requirement | Verification |
|---|---|
| 1, 2, 3, 4 | Unit tests in `internal/loadouts/loadouts_test.go`: save/list/round-trip, empty-list-when-no-dir, `EnsureDefault` creates once from the current manifest and is idempotent, reject-`default`-name on both save and remove, remove-unknown errors, frozen-snapshot (later manifest changes don't retroactively join `default`). |
| 5, 6, 10 | Unit tests in `internal/engine/removal_test.go` and `internal/engine/state_test.go`: `PlanRemoval` per merge-strategy branch, `ApplySwitch` writes nothing without confirmation (the permanent invariant test), never calls confirm on a true no-op, applies install+removal in one `state.yaml` save, converges correctly on a rerun after a simulated partial write. |
| 7 | Unit test: a fake adapter whose `Owned()` doesn't cover a target path causes `PlanRemoval` to warn and refuse the delete rather than removing it. |
| 8, 12 | Unit tests in `internal/adapter/block_test.go`: `RemoveBlock` preserves surrounding content byte-for-byte, leaves no double blank line, is a no-op on a missing block, errors on an unterminated block, empties a file that was only the block, leaves other blocks in the same file untouched. Integration test: a hand-edited (drifted) block is still force-removed on switch; a structurally broken block (no end marker) is left untouched with a warning. |
| 9 | Unit test: a `MergeJSON` target in `PlanRemoval` produces a warning and a `ClearState` entry with zero `Items` — the file is never touched. |
| 11 | Unit test: an artifact ID no longer in the manifest, or a recorded relPath no longer produced by a fresh render, produces a warning instead of a guessed removal; integration test confirms the CLI doesn't crash and prints a clear line. |
| 13 | Integration test: `rm` on a loadout whose artifacts are currently installed leaves those files and their `state.yaml` records untouched. |
| 14 | Integration test: switching to the already-active loadout prints a "nothing to do" message and touches no files. |
| End-to-end | `integration/loadouts_test.go`: full CRUD happy path; a real two-artifact switch that installs one set and removes the other from disk with `state.yaml` updated accordingly; a round trip back to `default` restoring the original baseline; agent-not-installed behaves like plain `sync`'s skip; `default` can't be saved over or removed. |

## Rollout

- No feature flag: additive package, additive CLI command group, additive `engine`/`adapter` API surface — `Engine.Apply` itself is untouched, so every existing sync/wizard/bundle call site keeps working byte-for-byte.
- CLI-first, as noted above; web UI is an explicit fast-follow, not part of this PR.
- README gets a new "Loadouts" section documenting the five commands, `default`'s protection, and the auto-baseline behavior.
- Rollback: revert the PR — no destructive migration; the only new persisted state is `<store>/loadouts/*.yaml` (unused if rolled back) and the cosmetic active-loadout marker in the platform data dir.

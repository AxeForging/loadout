# 004 — Sync safety: conflict blocking and pre-write snapshots

## Context

loadout's whole reason to exist is writing config into agent homes (`~/.claude`, …), and until now `engine.Apply` (`internal/engine/engine.go`) wrote every planned change unconditionally: for skills, commands and hook scripts it does a whole-file `MergeReplace` overwrite. `classify()` already knew how to label a target that exists on disk but was never installed by loadout as `conflict`, but that classification was display-only (`status`/TUI) — the apply path never consulted it. So a newcomer who imported someone else's bundle (a `plan` command, a `handoff` skill) and ran `loadout sync` would have their **own** pre-existing `~/.claude/commands/plan.md` or `~/.claude/skills/plan/` silently overwritten, with no warning and no way to get the original back. There has never been any backup, snapshot, or undo.

This is the failure the tool most needs to not have. Bundle *import* is already safe (it writes only to the store, gated, skip-on-conflict); the danger is entirely at **sync** time, which is where this spec adds two guards:

1. **Block unmanaged overwrites by default.** A whole-file target that already exists but loadout can't prove it owns is a conflict — skipped unless the user explicitly opts in per-run (`--force` / a web checkbox).
2. **Snapshot before writing.** Right before any overwrite or delete, copy the originals into a timestamped, machine-local snapshot, and add `loadout restore` to recover them.

Who is affected: anyone adopting loadout onto a machine that already has hand-made agent config — especially first-time users importing a shared bundle.

## Requirements

1. **MUST** treat a `MergeReplace` target (skill/command/hook-script file) that exists on disk but has no matching install record — or whose store and disk have both moved since install — as a **conflict**, reusing the existing `domain.StateConflict` classification rather than inventing a second taxonomy.
2. **MUST** skip conflicts by default: `Plan.Changes()` excludes them, so `Apply` does not write them and the existing confirm gate is unchanged.
3. **MUST** provide a per-run, non-persisted opt-in (`Plan.OverwriteConflicts`, set from a `--force` flag / web `overwriteConflicts` field) that promotes conflicts into `Changes()` so they are written.
4. **MUST NOT** record install state for a skipped conflict — otherwise the next plan would see it as a loadout-installed `drifted-local` file and silently overwrite it on the second sync. This is the single most important correctness invariant of the feature.
5. *(Superseded by spec 010, issue #23: local edits to text targets are now held back by default.)* **MUST NOT** block a file loadout provably installed that has since drifted locally (`drifted-local`): re-converging it to the store is the tool's core contract, and blocking it would train users to always pass `--force`. `MergeBlock` (managed-block) and `MergeJSON` (settings) targets are never blocked — they are non-destructive by construction.
6. **MUST** surface skipped/overwritten conflicts distinctly from ordinary changes in all three frontends (CLI plan output, web sync dock, TUI), never as a plain `update`.
7. **MUST** take a snapshot of the original content of every file an `Apply` or `ApplySwitch` is about to overwrite or delete, **before the first write**, storing it under the machine-local data dir (`<data>/loadout/backups/<id>/`), never inside the shareable store.
8. **MUST** capture whole directories for directory deletes (a skill's `skills/<id>/`), and skip pure creations (nothing on disk to preserve) so a first-time install with no collisions produces no snapshot.
9. **MUST** fail closed: if the snapshot can't be written, the apply aborts before touching anything (a safety feature that fails silently is worse than an aborted run).
10. **MUST** add `loadout restore` to list snapshots and restore one, gated by the exact same confirm-callback invariant as `Apply` (nil/false confirm writes nothing).
11. **MUST** make a restore itself snapshotted (so it's undoable) and **MUST NOT** modify `state.yaml` — after a restore, `status`/`doctor` honestly report whatever drift results and the user decides.
12. **MUST** cap retained snapshots (config `backups.keep`, default 10; negative keeps all) and prune the oldest beyond the cap.
13. **SHOULD** keep the TUI read-only with respect to forcing in v1 (log that conflicts were skipped and how to force from the CLI), rather than adding a TUI overwrite affordance.

## Non-goals

- A TUI `--force` affordance — CLI/web only for v1 (requirement 13).
- Restoring files an apply *created* (a snapshot only preserves what existed before; a create has nothing to preserve). Snapshot metadata could record creates for a future "undo an install" — out of scope here.
- Compressing/deduplicating snapshot storage, or a retention policy richer than "keep newest N".
- Blocking `MergeJSON`/`MergeBlock` writes — they are non-destructive and out of scope.

## Design

### Approach

Conflict detection lives entirely in `buildItem`: after constructing an `Item`, set `it.Conflict = op != OpNoop && merge == MergeReplace && classify(it) == StateConflict`. `Plan` gains an `OverwriteConflicts bool`; `Changes()` skips `Conflict && !OverwriteConflicts` items, a new `Conflicts()` returns them for display, and a new `stateItems()` (used by both `Apply` and `ApplySwitch` for state recording) excludes skipped conflicts. Because every write path already funnels through `Changes()` and every state write through the group builder, this one classification propagates correctly to CLI, TUI and web with no per-surface logic — each just gains conflict *display* and, for CLI/web, a `--force` / checkbox that flips `OverwriteConflicts`.

Snapshots live in a new `internal/engine/snapshot.go`. `takeSnapshot(backupDir, trigger, items, now)` copies the current on-disk content of every `OpUpdate`/`OpDelete` item (whole tree for directory deletes) into `<data>/loadout/backups/<UTC-timestamp>/files/<agent>/<relPath>`, plus a `snapshot.yaml` manifest; it returns `(nil,nil)` when nothing was worth preserving. `Apply`/`ApplySwitch` call it after the confirm gate and before the first write, fail closed on error, then `pruneSnapshots`. `Engine.ApplyRestore(snap, confirm)` re-uses the same confirm invariant, snapshots the current state first (trigger `restore`), writes the captured bytes back, and never touches `state.yaml`. A clock is injected on `Engine` for deterministic snapshot IDs in tests. `loadout restore` lists (newest-first, with relative time and trigger) or restores by ID.

### Alternatives considered

- **Also block `drifted-local` files (loadout-installed, then hand-edited).** Rejected: these were provably installed by loadout, so re-converging them is the core contract; blocking them makes every routine sync on a machine where agents rewrite their own files demand `--force`, training users to always pass it — which destroys the protection exactly where it matters. Recovery for those overwrites is instead guaranteed by the snapshot, which is taken for *every* overwrite, not just conflicts.
- **Warn-and-overwrite in the same run (with an extra ack) instead of skip-by-default.** Rejected: skip-by-default is the safe failure mode. A user who wants the overwrite types `--force` once; a user who didn't loses nothing. The asymmetry favors refusing to write.
- **`.bak` sibling files next to each overwritten file.** Rejected: litters agent homes, only keeps one generation, and pollutes the very directories the tool manages. A single machine-local snapshot tree keeps homes clean, versions naturally by timestamp, and is trivially prunable.
- **Snapshot lazily / warn-and-continue on snapshot failure.** Rejected: fail-closed. A backup that might not exist is not a backup; aborting the apply is recoverable, a missing snapshot after a clobber is not.
- **Record state for skipped conflicts anyway (simpler code).** Rejected outright: it silently reintroduces the exact clobber this feature prevents on the *second* sync (the file would classify as `drifted-local` next time). `stateItems()` exists solely to prevent this, and a dedicated regression test guards it.
- **Have restore rewrite `state.yaml` to match the restored content.** Rejected: a restore is a user escape hatch, not a sync; leaving state untouched means `status` tells the truth (the restored file now differs from the store) and the user consciously chooses to re-sync or keep it.

## Data & API changes

### `internal/engine` (engine.go)

```go
type Item struct { /* … */ Conflict bool }        // set in buildItem
type Plan struct { /* … */ OverwriteConflicts bool }
func (p *Plan) Changes() []Item      // now skips unresolved conflicts
func (p *Plan) Conflicts() []Item    // new: the blocked items, for display
func (p *Plan) stateItems() []Item   // new: Changes ∪ noops minus skipped conflicts

type Engine struct { /* … */ BackupDir string; KeepBackups int; Clock func() time.Time }
```

### `internal/engine/snapshot.go` (new)

```go
type SnapshotEntry struct { Agent domain.AgentID; RelPath, AbsPath string; Op Op; Mode fs.FileMode; IsDir bool }
type Snapshot struct { ID string; CreatedAt time.Time; Trigger string; Entries []SnapshotEntry }

func takeSnapshot(backupDir, trigger string, items []Item, now time.Time) (*Snapshot, error)
func pruneSnapshots(backupDir string, keep int) error
func ListSnapshots(backupDir string) ([]Snapshot, error)
func LoadSnapshot(backupDir, id string) (*Snapshot, error)
func (e *Engine) ApplyRestore(ctx, snap *Snapshot, confirm func(*Snapshot) bool) (int, error)
```

### `internal/config/config.go`

- `Config.Backups.Keep int` (`backups.keep`), `Config.BackupsKeep() int` (default `DefaultBackupsKeep = 10`), `BackupsDir() (string, error)` = `DataDir()/backups`.

### CLI

- `--force` on `sync`, `install`, `loadouts switch` → sets `Plan.OverwriteConflicts`.
- `loadout restore [id]` (`internal/cli/restore.go`), registered in `root.go`; completion scripts updated.
- `printChanges` renders conflicts as `! skip …` / `! force …` lines.

### Web API

- `POST /api/sync/plan` response gains `conflicts: []changeJSON`; `GET /api/overview` gains `conflicts: int`.
- `POST /api/sync/apply` accepts `overwriteConflicts bool`.
- SPA: `SyncDock.tsx` lists conflicts, offers an "also overwrite N …" checkbox, and surfaces a conflict-only state; `api.ts` types updated.

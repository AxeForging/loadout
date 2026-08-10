# Plan — 009 `loadout rm`: artifact removal with clean uninstall

Derived from [009-artifact-removal.md](009-artifact-removal.md). Checked off as work lands;
this file, not the conversation, carries progress.

## Approach

Fill the `MergeJSON` hole in the existing removal machinery (`engine.PlanRemoval` /
`ApplySwitch`, shipped with loadouts switch) with an inverse of `jsonMerge`, add the R4
drift gate to all three merge strategies, then wrap it in a new `ApplyRemoval` + store
delete + `loadout rm` command. TDD: the ROADMAP:104 leak (MCP + hook JSON entries left
behind) gets failing engine tests before any implementation.

Key semantics decided during design (see spec Design):

- `jsonRemove` walks the fragment against current bytes: deep-equal subtree → delete key;
  both objects → recurse (+ prune emptied objects, never the root); arrays remove
  deep-equal items only (edited items are unidentifiable — drift warning, `--force`
  cannot help there); value differs → drift warning, `--force` deletes the subtree.
  User-added sibling keys always survive (invariant 3); shared files are never deleted.
- Drift checks compare disk against the **install-time state hash** (not the current
  fragment) for replace/block targets, so a store-side update never blocks removal.
- `PlanRemoval(ctx, ids, RemovalOptions{Force, RemoveStore})`; `RemoveStore` adds the
  manifest + artifact content files to the plan (`StoreFiles`) so the pre-write snapshot
  captures them (R9) — restore can resurrect a regretted rm even without git.
- Order: `ApplyRemoval` (confirm → snapshot incl. store files → agent writes → state
  save) then `Store.RemoveArtifact` (manifest first, then content). Crash between the
  two leaves the artifact in the store; re-running `rm` converges (R6).

## Checklist

- [x] failing engine tests: hook + MCP JSON entries must be removed (currently warn/leak) <!-- sdd:tdd-repro -->
- [x] `jsonRemove` in engine/merge.go + unit tables (byte preservation, arrays, pruning, drift, no-op) <!-- sdd:json-remove -->
- [x] `PlanRemoval` options struct, MergeJSON case, drift checks for replace/block/json, StoreFiles <!-- sdd:plan-removal -->
- [x] `engine.ApplyRemoval` — confirm gate, snapshot trigger `rm`, single state save <!-- sdd:apply-removal -->
- [x] snapshot capture of store manifest + content (pseudo-agent `store`), restore round-trip <!-- sdd:store-snapshot -->
- [x] `Store.RemoveArtifact` (manifest entry + content, path-safe) + unit tests <!-- sdd:store-remove -->
- [x] `internal/cli/rm.go` + root.go registration + completion scripts + source-origin warning <!-- sdd:cli -->
- [x] invariant suite: rm writes nothing on nil/declined confirm <!-- sdd:invariant -->
- [x] integration: full removal, keep-store, force/drift, restore-after-rm, unknown ID <!-- sdd:integration -->
- [x] README rm section, ROADMAP:104 closure note, `make ci` + `make test-integration` green <!-- sdd:docs -->

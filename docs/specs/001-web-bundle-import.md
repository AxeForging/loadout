# 001 — Web UI bundle import

## Context

`loadout bundle export` lets a user package their whole store (skills, instructions, commands, hooks, settings) into a single `.loadout.tar.gz` for a teammate. The reverse direction, `loadout bundle import <path>`, already exists as a CLI command: it validates the bundle, diffs it against the local store (`internal/bundle.PlanImport`), prints the plan, prompts for confirmation, and merges it in (`internal/bundle.Apply`).

The web UI (`internal/web`, `loadout serve`) already has the export half — `GET /api/bundle/export` streams a `.loadout.tar.gz`, linked from the "share" section of the Dashboard page. It has no import half. A user who receives a bundle file today must drop to the CLI to import it, even though everything else in the web UI (sync, sources, wizard/profiles) is a full parity surface with the CLI.

This spec adds the import half to the web UI: upload a `.loadout.tar.gz`, review a plan (new / conflicting / identical artifacts, new sources), choose skip-vs-overwrite for conflicts, confirm, and apply — using the same `internal/bundle` core the CLI already uses, so there is exactly one implementation of import semantics.

Who is affected: users running `loadout serve` who want to receive a teammate's shared bundle without switching to a terminal.

Explicitly out of scope for this change: TUI bundle import/export (currently absent entirely — tracked as a separate follow-up), and any notion of an "active profile" (profiles are pre-existing wizard answer-set templates, unrelated to bundles; not touched here).

## Requirements

1. **MUST** expose a way to upload a `.loadout.tar.gz` file from the web UI and receive back a merge plan without any write to the store happening yet.
2. **MUST** reject uploads that are not valid gzip/tar or not a loadout bundle (missing/mismatched `bundle.yaml` `apiVersion`) with a clear 4xx error, matching the validation `bundle.PlanImport` already performs.
3. **MUST** reject uploads larger than 50MB with a 4xx error before reading the full body into memory or disk.
4. **MUST** display, per plan: bundle name/creator/created-at, count and IDs of new artifacts, conflicting artifacts (same ID, different content), identical artifacts (no-op), and new source refs — mirroring the CLI's printed plan.
5. **MUST** let the user choose a single global conflict policy (skip or overwrite) before applying, matching the CLI's `--on-conflict skip|overwrite` semantics (`bundle.Apply(store, plan, overwrite bool)`); default to skip.
6. **MUST NOT** write anything to the store until the user explicitly confirms the plan (two-phase plan → apply, same pattern as `/api/sync/plan` + `/api/sync/apply`).
7. **MUST** make the plan→apply token single-use and expire idle plans (10 minutes, matching the existing sync-plan eviction policy), releasing the plan's temp extraction directory (`ImportPlan.Close()`) on apply, on expiry, and on explicit discard/navigation-away.
8. **MUST** report, after apply, which artifacts were actually written and how many new sources were merged, then prompt the user to run a sync (same as the CLI's "Run 'loadout sync' to install into your agents" hint) since import only touches the store, not agent homes.
9. **SHOULD** show upload progress/pending state and a clear error state if the upload or plan step fails (e.g. wrong file type, corrupt archive).
10. **MUST** reuse `internal/bundle.PlanImport` / `internal/bundle.Apply` as-is — no new import/merge logic duplicated in `internal/web`.

## Non-goals

- Directory-tree upload (`webkitdirectory`) or any bundle format other than `.loadout.tar.gz` for the web path. Directory bundles remain CLI-only (`loadout bundle import ./dir`).
- Per-artifact conflict resolution (checkbox per conflicting artifact). v1 matches the CLI's single global skip/overwrite toggle.
- TUI bundle export/import (tracked separately; TUI currently has no bundle support at all).
- Any "active profile" / switchable-configuration concept — unrelated to this feature.
- Authentication/authorization on the upload endpoint beyond what the rest of the localhost-only API already relies on (loopback bind).

## Design

### Approach

Add two new routes mirroring the existing sync two-phase gate exactly:

- `POST /api/bundle/import/plan` — accepts a `multipart/form-data` upload (field `bundle`), writes it to a temp file, calls `bundle.PlanImport(store, tempFilePath)`, stashes the resulting `*bundle.ImportPlan` in the same kind of token-keyed pending map already used for sync plans, and returns a JSON view of the plan.
- `POST /api/bundle/import/apply` — accepts `{token, overwrite}`, pops the pending plan, calls `bundle.Apply(store, plan, overwrite)`, closes the plan (releasing its temp extraction dir), and returns what was applied.

No changes to `internal/bundle` are needed — `PlanImport` already accepts a plain file path (directory or `.tar.gz`, detected via `os.Stat`/gzip probe in `openBundle`), so writing the uploaded bytes to a temp `*.tar.gz` file and pointing `PlanImport` at it is sufficient.

### Alternatives considered

- **Pass a server-side filesystem path instead of uploading bytes.** Rejected: the web UI is meant to be usable from a browser tab, and the browser has no way to hand the Go process an arbitrary local path — only the bytes of a file the user picks via `<input type="file">`. A path-based API would only work if the bundle happened to already sit on the same machine in a location the user can type out, which defeats "click to import."
- **Raw request body upload (`Content-Type: application/gzip`, no multipart) instead of `multipart/form-data`.** Considered simpler (no multipart parsing), but multipart is what an `<input type="file">` + `FormData` naturally produces in the browser without extra client-side plumbing, and leaves room for future fields (e.g. an conflict-policy field) in the same request if ever needed. Multipart chosen for standard fit with browser file inputs.
- **Extend `bundle.Apply`'s signature to take per-artifact decisions.** Rejected for v1 per the global-toggle requirement above — keeps `internal/bundle` unchanged and the CLI/web behavior identical.
- **Reuse the *existing* `s.plans map[string]pendingPlan` (typed for `*engine.Plan`) for import plans too.** Rejected: import plans (`*bundle.ImportPlan`) need `Close()` called to release a temp directory, which `engine.Plan` doesn't have. A second map (`s.bundlePlans map[string]pendingBundlePlan`) with its own mutex-guarded eviction (also calling `Close()` on evicted entries) keeps the two concerns (and their cleanup rules) separate and correct, at the cost of a small amount of duplicated bookkeeping code.

### Frontend

- New `api.ts` methods: `bundleImportPlan(file: File)` (builds `FormData`, POSTs multipart — the one endpoint that can't use the existing JSON `request<T>()` helper as-is, since it must not set `Content-Type: application/json`) and `bundleImportApply(token: string, overwrite: boolean)`.
- New component modeled on `SyncDock.tsx`'s plan → review → apply flow, added to the Dashboard "share" section alongside the existing export link: a file picker, an upload/plan button, a review panel (new/conflicts/identical/sources counts and IDs, skip/overwrite toggle), and a confirm button. Reuses the same visual/interaction language (token display, "Requisition" framing) already established by `SyncDock`.
- After a successful apply, invalidate the same queries `SyncDock` invalidates (`queryClient.invalidateQueries()`) so overview/artifacts refresh, and surface the "run a sync" hint.

## Data & API changes

### `POST /api/bundle/import/plan`

Request: `multipart/form-data` with a single file field `bundle`.

Response `200`:
```json
{
  "token": "…",
  "meta": { "name": "…", "createdBy": "…", "createdAt": "2026-…", "artifacts": 12 },
  "new": [{ "id": "…", "kind": "…" }],
  "conflicts": [{ "id": "…", "kind": "…" }],
  "identical": ["…"],
  "sources": [{ "name": "…", "url": "…" }]
}
```

Errors: `400` (no file / file too large / not gzip / not a loadout bundle / apiVersion mismatch), `413` if over 50MB.

### `POST /api/bundle/import/apply`

Request:
```json
{ "token": "…", "overwrite": false }
```

Response `200`:
```json
{ "applied": ["artifact-id", "…"], "sourcesAdded": 1 }
```

Errors: `400` (missing token), `409` (unknown/expired token).

### Server-side additions

- `internal/web/server.go`: `s.bundlePlans map[string]pendingBundlePlan` + mutex (or reuse `s.mu`), `pendingBundlePlan{plan *bundle.ImportPlan, created time.Time}`, eviction loop mirroring the sync-plan one but calling `.Close()` on eviction.
- `handleBundleImportPlan`: `r.ParseMultipartForm(50 << 20)`, read `bundle` file part, write to `os.CreateTemp("", "loadout-upload-*.tar.gz")`, call `bundle.PlanImport`, stash, respond.
- `handleBundleImportApply`: same token pop/404/409 pattern as `handleSyncApply`, call `bundle.Apply`, `plan.Close()`, respond, and clean up the temp upload file the plan pointed at.
- Register both routes in `routes()` next to the existing `GET /api/bundle/export`.

## Test plan

| Requirement | Verification |
|---|---|
| 1, 6 | Unit test: POST a valid bundle to `/api/bundle/import/plan`, assert `200` with a token and correct new/conflicts/identical/sources counts; assert nothing changed in the store's manifest after this call alone. |
| 2 | Unit tests: POST a non-gzip file, a gzip-but-not-tar file, and a tar.gz missing `bundle.yaml` / with a mismatched `apiVersion` → each asserts `400` with a descriptive error. |
| 3 | Unit test: POST a file over the 50MB cap → asserts rejection (`400`/`413`) without the handler attempting to fully buffer it. |
| 4, 5 | Unit test against a fixture bundle with a mix of new/conflicting/identical artifacts and one new source ref → assert the plan JSON matches exactly (IDs, counts, meta fields). |
| 6, 7 | Unit test: call plan, then apply with a *different* (bogus) token → `409`; call apply twice with the same valid token → second call `409` (single-use); construct a plan, don't apply, advance time > 10 min (inject clock or reuse existing sync-plan eviction test pattern) → subsequent apply `409` and temp dir removed. |
| 8 | Unit test: apply with `overwrite=false` → conflicting artifacts unchanged, new artifacts present, sources merged; apply with `overwrite=true` → conflicting artifacts overwritten. Assert response reports applied IDs and `sourcesAdded` count. |
| 9 | Component test (Testing Library) for the new upload component: renders idle state, shows pending/loading state during plan mutation, shows an error state when the plan call rejects. |
| 10 | Code review invariant, not a runtime test: `internal/web` handler diff must show zero new merge/diff logic — only calls into `bundle.PlanImport`/`bundle.Apply`. |
| Integration | New `integration/` scenario (build tag `integration`, temp HOME + fake store): export a bundle from store A via the CLI, POST it through the real HTTP server for store B's `loadout serve`, plan → apply via the API, then assert store B's manifest/files reflect the import — full parity check against the existing `bundle import` CLI integration coverage. |

## Rollout

- No feature flag: this is additive (two new routes, one new UI section); nothing existing changes behavior.
- Ship order: backend routes + unit tests first (independently mergeable and testable via `curl`/httptest), then frontend component + `api.ts` methods, then the integration scenario tying both together. All three can land in one PR since the feature isn't usable end-to-end until all parts exist, but should be reviewed as logically separate commits.
- `make build-web` must be run and the refreshed `internal/web/dist` committed in the same PR (per repo convention for any `web/` change).
- README: add the web-import step to the existing bundle export/import section.
- Rollback: revert the PR — no persisted state format changes, no migration involved (temp upload files and pending-plan tokens are in-memory/ephemeral only).

# 002 — Structured custom instructions with a reusable library

## Context

The wizard (`internal/wizard`, driven by CLI `loadout wizard` and the web Builder) turns a questionnaire into canonical instruction artifacts. Every free-text question today (`TypeText`) renders as a single-line input, and multi-value free text is captured via a comma-separated `.other` companion field split at generation time (`Answers.EffectiveList`, `internal/wizard/answers.go:63-83`).

There is no field for open-ended "house rules" style custom instructions. Users who want to add several distinct instructions (e.g. "always write tests before fixing a bug", "no coverage theater", "edge cases required") are forced to cram them into one small text box, typically as a comma- or manually bullet-prefixed blob. Because the existing render path treats each comma-split segment as a bullet without stripping any bullet characters the user typed themselves, output is visibly mangled: `- - Always ensure tests that are real, - edge cases, - nagative...`.

This spec adds:
1. A new wizard question type, `list`, that captures an ordered set of free-text entries.
2. A `standards.custom` question of this type, rendered generated Markdown as clean one-bullet-per-entry.
3. A store-level, git-shareable **library** of previously-typed instructions that can be browsed and re-added to any future wizard run, so a user doesn't retype the same house rules for every profile/project.

Who is affected: anyone running the wizard (CLI or web) who wants project-specific custom instructions beyond the built-in questionnaire sections.

## Requirements

1. **MUST** add a new `QuestionType` (`list`) whose answers are an ordered `[]string`, alongside the existing `select`/`multi`/`text`/`bool` types, in `internal/wizard/schema.go` — the schema remains the single source of truth for CLI and web.
2. **MUST** add a `standards.custom` question of type `list` to the existing `standards` section. `standards.refs` is unchanged.
3. **MUST NOT** reject previously-valid answers files, wizard profiles, or presets that don't set `standards.custom` — the field is optional and defaults to an empty list.
4. **MUST** coerce a hand-edited YAML answers file that sets `standards.custom` to a plain string (newline-separated) rather than a list, instead of rejecting it — API/programmatic payloads should still send `[]string`.
5. **MUST** render each list entry as exactly one Markdown bullet (`- <entry>`) in the generated instructions, stripping any bullet character (`-`, `*`, `•`) and surrounding whitespace the user typed themselves, so re-typed bullets never double up.
6. **MUST** emit the custom-instructions content as its own artifact (`custom-instructions`, kind instruction) only when at least one non-empty entry exists after cleaning; emit nothing when the list is empty.
7. **MUST** provide a persistent, store-level library of instruction strings: list, add (dedup by normalized text), and remove, scoped to the whole store (not per-profile), persisted as YAML under the store so it is shareable via git like wizard profiles.
8. **MUST** expose the library over the web API (list/add/delete) without the two-phase confirm-token gate used for agent-home writes — library entries live in the store only, matching the existing ungated `/api/profiles` write path.
9. **MUST** let the web Builder capture list answers by typing an entry and pressing Enter to add it as a row, with a per-row delete control.
10. **MUST** let the web Builder save any row to the library, and add any library entry into the current list with one click; already-added entries must be visually distinguished from ones not yet added.
11. **SHOULD** support the `list` question type in the interactive CLI wizard via a multiline text field (one instruction per line), so CLI-only users aren't blocked, even though library browsing remains web-only.

## Non-goals

- CLI commands for library management (`loadout library ...`) — CLI users manage the library file directly or via the web UI.
- Editing an existing library entry in place (only add/remove).
- Per-agent or per-profile libraries — the library is a single store-wide list.
- Migrating `standards.refs` to the new list type or changing its existing comma-separated semantics.
- Any notion of an "active artifact set" / switchable profile (tracked separately as a follow-up, unrelated to this feature).

## Design

### Approach

`standards.custom` is answered as `[]string`, one instruction per entry, order preserved. `Answers.validate()` gains a `TypeList` case that accepts `[]string`/`[]any`-of-strings as-is and coerces a bare string by splitting on newlines (covers hand-written YAML); non-string list members are rejected with a descriptive error, consistent with how `TypeMulti`/`TypeSelect` already reject out-of-schema values.

At generation time, a new builder `customInstruction(a Answers) string` in `internal/wizard/content.go` cleans each entry (`cleanItem`: trim + strip one leading bullet marker) and renders a small Markdown doc, one clean bullet per non-empty entry. `internal/wizard/generate.go` appends a conditional `custom-instructions` artifact spec only when the rendered content is non-empty — mirroring how other conditional sections already work in that file.

The library is a new file, `internal/wizard/library.go`, following the exact shape of `internal/wizard/profiles.go` (same package, same `*store.Store` receiver, same YAML-under-store persistence pattern) but storing a single file `library/instructions.yaml` (not one file per entry, since entries are short strings, not named documents). Each entry's ID is derived from `sha256(strings.TrimSpace(text))[:12]`, so adding the same text twice is a no-op that returns the existing entry (content-addressed dedup, no separate uniqueness check needed).

The web API adds three endpoints mirroring the existing profile endpoints exactly (list/add/delete, JSON body, no token gate — store writes, not agent-home writes). The Builder gets a new `InstructionListEditor` component wired into the existing `Question` renderer as a new `type === "list"` branch; it reuses the existing `.field` (text input) and `.chip` (toggleable pill) CSS patterns already used for wizard options and profile selection, so no new visual language is introduced.

### Alternatives considered

- **Reuse `TypeText` + comma-splitting for `standards.custom`, just fix the bullet-stripping bug.** Rejected: comma-splitting breaks the moment an instruction itself contains a comma (very common in natural-language rules, e.g. "write tests, not documentation, for behavior changes"). A first-class ordered list avoids this ambiguity entirely and matches the actual UX being requested (row-based, not comma-based).
- **One file per library entry under `library/<id>.yaml`, mirroring `profiles/<name>.yaml` one-per-file.** Rejected: profiles are named, user-chosen documents; library entries are anonymous short strings the user never names. A single `library/instructions.yaml` list avoids inventing throwaway filenames for one-liners and keeps listing/loading O(1) file reads instead of a directory scan.
- **Store the library inside `loadout.yaml` (the manifest) as a new top-level field.** Rejected: the manifest is artifact-focused (`domain.Manifest.Artifacts`); mixing in an unrelated instruction-library list would blur `internal/domain`'s "pure shared types" boundary and force every manifest reader/writer to understand a field they don't use. A separate file under the store keeps concerns isolated, same as `profiles/`.
- **Gate library writes behind the two-phase plan→apply token like agent-home syncs.** Rejected: the token gate exists specifically because agent-home writes touch files outside the store and need a confirm step before mutating a user's `~/.claude` etc. Library entries only ever touch the store repo, which is already the pattern for `/api/profiles` (ungated) — consistent precedent, and an unnecessary confirm step for a two-field "save this sentence" action would be poor UX.

### Frontend

- `web/src/api.ts`: widen `WizardQuestion["type"]` to include `"list"`; add `LibraryEntry` type (`{id, text, createdAt}`) and `api.libraryInstructions()`, `api.libraryAdd(text)`, `api.libraryRemove(id)`.
- New `web/src/components/InstructionListEditor.tsx`: controlled `{ value: string[]; onChange(v: string[]): void }` component.
  - A `.field` input where Enter adds the trimmed, non-empty, non-duplicate draft text as a new row and clears the draft.
  - Each row shows its text, a delete control (removes from `value`), and a save-to-library control (calls `api.libraryAdd`, and afterwards renders as already-saved once the library query reflects it).
  - Below the rows, saved library entries render as `.chip` buttons (`data-on` when their text is already present in `value`); clicking a chip not yet in `value` appends it, clicking one already present is a no-op for the list (a separate small control removes it from the library entirely).
  - React Query key `["library"]`; add/remove mutations invalidate it.
- `web/src/pages/Builder.tsx`: `Question` renderer gets a new branch for `q.type === "list"` rendering `<InstructionListEditor value={...} onChange={...} />`. No changes needed to profile save/load or `decodeAnswers` — both already pass arbitrary JSON values (including arrays) through untouched.

## Data & API changes

### Schema

```go
const TypeList QuestionType = "list" // ordered free-text entries, answered as []string
```

New question (`internal/wizard/schema.go`, `standards` section):
```go
{ID: "standards.custom", Prompt: "Custom instructions — one per entry", Type: TypeList, Default: []string{}}
```

### `GET /api/library/instructions`

Response `200`: `[{ "id": "…", "text": "…", "createdAt": "2026-…" }, …]` (empty array, never `null`, when the library file is absent).

### `POST /api/library/instructions`

Request: `{ "text": "…" }`. Response `200`: the created (or existing, if a dedup hit) `LibraryEntry`. Errors: `400` on empty/whitespace-only text.

### `DELETE /api/library/instructions/{id}`

Response `200`: `{ "removed": true }`. Errors: `404` on unknown id.

### Server-side additions

- `internal/wizard/library.go`: `LibraryEntry`, `ListLibrary`, `AddToLibrary`, `RemoveFromLibrary` — same package and store-receiver pattern as `profiles.go`.
- `internal/web/server.go`: three routes registered next to the existing `/api/profiles` routes; three handlers (`handleLibraryList`, `handleLibraryAdd`, `handleLibraryDelete`) next to the existing profile handlers, following their exact error-handling shape.

## Test plan

| Requirement | Verification |
|---|---|
| 1, 4 | Unit test (`answers_test.go`): `TestValidate_ListQuestion` — `[]string` and `[]any`-of-strings accepted as-is; a multiline plain string is coerced to a split `[]string`; a non-string list member (`42`) is rejected with an error. |
| 2, 3 | Unit test: an answers file / profile YAML without `standards.custom` still validates and `Answers.List("standards.custom")` returns nil/empty — no regression for existing profiles/presets. |
| 5, 6 | Unit tests (`content_test.go`): `TestCustomInstruction_SanitizesUserBullets` — entries with user-typed leading `-`/`*`/`•` produce exactly one clean bullet each (golden string compare); `TestCustomInstruction_EmptyReturnsEmpty` — empty/whitespace-only list yields `""`. `generate_test.go`: `TestPlan_CustomInstructionsOnlyWhenAnswered` — artifact absent with no answers, present with at least one non-empty entry. |
| 7 | Unit tests (`library_test.go`): add/list/remove round-trip; adding identical (including differently-whitespaced) text twice returns the same entry and the library still has one item; removing an unknown id errors; listing against a store with no library file yet returns an empty slice, not an error. |
| 8 | Unit tests (`server_test.go`): `TestAPI_LibraryAddListDelete` — add via POST, list shows it, delete via DELETE, delete again returns 404; add with empty text returns 400. |
| 9, 10 | Component test (`InstructionListEditor.test.tsx`, Testing Library, mirroring `BundleImport.test.tsx`'s `vi.mock("../api")` pattern): typing text and pressing Enter adds a row and calls `onChange`; clicking a row's delete calls `onChange` with that row removed; clicking a row's save control calls `api.libraryAdd` with the row's text; clicking a library chip not yet in the list calls `onChange` with the chip's text appended. |
| 11 | Manual verification: `go run ./cmd/loadout wizard` against a temp store presents a multiline field for `standards.custom` and the generated `custom-instructions` artifact reflects the entered lines. |
| End-to-end | Manual UI walk (Builder → Standards → add/delete/save/reuse rows → Review → Generate) confirming the generated file has clean bullets and the library persists across a page reload. |

## Rollout

- No feature flag: additive schema question + additive API routes + additive UI branch; nothing existing changes behavior. Old answers files/profiles without `standards.custom` continue to work unchanged.
- Ship order: schema/answers/content/generate changes and their unit tests first (self-contained, no API surface), then the library package and its tests, then the web API routes and handler tests, then the frontend component and its test, finally the CLI wizard's `TypeList` support. All can land in one PR since the feature isn't usable end-to-end until the frontend piece exists, but should be reviewed as logically separate commits.
- `make build-web` must be run and the refreshed `internal/web/dist` committed in the same PR (per repo convention for any `web/` change).
- README: document the new `standards.custom` question, the generated `custom-instructions` artifact, the Builder's row editor + library, and the three new API endpoints.
- Rollback: revert the PR — no destructive migration; the only new persisted state is the optional `library/instructions.yaml` file, which is simply unused (not deleted) if rolled back.

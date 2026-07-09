# 005 — Naming safety: namespace prefix, reserved-name warnings, YAML upload

## Context

Two smaller newcomer-safety gaps, grouped because they share a theme (don't let a fresh user shoot themselves in the foot with names or file formats):

1. **Default names collide with agent built-ins.** The wizard's `artifactSpecs()` (`internal/wizard/generate.go`) defaults command IDs to `plan`, `spec`, `handoff`, `resume`. A generated `plan` command renders to `~/.claude/commands/plan.md` → `/plan`, which fights Claude Code's built-in `/plan`. Nothing warned about this, and there was no low-friction way to namespace a whole set of generated commands — only per-name manual renames. The only validation on artifact names (`store.ValidateName`, `internal/store/store.go`) checks kebab-case shape, not reserved words.

2. **The web import file-picker rejected YAML.** The single-file `.loadout.yaml` bundle format (spec-less, shipped in #13) imports fine server-side — `openBundle` (`internal/bundle/archive.go`) content-sniffs gzip magic and falls through to flat-YAML parsing — but the SPA file `<input>` was `accept=".tar.gz,.tgz"`, so a user with a `.loadout.yaml` had to know to use the separate paste-text tab. The uploader should just accept it.

Who is affected: anyone running the Builder (collision risk on the very command the wizard advertises), anyone scaffolding with `loadout new`, and anyone importing a teammate's YAML export through the web UI.

## Requirements

1. **MUST** add an optional wizard **namespace prefix** (`naming.prefix`) that, when set, prefixes the *defaulted* generated command/skill names (`plan` → `<prefix>-plan`), keeping them renamable exactly as before.
2. **MUST** apply the prefix only to names left at their default — an explicit per-name rename wins verbatim (no double-prefixing) — and **MUST** ignore an invalid prefix (same lenient validation as an explicit rename), leaving bare names.
3. **MUST** keep the question schema the single source of truth: one new schema entry surfaces the prefix on every questionnaire surface (CLI + web Builder) with no per-surface code.
4. **MUST** add a shared reserved-name check (`domain.ReservedNameWarning`) covering Claude Code's built-in slash commands, applied to command- and skill-kind artifacts (skills are slash-invocable too), and **MUST** treat it as advisory — it **never blocks** any operation (a user may deliberately shadow a built-in).
5. **MUST** surface reserved-name warnings in every place a command/skill name is chosen or introduced: the wizard plan (CLI + web), `loadout new`, bundle import (CLI + web plan), and `doctor` (as a non-failing note).
6. **MUST** place the reserved-name knowledge where every consumer can import it without violating the layering rules (`domain` — the only dependency-free package all of wizard/store/bundle/cli/web already import; adapter placement would create forbidden `wizard→adapter`/`bundle→adapter` edges).
7. **MUST** make the web bundle-import file picker accept `.yaml`/`.yml` in addition to `.tar.gz`/`.tgz`, with copy that says so. No backend change is required (content-sniffing already handles it); a test **MUST** prove a flat YAML uploaded through the multipart field imports.
8. **MUST NOT** change any existing default name (a prefix is opt-in; with no prefix and no rename, output is byte-identical to before) and **MUST NOT** break loading of existing saved profiles/presets that predate the `naming` section.

## Non-goals

- Hard-blocking reserved names anywhere — warnings only (requirement 4).
- Per-agent reserved-name tables — v1 curates one Claude Code list keyed by kind; per-agent knowledge is a later extension when a second agent needs a different list.
- Auto-suggesting or auto-applying a prefix — the wizard asks; it doesn't decide for the user.
- Renaming already-installed artifacts, or migrating a store's existing names — the prefix affects generation only.

## Design

### Approach

`internal/domain/reserved.go` holds a private `reservedCommandNames` map and one exported `ReservedNameWarning(kind, name) string` (empty = safe; only `KindCommand`/`KindSkill` can collide). Domain is the natural home: it's dependency-free and already imported everywhere, and the import rules forbid wizard/bundle from depending on adapter where per-agent command knowledge would otherwise live.

The wizard gains a `naming` schema section with one `naming.prefix` text question. `artifactSpecs()` resolves each renamable name through a local `named(id, def)` helper: an explicit rename (`customName`) wins verbatim; otherwise, if a valid prefix is set and the name is still the default, it returns `<prefix>-<def>`. `namespacePrefix()` mirrors `customName`'s lenient validation. `NameWarnings(a)` walks the resolved specs and returns `ReservedNameWarning` hits — so a prefixed or renamed set produces no warnings, which is the intended "you fixed it" feedback.

Warnings thread through each surface as plain advisory lines: CLI wizard/`new`/`bundle import`/`doctor` print `! …`; the web wizard-plan and bundle-import-plan responses gain a `warnings: []string` field the SPA renders. `ImportPlan` gains a `Warnings` field populated over `New ∪ Conflicts`.

The web uploader's `accept` list gains `.yaml,.yml`; the backend is unchanged because `openBundle` already sniffs content, so the multipart path (which names its temp file `.tar.gz`) parses a YAML body correctly.

### Alternatives considered

- **New static non-colliding defaults (e.g. `sdd-plan`).** Rejected: it trades one fixed name for another (still not the user's choice), churns every existing user's generated names, and doesn't generalize — the prefix lets a team namespace *all* generated commands consistently and is opt-in, so nothing changes for anyone who doesn't want it.
- **Hard-block reserved names.** Rejected: shadowing a built-in is occasionally intentional, and a hard block on `loadout new plan` or on import would be a hostile surprise. Warnings inform without taking away agency; the wizard's prefix gives an easy fix when the user does want to avoid the clash.
- **Put the reserved list in the adapter package** (per-agent command knowledge lives there conceptually). Rejected: it would force `wizard→adapter` and `bundle→adapter` import edges the architecture forbids. Domain is dependency-free and already imported by every consumer; the table is small, curated, and documented for later per-agent extension.
- **Prefix every generated name, including explicit renames.** Rejected: an explicit rename is the user stating exactly what they want; prefixing it would be surprising and could re-introduce a name they deliberately chose to avoid. Prefix applies to defaults only.
- **Add a YAML-specific upload endpoint / detect by extension.** Rejected: the backend already sniffs content, so the only change needed is widening the picker's `accept`. A second endpoint or extension-based routing would duplicate logic the sniffer already handles correctly.

## Data & API changes

### `internal/domain/reserved.go` (new)

```go
func ReservedNameWarning(kind ArtifactKind, name string) string // "" = safe
```

### `internal/wizard`

- `schema.go`: new `naming` section with `naming.prefix` (TypeText, default ""); the five rename prompts note the prefix applies to defaults.
- `content.go`: `namespacePrefix(a Answers) string`.
- `generate.go`: `artifactSpecs` composes prefix onto defaulted names; new `NameWarnings(a Answers) []string`.

### `internal/bundle`

- `ImportPlan.Warnings []string`, populated in `PlanImport` over `New ∪ Conflicts`.

### CLI

- `loadout wizard`, `loadout new`, `loadout bundle import`, `loadout doctor` print reserved-name warnings (advisory; exit code unchanged).

### Web API + SPA

- `POST /api/wizard/plan` response gains `warnings: []string`; `POST /api/bundle/import/plan` gains `warnings: []string`.
- `api.ts` types updated; `Builder.tsx` and `BundleImport.tsx` render warnings; `BundleImport.tsx` file input accepts `.yaml,.yml`.

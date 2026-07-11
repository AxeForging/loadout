# 006 — MCP server config management across agents

**Status:** Draft — pending review
**Owner:** Lucas Machado
**Related:** ROADMAP "Project-scoped artifacts", spec 004 (sync safety), spec 005 (naming safety)

## Context

Every agent loadout manages (Claude Code, Codex, Gemini CLI, Cursor, Antigravity) has its own
MCP server configuration format and location. Today users hand-maintain the same server entry
(command, args, env, URL) in three or four places, and there is no drift detection, no sharing,
and no safety gate around those files — exactly the problem loadout already solves for skills,
instructions, commands, hooks and settings.

MCP configs are also the first artifact where two long-deferred needs become unavoidable:

1. **Secrets.** MCP entries routinely carry API keys. The store must stay shareable
   (safety invariant 4: no secrets in the store), so literal secrets can never land in it.
2. **Project scope.** Several agents support per-project MCP config (`.mcp.json`,
   `.cursor/mcp.json`). The engine currently only knows agent homes; this spec introduces a
   minimal project-root scope that later project-scoped artifacts (Cursor rules, `.agents/`)
   can reuse.

Affected: anyone using MCP servers with more than one agent, and anyone sharing a store or
bundle with teammates.

## Requirements

Numbered for test-plan traceability. MUST = v1 blocker, SHOULD = v1 if cheap, else follow-up.

- **R1.** A new artifact kind `mcp` MUST exist. Canonical form: `mcps/<name>/mcp.yaml` in the
  store, one MCP server per artifact, listed in `loadout.yaml` like any other artifact
  (targets, tags, origin all work unchanged).
- **R2.** `mcp.yaml` MUST support stdio servers (`command`, `args`, `env`) and remote servers
  (`transport: http|sse`, `url`, `headers`). Validation MUST reject entries that are neither
  (no command and no url) or both.
- **R3.** Env/header values MUST support `${VAR}` placeholders passed through **verbatim** to
  the rendered agent config — loadout never resolves them. Values whose key looks
  secret-bearing (`*_KEY`, `*_TOKEN`, `*_SECRET`, `AUTHORIZATION`, case-insensitive) and whose
  value is not a `${VAR}` placeholder MUST be rejected at `add`/import/bundle-import time with
  a message telling the user to use a placeholder. There is no override flag in v1.
- **R4.** `mcp.yaml` MUST support `scope: user | project` (default `user`). User scope renders
  into the agent's global config; project scope renders relative to a project root.
- **R5.** Per-agent rendering (all MUST except where marked):

  | Agent | User scope | Project scope | Merge |
  |---|---|---|---|
  | claude-code | `~/.claude.json` → `mcpServers.<name>` | `<project>/.mcp.json` → `mcpServers.<name>` | json-merge |
  | gemini-cli | `~/.gemini/settings.json` → `mcpServers.<name>` | `<project>/.gemini/settings.json` | json-merge |
  | cursor | `~/.cursor/mcp.json` → `mcpServers.<name>` | `<project>/.cursor/mcp.json` | json-merge |
  | antigravity | shared Gemini layout — exact file verified during implementation (SHOULD) | — | json-merge |
  | codex | `~/.codex/config.toml` `[mcp_servers.<name>]` (SHOULD, see Design) | — | managed TOML block |

- **R6.** Engine project scope: `loadout sync|status|diff|check` MUST accept `--project <dir>`
  (default: current directory when it contains a `.git` or a project-scoped artifact is
  targeted; otherwise project-scoped artifacts are reported `n/a`). Project-scoped writes go
  through the exact same plan → confirm → apply gate as agent-home writes.
- **R7.** State: install hashes for project-scoped files MUST be recorded in the machine-local
  state lockfile keyed by absolute project path — never inside the project repo or the store.
- **R8.** Drift/conflict classification (spec 004) MUST work unchanged for MCP targets: a
  user-edited `mcpServers.<name>` entry is `drifted-local`; an entry loadout doesn't manage is
  never touched (json-merge only upserts the managed key).
- **R9.** *(Amended during implementation — see plan.)* Creation and adoption follow the
  repo's existing CLI split: `loadout new mcp <name>` MUST scaffold `mcps/<name>/mcp.yaml`
  from a commented template (placeholder-style env example), and `loadout add <dir>` MUST
  adopt a directory containing `mcp.yaml` (inference; `--kind mcp` accepted). The original
  flag-driven `add mcp` form was dropped: it would have introduced a second creation UX
  inconsistent with every other kind. Naming safety (spec 005) applies.
- **R10.** Bundles: MCP artifacts MUST round-trip through **all three** bundle forms —
  directory export, `tar.gz`, and the single-file flat YAML bundle (`mcps:` section).
  Import runs R2/R3 validation and the existing gated-import confirmation.
- **R11.** Web UI and TUI MUST list `mcp` artifacts wherever other kinds appear (library,
  status, selective export picker). No dedicated MCP editor UI in v1.
- **R12.** README, completion scripts and `.structlint.yaml` MUST be updated in the same PR
  as the implementation (dev rule 1/3).

## Non-goals

- Resolving or storing secret **values** anywhere (no local overrides file in v1 — revisit if
  placeholder-only proves painful).
- Managing project-scoped artifacts other than MCP (Cursor `.mdc` rules, `.agents/` — the
  project-root plumbing lands here, those kinds land later).
- Health-checking / launching MCP servers, OAuth flows, or per-server enable/disable state.
- Windows agent-home discovery differences beyond what adapters already do.
- A dedicated MCP editing UI in web/TUI.

## Design

### Canonical schema

```yaml
# mcps/github/mcp.yaml
transport: stdio            # stdio (default) | http | sse
command: npx
args: ["-y", "@modelcontextprotocol/server-github"]
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}   # placeholder — passed through verbatim
# remote form:
# transport: http
# url: https://mcp.example.com/v1
# headers:
#   Authorization: Bearer ${EXAMPLE_KEY}
scope: user                 # user | project
```

Parsed by a new `domain.MCPSpec` + `domain.ParseMCPSpec` (mirrors `HookSpec`/`ParseHookSpec`).
Validation (R2/R3) lives in domain so store, bundle import, and web API share one code path.

### TargetFile roots — the one engine change that matters

`adapter.TargetFile.RelPath` is currently relative to the agent home. Two new cases break
that assumption: `~/.claude.json` is a **sibling** of `~/.claude`, and project files are
relative to a project root. Rather than `..` hacks, `TargetFile` gains a root selector:

```go
type TargetRoot string
const (
    RootAgentHome TargetRoot = ""        // default — today's behavior, zero adapter churn
    RootUserHome  TargetRoot = "user-home"
    RootProject   TargetRoot = "project"
)
```

The engine resolves the base per root; adapters stay pure translators. A `RootProject` target
with no project root available is planned as `n/a`, never guessed.

**Alternative rejected:** allowing `../` in RelPath — fragile, defeats path-safety checks, and
doesn't solve project scope. **Alternative rejected:** treating `~/.claude.json` writes as a
claude-code "settings" special case — spreads root logic into adapters, violating
"paths in data, not scattered logic".

### Codex TOML (the `SHOULD`)

Rewriting `config.toml` destroys user comments (why settings are unsupported there today).
MCP entries can instead use the existing **managed block** strategy: TOML comments make
sentinels legal, and `[mcp_servers.<name>]` tables appended inside a
`# <!-- loadout:begin mcp-<name> -->` block at EOF are valid TOML. One edge: if the user
already defines the same table outside the block, the file fails to parse → the adapter's
plan step MUST detect a pre-existing `[mcp_servers.<name>]` outside loadout's block and
classify it as `conflict` instead of writing. If this proves brittle during implementation,
codex MCP degrades to documented-unsupported (it is a SHOULD) without blocking the release.

**Alternative rejected:** comment-preserving TOML AST rewrite — heavy dependency for one
agent; revisit when codex settings support lands.

### Secret heuristic (R3)

Deny-by-default on key names only (`(?i)(key|token|secret|password|authorization)`), allow
when the value is exactly a `${VAR}` reference or empty. Key-name matching gives no false
negatives on the common cases and the failure mode of a false positive is a clear message
("use `${MY_VAR}`"), which is acceptable. Value-entropy scanning rejected: flaky, and a
confident-looking non-match would be a worse outcome than an occasional forced placeholder.

### CLI/flow summary

`add mcp` writes the canonical artifact; `status/diff/sync` work via the normal engine path;
`--project` threads a project root into Plan/Apply. Nothing new writes to disk outside the
engine (safety invariant 1) and every write remains behind the confirm gate (invariant 2).

## Data & API changes

- `domain`: `KindMCP ArtifactKind = "mcp"`, `MCPSpec`, `ParseMCPSpec`, secret validation.
- `adapter`: `TargetFile.Root TargetRoot` (additive; zero value = current behavior).
- `engine`: Plan/Status/Apply accept optional project root; root resolution; state lockfile
  entries gain a root/path discriminator (additive field — old entries read unchanged).
- `store`: scaffold for `mcps/<name>/mcp.yaml`; manifest untouched structurally (`kind: mcp`
  is a new enum value under the existing `loadout/v1` apiVersion).
- `bundle`: dir/tar.gz include `mcps/`; flat YAML schema gains `mcps:` (name → spec + targets).
- `web`: artifact listing/export/import endpoints pass `mcp` through; JSON casing follows the
  existing exported-field contract.
- No migrations. Stores without MCP artifacts are untouched; older binaries reading a store
  *with* `kind: mcp` fail loudly on the unknown kind — release notes state the minimum
  version, no silent skips.

## Test plan

Per requirement; unit tests colocated, integration under `integration/` (tag `integration`,
real binary, temp HOME + fake agent homes, per team convention).

- **R1/R2/R3 (unit, domain/store):** parse valid stdio/remote specs; reject neither/both
  command+url; reject `GITHUB_TOKEN: ghp_abc123` with placeholder guidance; accept
  `${GITHUB_TOKEN}`; accept non-secret literal env (`DEBUG: "1"`). Table tests, negative and
  positive cases.
- **R5 (unit, per adapter, golden files):** render each agent's fragment for stdio and remote
  servers; codex golden covers block layout and the pre-existing-table conflict case.
- **R4/R6/R7 (integration):** binary in a temp project dir — `sync` writes `.mcp.json` /
  `.cursor/mcp.json` only after confirm; state lands in the data dir keyed by project path;
  `status` without a project root reports project-scoped artifacts `n/a`.
- **R8 (integration):** hand-edit a managed `mcpServers` entry → `drifted-local`; a foreign
  entry in the same file survives sync byte-for-byte.
- **Invariant (integration):** extend the permanent no-confirm-no-write test to cover
  `RootUserHome` and `RootProject` targets — nil/false confirm writes nothing anywhere.
- **R9 (integration):** `loadout add mcp` via flags creates artifact + manifest entry;
  secret literal via `--env` fails with the R3 message (exit non-zero).
- **R10 (integration):** export → import round-trip for all three bundle forms preserves MCP
  specs byte-for-byte; a tampered YAML bundle with a literal secret is rejected at import.
- **R11 (unit, web/TUI):** library listing includes `mcp` kind; selective export picker
  offers it.

## Rollout

1. Land in dependency order: domain → adapter root support + engine → claudecode/gemini/cursor
   adapters → CLI `add mcp` + `--project` → bundles → web/TUI listing → codex block (last,
   droppable). Each step keeps `make ci` + `make test-integration` green.
2. No feature flag: the kind is inert until a user adds an MCP artifact.
3. README + completion scripts + `.structlint.yaml` in the implementation PRs (R12).
4. Rollback: remove MCP artifacts from the store; managed entries are removed by the normal
   orphan-detection path. No data migration to unwind.
5. Follow-ups tracked in ROADMAP: antigravity exact path (if not confirmed in v1), local
   secret-overrides file, project scope for other artifact kinds.

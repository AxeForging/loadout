---
name: {{workflow}}
description: The team's spec-driven development flow — when specs are required, where specs and their plans live (GitHub issues, not repo files), and how /{{spec}}, /{{plan}}, /{{handoff}} and /{{resume}} chain together. Use when starting any non-trivial feature or resuming one.
---

# Spec-driven development workflow

Specs and plans live in **GitHub issues** (via `gh`), so they never clutter the repo. One issue per spec: label `spec`, title `spec(NNN): <short name>`, body = spec sections + a plan section between `<!-- plan:start -->` / `<!-- plan:end -->`.

1. **Spec first** — for any feature beyond a small fix, run /{{spec}}: it interviews, drafts, and opens the spec issue.
2. **Review** — discuss in issue comments or chat; approval adds the `approved` label (or is waived explicitly in the PR description).
3. **Plan in the issue** — run /{{plan}}: it writes an ordered `- [ ]` checklist into the issue's plan section. The issue — not the conversation — is the source of truth for progress.
4. **Implement with the checklist** — check off steps in the issue as they complete (`loadout spec check NNN <N> --backend issues`) and append discovered steps. If reality diverges from the spec, update the spec section and leave a comment explaining the change.
5. **PR** — `Refs #N` on intermediate PRs, `Closes #N` on the one that finishes the plan.

## Editing issues safely

- Re-fetch the body (`gh issue view N --json body -q .body`) right before every edit, change a temp copy (`mktemp`, outside the repo), write back with `gh issue edit N --body-file`. Never rewrite from memory.
- Touch only the section you mean to change; keep the plan markers intact.
- Body limit is 65,536 characters; move long appendices to comments.

## Change scope

{{scope}}

## Crash / restart / agent-switch safety

Because progress lives in the issue, work is resumable from any machine or agent with `gh`:
- After a restart or crash, or in a different agent: run /{{resume}} — it reads the latest `<!-- handoff -->` comment on the spec issue AND the plan checklist, then continues at the first unchecked step.
- Before ending a session mid-feature, run /{{handoff}} — it comments the handoff on the spec issue.
- Never keep plan state only in your head or the chat — if a step completed, its checkbox must be checked in the issue before moving on.

## Fallback

No GitHub remote, or `gh` not authenticated → say so and ask before falling back to local `docs/specs/NNN-<name>.md` files with the same structure.

Never start implementation of a large feature without a spec — if asked to, suggest drafting one with /{{spec}} first.

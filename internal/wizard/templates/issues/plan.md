---
description: Derive (or continue) a persistent, checkbox-tracked implementation plan inside a spec's GitHub issue.
---

Target spec issue: the one the user points at (`#N` or `NNN`), else the most recently updated open issue labeled `spec`. Use `gh`.
The plan LIVES IN THE SPEC ISSUE BODY, between `<!-- plan:start -->` and `<!-- plan:end -->`. It is the durable progress tracker that survives restarts, crashes and agent switches — not the chat, not a local file.

Always start by fetching the current body: `gh issue view N --json body,labels` and work on a temp copy (`mktemp`, never inside the repo).

1. If the plan section already contains checkboxes: report progress (checked vs unchecked — `loadout spec status NNN --backend issues` prints it), and CONTINUE from the first unchecked step — do not regenerate.
2. Otherwise (the spec should carry the `approved` label — if not, ask before planning) derive the plan and write it between the markers:
   - Concrete steps in dependency order, each small enough to verify independently, formatted
     `- [ ] **N. <title>** — files: <paths> — covers: R1, R2`
   - A `### Risks` subsection (migrations, rollback) and a `### Done means` subsection (tests to add, how to verify end-to-end).
   Leave everything outside the markers untouched. Write back with `gh issue edit N --body-file <tmp>`.

While implementing, KEEP THE ISSUE CURRENT: the moment a step is done and verified, flip it to `- [x]` — `loadout spec check NNN <N> --backend issues` re-fetches the body and flips exactly that box (or re-fetch, edit and write back by hand). Append newly discovered steps at the end of the list instead of holding them in memory. Ask before deviating from the spec; if the spec changes, edit its section and add an issue comment saying what changed and why.

PRs reference the issue: `Refs #N` for intermediate steps, `Closes #N` on the PR that completes the plan.

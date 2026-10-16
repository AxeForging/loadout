package wizard

import (
	"fmt"
	"strings"
)

// Issues-only SDD (sdd.backend: issues, spec 011): every spec AND its plan
// live in one GitHub issue (label `spec`, title `spec(NNN): name`, plan
// checklist between plan markers, handoffs as `<!-- handoff -->` comments).
// Nothing is written into the repository. These generators replace the
// file-based ones when that backend is selected; local/github output is
// untouched.

// sddIssuesMode reports whether SDD artifacts should use the issues-only
// flow. Handoff/resume follow it too, so they comment on the spec issue.
func sddIssuesMode(a Answers) bool {
	return a.Bool("sdd.enable") && sddBackend(a) == "issues"
}

// issueEditRule is the fetch-first editing discipline shared by every
// generated artifact that touches a spec issue body.
const issueEditRule = "fetch the current body first (`gh issue view N --json body -q .body > <tmp>`), edit that file, then `gh issue edit N --body-file <tmp>`. Never rewrite the body from memory."

func sddIssuesInstruction(specID, planID string) string {
	return fmt.Sprintf(`# Spec-driven development

Non-trivial features start with a spec before any code. Specs and their plans live in **GitHub issues** (label `+"`spec`"+`, title `+"`spec(NNN): short-name`"+`), not in the repo. Use `+"`/%[1]s`"+` to draft one and `+"`/%[2]s`"+` to derive the implementation plan into the same issue.

- A spec must be reviewed (`+"`approved`"+` label) or explicitly waived before implementation.
- The plan is the checklist inside the spec issue; PRs reference it (`+"`Refs #N` / `Closes #N`"+`).
`, specID, planID)
}

func specIssuesCommand(a Answers, planID string) string {
	sections := `   ## Context — why now, what problem, who is affected
   ## Requirements — numbered (R1, R2…), testable statements (MUST/SHOULD)
   ## Non-goals — explicitly out of scope
   ## Design — approach, alternatives considered and why rejected
   ## Data & API changes — schemas, endpoints, migrations
   ## Test plan — per requirement, how it is verified
   ## Rollout — flags, migration order, rollback`
	description := "Draft a full spec (context, requirements, design, test plan, rollout) for a feature as a GitHub issue."
	style := "full"
	if a.Str("sdd.style") != "full" {
		sections = `   ## Problem — one paragraph, user-visible framing
   ## Approach — the chosen solution in a few bullets, plus what was ruled out`
		description = "Draft a light spec (problem, approach) for a feature as a GitHub issue."
		style = "light"
	}
	return fmt.Sprintf(`---
description: %[1]s
---

Specs live in **GitHub issues**, not in the repo. One issue per spec, labeled `+"`spec`"+`, title `+"`spec(NNN): <short name>`"+`. Use the `+"`gh`"+` CLI. If the repo has no GitHub remote or `+"`gh auth status`"+` fails, stop and ask whether to fall back to a local `+"`docs/specs/`"+` file.

1. **Number**: NNN = highest number among `+"`gh issue list --label spec --state all --limit 500 --json title`"+` titles matching `+"`spec(NNN):`"+`, plus 1 (zero-padded, start at 001).
2. **Interview** me for anything ambiguous before writing. Keep requirements independently verifiable.
3. **Draft** the body in a temp file (`+"`mktemp`"+`, never inside the repo) with these sections:

%[2]s

   End the body with the plan placeholder, exactly:

   `+"```"+`
   <!-- plan:start -->
   ## Plan
   _Not derived yet — run /%[3]s after the spec is approved._
   <!-- plan:end -->
   `+"```"+`

4. **Create**: ensure the label exists (`+"`gh label create spec --color 5319E7 --description \"Spec + plan (spec-driven development)\" 2>/dev/null || true`"+`), then `+"`gh issue create --title \"spec(NNN): <short name>\" --label spec --body-file <tmp>`"+`. Report the issue URL.
   Shortcut: `+"`loadout spec init \"<short name>\" --backend issues --style %[4]s`"+` does steps 1 and 4 in one go (numbering, label, template body) — then fill the sections in.
5. **Review**: the spec is approved when I say so (or comment approval on the issue). On approval add the `+"`approved`"+` label (create it if missing). Review feedback goes into the issue body; note each material change in an issue comment so the history is readable.

Editing rule for every later change: %[5]s Keep the body under ~60k characters (GitHub's limit is 65,536); move long appendices to comments.
`, description, sections, planID, style, issueEditRule)
}

func planIssuesCommand() string {
	return `---
description: Derive (or continue) a persistent, checkbox-tracked implementation plan inside a spec's GitHub issue.
---

Target spec issue: the one the user points at (` + "`#N`" + ` or ` + "`NNN`" + `), else the most recently updated open issue labeled ` + "`spec`" + `. Use ` + "`gh`" + `.
The plan LIVES IN THE SPEC ISSUE BODY, between ` + "`<!-- plan:start -->`" + ` and ` + "`<!-- plan:end -->`" + `. It is the durable progress tracker that survives restarts, crashes and agent switches — not the chat, not a local file.

Always start by fetching the current body: ` + "`gh issue view N --json body,labels`" + ` and work on a temp copy (` + "`mktemp`" + `, never inside the repo).

1. If the plan section already contains checkboxes: report progress (checked vs unchecked — ` + "`loadout spec status NNN --backend issues`" + ` prints it), and CONTINUE from the first unchecked step — do not regenerate.
2. Otherwise (the spec should carry the ` + "`approved`" + ` label — if not, ask before planning) derive the plan and write it between the markers:
   - Concrete steps in dependency order, each small enough to verify independently, formatted
     ` + "`- [ ] **N. <title>** — files: <paths> — covers: R1, R2`" + `
   - A ` + "`### Risks`" + ` subsection (migrations, rollback) and a ` + "`### Done means`" + ` subsection (tests to add, how to verify end-to-end).
   Leave everything outside the markers untouched. Write back with ` + "`gh issue edit N --body-file <tmp>`" + `.

While implementing, KEEP THE ISSUE CURRENT: the moment a step is done and verified, flip it to ` + "`- [x]`" + ` — ` + "`loadout spec check NNN <N> --backend issues`" + ` re-fetches the body and flips exactly that box (or re-fetch, edit and write back by hand). Append newly discovered steps at the end of the list instead of holding them in memory. Ask before deviating from the spec; if the spec changes, edit its section and add an issue comment saying what changed and why.

PRs reference the issue: ` + "`Refs #N`" + ` for intermediate steps, ` + "`Closes #N`" + ` on the PR that completes the plan.
`
}

func sddIssuesWorkflowSkill(a Answers, workflowID, specID, planID, handoffID, resumeID string) string {
	return fmt.Sprintf(`---
name: %[1]s
description: The team's spec-driven development flow — when specs are required, where specs and their plans live (GitHub issues, not repo files), and how /%[2]s, /%[3]s, /%[4]s and /%[5]s chain together. Use when starting any non-trivial feature or resuming one.
---

# Spec-driven development workflow

Specs and plans live in **GitHub issues** (via `+"`gh`"+`), so they never clutter the repo. One issue per spec: label `+"`spec`"+`, title `+"`spec(NNN): <short name>`"+`, body = spec sections + a plan section between `+"`<!-- plan:start -->` / `<!-- plan:end -->`"+`.

1. **Spec first** — for any feature beyond a small fix, run /%[2]s: it interviews, drafts, and opens the spec issue.
2. **Review** — discuss in issue comments or chat; approval adds the `+"`approved`"+` label (or is waived explicitly in the PR description).
3. **Plan in the issue** — run /%[3]s: it writes an ordered `+"`- [ ]`"+` checklist into the issue's plan section. The issue — not the conversation — is the source of truth for progress.
4. **Implement with the checklist** — check off steps in the issue as they complete (`+"`loadout spec check NNN <N> --backend issues`"+`) and append discovered steps. If reality diverges from the spec, update the spec section and leave a comment explaining the change.
5. **PR** — `+"`Refs #N`"+` on intermediate PRs, `+"`Closes #N`"+` on the one that finishes the plan.

## Editing issues safely

- Re-fetch the body (`+"`gh issue view N --json body -q .body`"+`) right before every edit, change a temp copy (`+"`mktemp`"+`, outside the repo), write back with `+"`gh issue edit N --body-file`"+`. Never rewrite from memory.
- Touch only the section you mean to change; keep the plan markers intact.
- Body limit is 65,536 characters; move long appendices to comments.

## Change scope

%[6]s

## Crash / restart / agent-switch safety

Because progress lives in the issue, work is resumable from any machine or agent with `+"`gh`"+`:
- After a restart or crash, or in a different agent: run /%[5]s — it reads the latest `+"`<!-- handoff -->`"+` comment on the spec issue AND the plan checklist, then continues at the first unchecked step.
- Before ending a session mid-feature, run /%[4]s — it comments the handoff on the spec issue.
- Never keep plan state only in your head or the chat — if a step completed, its checkbox must be checked in the issue before moving on.

## Fallback

No GitHub remote, or `+"`gh`"+` not authenticated → say so and ask before falling back to local `+"`docs/specs/NNN-<name>.md`"+` files with the same structure.

Never start implementation of a large feature without a spec — if asked to, suggest drafting one with /%[2]s first.
`, workflowID, specID, planID, handoffID, resumeID, changeScopeGuidance(a.Str("sdd.changeScope")))
}

func handoffIssuesCommand(resumeID string) string {
	return fmt.Sprintf(`---
description: Dump the current session context so any agent (or a later session) can pick up exactly where this one left off — as a comment on the active spec issue, or a local untracked file when there is none.
---

Where to write:
- **Active spec issue** (work is tied to a GitHub issue labeled `+"`spec`"+`): post a comment with `+"`gh issue comment N --body-file <tmp>`"+` (temp file from `+"`mktemp`"+`, never inside the repo). The comment's first line MUST be `+"`<!-- handoff -->`"+` so /%[1]s can find the latest one.
- **No spec issue**: write `+"`.agents/context/handoff.md`"+` (create the directory, overwrite the previous handoff) and keep it untracked: add `+"`.agents/context/`"+` to `+"`.git/info/exclude`"+` unless it is already ignored.

Structure it exactly like this:

# Handoff — <date> <time>

## Goal
What we are ultimately trying to achieve, in one or two sentences.

## Current state
What is DONE and verified, what is in progress, what is untouched. Reference concrete files, branches and PRs. If a spec issue is active, name it (`+"`#N`"+`) — its plan checkboxes carry the step-level progress.

## Decisions & constraints
Choices already made (and why), approaches ruled out, constraints to respect.

## Next steps
Ordered, concrete, resumable steps. The first one should be actionable immediately.

## Gotchas
Anything surprising discovered this session that would waste the next agent's time to rediscover.

Keep it dense and factual — this is for another AI agent, not a status report for humans. After writing, give the comment URL or the file path.
`, resumeID)
}

func resumeIssuesCommand(handoffID string) string {
	return fmt.Sprintf(`---
description: Resume work from the last context handoff written by /%[1]s (from any agent or a previous session).
---

1. Find the active spec issue: the one the user names, else the most recently updated open issue labeled `+"`spec`"+` (`+"`gh issue list --label spec --state open --json number,title,updatedAt`"+`).
2. Read the latest handoff: the newest comment on that issue whose body starts with `+"`<!-- handoff -->`"+` (`+"`gh issue view N --comments --json comments`"+`); if there is none, read `+"`.agents/context/handoff.md`"+`. If neither exists, say so and ask what to work on instead.
3. Read the plan in the issue body (between `+"`<!-- plan:start -->`"+` and `+"`<!-- plan:end -->`"+`) — its unchecked `+"`- [ ]`"+` items are the authoritative remaining work, newer than any handoff.
4. Cross-check against reality: `+"`git status`"+`, `+"`git log --oneline -5`"+`, PRs referencing the issue (`+"`gh pr list --state all --search \"#N\"`"+`), and spot-check any file the handoff claims was changed.
5. Summarize in a few lines: the goal, where things stand, and the first next step. Flag any drift between the handoff/plan and the actual repo state.
6. Continue with the first unchecked plan step (or the handoff's first next step). Respect recorded decisions — do not relitigate them without new information. Keep checking off plan items in the issue as you complete them (re-fetch the body before every edit).
`, handoffID)
}

func contextHandoffIssuesSkill(handoffID, resumeID string) string {
	return strings.NewReplacer(
		"- **Location**: `.agents/context/handoff.md` in the project root (agent-agnostic — Claude Code, Codex, Gemini CLI, Antigravity and Cursor can all read and write it).",
		"- **Location**: a comment on the active spec issue whose first line is `<!-- handoff -->` (any agent with `gh` can read it); with no spec issue, `.agents/context/handoff.md` in the project root, kept untracked via `.git/info/exclude`.",
		"- **Lifecycle**: one live handoff per project; it is safe to commit. Delete or rewrite it when the task completes — a stale handoff is worse than none.",
		"- **Lifecycle**: the newest `<!-- handoff -->` comment wins; the local file is never committed. Rewrite it when the task moves on — a stale handoff is worse than none.",
	).Replace(contextHandoffSkill(handoffID, resumeID))
}

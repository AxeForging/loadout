---
description: Dump the current session context so any agent (or a later session) can pick up exactly where this one left off — as a comment on the active spec issue, or a local untracked file when there is none.
---

Where to write:
- **Active spec issue** (work is tied to a GitHub issue labeled `spec`): post a comment with `gh issue comment N --body-file <tmp>` (temp file from `mktemp`, never inside the repo). The comment's first line MUST be `<!-- handoff -->` so /{{resume}} can find the latest one.
- **No spec issue**: write `.agents/context/handoff.md` (create the directory, overwrite the previous handoff) and keep it untracked: add `.agents/context/` to `.git/info/exclude` unless it is already ignored.

Structure it exactly like this:

# Handoff — <date> <time>

## Goal
What we are ultimately trying to achieve, in one or two sentences.

## Current state
What is DONE and verified, what is in progress, what is untouched. Reference concrete files, branches and PRs. If a spec issue is active, name it (`#N`) — its plan checkboxes carry the step-level progress.

## Decisions & constraints
Choices already made (and why), approaches ruled out, constraints to respect.

## Next steps
Ordered, concrete, resumable steps. The first one should be actionable immediately.

## Gotchas
Anything surprising discovered this session that would waste the next agent's time to rediscover.

Keep it dense and factual — this is for another AI agent, not a status report for humans. After writing, give the comment URL or the file path.

---
description: Resume work from the last context handoff written by /{{handoff}} (from any agent or a previous session).
---

1. Find the active spec issue: the one the user names, else the most recently updated open issue labeled `spec` (`gh issue list --label spec --state open --json number,title,updatedAt`).
2. Read the latest handoff: the newest comment on that issue whose body starts with `<!-- handoff -->` (`gh issue view N --comments --json comments`); if there is none, read `.agents/context/handoff.md`. If neither exists, say so and ask what to work on instead.
3. Read the plan in the issue body (between `<!-- plan:start -->` and `<!-- plan:end -->`) — its unchecked `- [ ]` items are the authoritative remaining work, newer than any handoff.
4. Cross-check against reality: `git status`, `git log --oneline -5`, PRs referencing the issue (`gh pr list --state all --search "#N"`), and spot-check any file the handoff claims was changed.
5. Summarize in a few lines: the goal, where things stand, and the first next step. Flag any drift between the handoff/plan and the actual repo state.
6. Continue with the first unchecked plan step (or the handoff's first next step). Respect recorded decisions — do not relitigate them without new information. Keep checking off plan items in the issue as you complete them (re-fetch the body before every edit).

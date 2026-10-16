# Spec-driven development

Non-trivial features start with a spec before any code. Specs and their plans live in **GitHub issues** (label `spec`, title `spec(NNN): short-name`), not in the repo. Use `/{{spec}}` to draft one and `/{{plan}}` to derive the implementation plan into the same issue.

- A spec must be reviewed (`approved` label) or explicitly waived before implementation.
- The plan is the checklist inside the spec issue; PRs reference it (`Refs #N` / `Closes #N`).

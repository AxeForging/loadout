---
description: {{description}}
---

Specs live in **GitHub issues**, not in the repo. One issue per spec, labeled `spec`, title `spec(NNN): <short name>`. Use the `gh` CLI. If the repo has no GitHub remote or `gh auth status` fails, stop and ask whether to fall back to a local `docs/specs/` file.

1. **Number**: NNN = highest number among `gh issue list --label spec --state all --limit 500 --json title` titles matching `spec(NNN):`, plus 1 (zero-padded, start at 001).
2. **Interview** me for anything ambiguous before writing. Keep requirements independently verifiable.
3. **Draft** the body in a temp file (`mktemp`, never inside the repo) with these sections:

{{sections}}

   End the body with the plan placeholder, exactly:

   ```
   <!-- plan:start -->
   ## Plan
   _Not derived yet — run /{{plan}} after the spec is approved._
   <!-- plan:end -->
   ```

4. **Create**: ensure the label exists (`gh label create spec --color 5319E7 --description "Spec + plan (spec-driven development)" 2>/dev/null || true`), then `gh issue create --title "spec(NNN): <short name>" --label spec --body-file <tmp>`. Report the issue URL.
   Shortcut: `loadout spec init "<short name>" --backend issues --style {{style}}` does steps 1 and 4 in one go (numbering, label, template body) — then fill the sections in.
5. **Review**: the spec is approved when I say so (or comment approval on the issue). On approval add the `approved` label (create it if missing). Review feedback goes into the issue body; note each material change in an issue comment so the history is readable.

Editing rule for every later change: fetch the current body first (`gh issue view N --json body -q .body > <tmp>`), edit that file, then `gh issue edit N --body-file <tmp>`. Never rewrite the body from memory. Keep the body under ~60k characters (GitHub's limit is 65,536); move long appendices to comments.

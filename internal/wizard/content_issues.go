package wizard

import (
	"embed"
	"strings"
)

// Issues-only SDD (sdd.backend: issues, spec 011): every spec AND its plan
// live in one GitHub issue (label `spec`, title `spec(NNN): name`, plan
// checklist between plan markers, handoffs as `<!-- handoff -->` comments).
// Nothing is written into the repository. These generators replace the
// file-based ones when that backend is selected; local/github output is
// untouched.
//
// The artifact bodies are plain markdown under templates/issues/ (embedded),
// with {{placeholders}} for the resolved command/skill names, so the prose
// stays readable and reviewable as markdown.

//go:embed templates/issues/*.md
var issuesTemplates embed.FS

// sddIssuesMode reports whether SDD artifacts should use the issues-only
// flow. Handoff/resume follow it too, so they comment on the spec issue.
func sddIssuesMode(a Answers) bool {
	return a.Bool("sdd.enable") && sddBackend(a) == "issues"
}

// renderIssuesTemplate fills templates/issues/<name> with key/value pairs
// ("spec", "spec-af", …) replacing {{key}}.
func renderIssuesTemplate(name string, kv ...string) string {
	raw, err := issuesTemplates.ReadFile("templates/issues/" + name)
	if err != nil {
		panic("wizard: missing embedded template " + name) // a build-time invariant
	}
	pairs := make([]string, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		pairs = append(pairs, "{{"+kv[i]+"}}", kv[i+1])
	}
	return strings.NewReplacer(pairs...).Replace(string(raw))
}

func sddIssuesInstruction(specID, planID string) string {
	return renderIssuesTemplate("instruction.md", "spec", specID, "plan", planID)
}

const (
	fullIssueSections = `   ## Context — why now, what problem, who is affected
   ## Requirements — numbered (R1, R2…), testable statements (MUST/SHOULD)
   ## Non-goals — explicitly out of scope
   ## Design — approach, alternatives considered and why rejected
   ## Data & API changes — schemas, endpoints, migrations
   ## Test plan — per requirement, how it is verified
   ## Rollout — flags, migration order, rollback`
	lightIssueSections = `   ## Problem — one paragraph, user-visible framing
   ## Approach — the chosen solution in a few bullets, plus what was ruled out`
)

func specIssuesCommand(a Answers, planID string) string {
	if a.Str("sdd.style") == "full" {
		return renderIssuesTemplate("spec.md", "plan", planID, "style", "full", "sections", fullIssueSections,
			"description", "Draft a full spec (context, requirements, design, test plan, rollout) for a feature as a GitHub issue.")
	}
	return renderIssuesTemplate("spec.md", "plan", planID, "style", "light", "sections", lightIssueSections,
		"description", "Draft a light spec (problem, approach) for a feature as a GitHub issue.")
}

func planIssuesCommand() string { return renderIssuesTemplate("plan.md") }

func sddIssuesWorkflowSkill(a Answers, workflowID, specID, planID, handoffID, resumeID string) string {
	return renderIssuesTemplate("workflow-skill.md",
		"workflow", workflowID, "spec", specID, "plan", planID, "handoff", handoffID, "resume", resumeID,
		"scope", changeScopeGuidance(a.Str("sdd.changeScope")))
}

func handoffIssuesCommand(resumeID string) string {
	return renderIssuesTemplate("handoff.md", "resume", resumeID)
}

func resumeIssuesCommand(handoffID string) string {
	return renderIssuesTemplate("resume.md", "handoff", handoffID)
}

func contextHandoffIssuesSkill(handoffID, resumeID string) string {
	return strings.NewReplacer(
		"- **Location**: `.agents/context/handoff.md` in the project root (agent-agnostic — Claude Code, Codex, Gemini CLI, Antigravity and Cursor can all read and write it).",
		"- **Location**: a comment on the active spec issue whose first line is `<!-- handoff -->` (any agent with `gh` can read it); with no spec issue, `.agents/context/handoff.md` in the project root, kept untracked via `.git/info/exclude`.",
		"- **Lifecycle**: one live handoff per project; it is safe to commit. Delete or rewrite it when the task completes — a stale handoff is worse than none.",
		"- **Lifecycle**: the newest `<!-- handoff -->` comment wins; the local file is never committed. Rewrite it when the task moves on — a stale handoff is worse than none.",
	).Replace(contextHandoffSkill(handoffID, resumeID))
}

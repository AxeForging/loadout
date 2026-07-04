package runner

import (
	"context"
	"fmt"
	"strings"
)

// DraftSkill asks a runner to write an Agent Skills-standard SKILL.md from a
// short description. The output is validated before anyone gets to write it.
func DraftSkill(ctx context.Context, r Runner, name, description string) (string, error) {
	prompt := fmt.Sprintf(`Write a SKILL.md file following the Agent Skills standard (agentskills.io) for a skill named %q.

What the skill should do: %s

Requirements:
- Start with YAML frontmatter delimited by --- lines containing exactly two fields: "name: %s" and a one-line "description:" that states what the skill does AND when an agent should use it.
- After the frontmatter: a markdown body with a title, a "When to use" section, and concrete step-by-step instructions an AI coding agent can follow.
- Be specific and actionable — no filler, no marketing language.
- Output ONLY the raw file content. No surrounding explanation, no code fences.`, name, description, name)

	out, err := r.Run(ctx, prompt)
	if err != nil {
		return "", err
	}
	content := StripFences(out)
	if err := ValidateSkillMD(content, name); err != nil {
		return "", fmt.Errorf("runner %s returned invalid SKILL.md: %w", r.ID(), err)
	}
	return content, nil
}

// EnrichMarkdown asks a runner to deepen an existing instruction document
// using the team's context, preserving its role and format.
func EnrichMarkdown(ctx context.Context, r Runner, docName, current, teamContext string) (string, error) {
	prompt := fmt.Sprintf(`You are improving one instruction document that gets installed into AI coding agents' global instructions.

Team context:
%s

Current content of %s:
---
%s
---

Rewrite this document to be more specific and more useful for THIS team's stack: concrete commands, version-appropriate guidance, real pitfalls. Keep the same overall structure and heading style, keep it dense (agents read this, not humans), and do not invent tools the team did not mention.
Output ONLY the raw markdown document. No explanation, no code fences.`, teamContext, docName, current)

	out, err := r.Run(ctx, prompt)
	if err != nil {
		return "", err
	}
	content := StripFences(out)
	if strings.TrimSpace(content) == "" || !strings.Contains(content, "#") {
		return "", fmt.Errorf("runner %s returned empty or non-markdown output", r.ID())
	}
	return content, nil
}

// StripFences removes a wrapping markdown code fence if the model added one
// despite instructions.
func StripFences(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return ensureTrailingNewline(s)
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 {
		return ensureTrailingNewline(s)
	}
	// Drop the opening fence line and a trailing closing fence.
	lines = lines[1:]
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > 0 && strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		lines = lines[:len(lines)-1]
	}
	return ensureTrailingNewline(strings.Join(lines, "\n"))
}

// ValidateSkillMD checks the minimal Agent Skills contract: frontmatter with
// the right name and a description, plus a body.
func ValidateSkillMD(content, name string) error {
	if !strings.HasPrefix(content, "---\n") {
		return fmt.Errorf("missing frontmatter opening ---")
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return fmt.Errorf("missing frontmatter closing ---")
	}
	header := rest[:end]
	if !strings.Contains(header, "name: "+name) {
		return fmt.Errorf("frontmatter must contain %q", "name: "+name)
	}
	if !strings.Contains(header, "description:") {
		return fmt.Errorf("frontmatter missing description")
	}
	body := strings.TrimSpace(rest[end+len("\n---"):])
	if body == "" {
		return fmt.Errorf("empty skill body")
	}
	return nil
}

func ensureTrailingNewline(s string) string {
	if !strings.HasSuffix(s, "\n") {
		return s + "\n"
	}
	return s
}

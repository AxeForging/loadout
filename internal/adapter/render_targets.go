package adapter

import (
	"fmt"
	"path"

	"github.com/AxeForging/loadout/internal/domain"
)

// InstructionBlock renders a single-file instruction artifact as a managed
// block inside the agent's global instruction file (CLAUDE.md, AGENTS.md,
// GEMINI.md, ...).
func InstructionBlock(file string, art domain.Artifact, content domain.Content) ([]TargetFile, error) {
	body, err := SingleFile(art, content)
	if err != nil {
		return nil, err
	}
	return []TargetFile{{
		RelPath: file,
		Content: WrapBlock(art.ID, body),
		Mode:    0o644,
		Merge:   MergeBlock,
	}}, nil
}

// CommandSkillFiles renders a command artifact as an Agent Skills skill under
// baseDir, for agents whose slash commands are backed by skills.
func CommandSkillFiles(baseDir string, art domain.Artifact, content domain.Content) ([]TargetFile, error) {
	body, err := SingleFile(art, content)
	if err != nil {
		return nil, err
	}
	return []TargetFile{{
		RelPath: path.Join(baseDir, art.ID, "SKILL.md"),
		Content: CommandAsSkill(art.ID, body),
		Mode:    0o644,
		Merge:   MergeReplace,
	}}, nil
}

// SingleFile returns the sole file of a single-file artifact.
func SingleFile(art domain.Artifact, content domain.Content) ([]byte, error) {
	if len(content.Files) != 1 {
		return nil, fmt.Errorf("artifact %s: expected exactly one file, got %d", art.ID, len(content.Files))
	}
	for _, raw := range content.Files {
		return raw, nil
	}
	return nil, nil
}

// ErrUnsupportedKind is the standard error for a Render call on a kind the
// adapter's Supports already rejects.
func ErrUnsupportedKind(agent domain.AgentID, kind domain.ArtifactKind) error {
	return fmt.Errorf("%s: unsupported artifact kind %q", agent, kind)
}

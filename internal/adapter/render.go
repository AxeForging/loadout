package adapter

import (
	"bytes"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/goccy/go-yaml"
)

// Shared render helpers. Skills follow the Agent Skills standard everywhere,
// so every adapter installs them the same way — only the base directory
// differs. Commands are also portable: agents without a native slash-command
// format receive them as skills.

// SkillFiles maps an artifact's file set under baseDir/<artifact-id>/,
// marking scripts executable. Output is sorted for deterministic plans.
func SkillFiles(baseDir string, art domain.Artifact, content domain.Content) []TargetFile {
	out := make([]TargetFile, 0, len(content.Files))
	for rel, raw := range content.Files {
		mode := fs.FileMode(0o644)
		if strings.HasPrefix(rel, "scripts/") || strings.HasSuffix(rel, ".sh") {
			mode = 0o755
		}
		out = append(out, TargetFile{
			RelPath: path.Join(baseDir, art.ID, rel),
			Content: raw,
			Mode:    mode,
			Merge:   MergeReplace,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out
}

// CommandAsSkill converts a canonical command (frontmatter + prompt body)
// into an Agent Skills SKILL.md document, for agents whose slash commands are
// backed by skills.
func CommandAsSkill(id string, raw []byte) []byte {
	description, body := splitFrontmatter(raw)
	if description == "" {
		description = fmt.Sprintf("Slash command /%s managed by loadout.", id)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "---\nname: %s\ndescription: %s\n---\n\n", id, description)
	buf.Write(bytes.TrimLeft(body, "\n"))
	if !bytes.HasSuffix(buf.Bytes(), []byte("\n")) {
		buf.WriteByte('\n')
	}
	return buf.Bytes()
}

// splitFrontmatter extracts the description field from a YAML frontmatter
// header and returns it with the remaining body. Documents without
// frontmatter return ("", raw).
func splitFrontmatter(raw []byte) (description string, body []byte) {
	const fence = "---\n"
	s := string(raw)
	if !strings.HasPrefix(s, fence) {
		return "", raw
	}
	rest := s[len(fence):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", raw
	}
	header := rest[:end]
	body = []byte(strings.TrimPrefix(rest[end+len("\n---"):], "\n"))
	var fields struct {
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(header), &fields); err != nil {
		return "", raw
	}
	return strings.TrimSpace(fields.Description), body
}

// SettingsFragment converts the per-agent YAML settings file inside a
// settings artifact into a JSON merge fragment. It returns (nil, nil) when
// the artifact carries no file for this agent — the artifact is simply not
// addressed to it.
func SettingsFragment(agentFile string, content domain.Content) ([]byte, error) {
	raw, ok := content.Files[agentFile]
	if !ok {
		return nil, nil
	}
	fragment, err := yaml.YAMLToJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("settings fragment %s: %w", agentFile, err)
	}
	return fragment, nil
}

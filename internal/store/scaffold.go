package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/AxeForging/loadout/internal/domain"
)

// New scaffolds a fresh artifact of the given kind in the store and registers
// it in the manifest.
func (s *Store) New(kind domain.ArtifactKind, name string) (domain.Artifact, error) {
	if err := ValidateName(name); err != nil {
		return domain.Artifact{}, err
	}
	dir, ok := kindDirs[kind]
	if !ok {
		return domain.Artifact{}, fmt.Errorf("unknown artifact kind %q", kind)
	}
	if _, exists := s.Artifact(name); exists {
		return domain.Artifact{}, fmt.Errorf("artifact %q already exists", name)
	}

	var relPath string
	files := map[string]scaffoldFile{}
	switch kind {
	case domain.KindSkill:
		relPath = filepath.Join(dir, name)
		files["SKILL.md"] = scaffoldFile{content: skillTemplate(name)}
	case domain.KindInstruction:
		relPath = filepath.Join(dir, name+".md")
		files[""] = scaffoldFile{content: instructionTemplate(name)}
	case domain.KindCommand:
		relPath = filepath.Join(dir, name+".md")
		files[""] = scaffoldFile{content: commandTemplate(name)}
	case domain.KindHook:
		relPath = filepath.Join(dir, name)
		files["hook.yaml"] = scaffoldFile{content: hookYAMLTemplate()}
		files["run.sh"] = scaffoldFile{content: hookScriptTemplate(name), mode: 0o755}
	case domain.KindMCP:
		relPath = filepath.Join(dir, name)
		files["mcp.yaml"] = scaffoldFile{content: mcpYAMLTemplate(name)}
	case domain.KindSettings:
		return domain.Artifact{}, fmt.Errorf("settings fragments are created per agent: add settings/<agent>.yaml manually")
	}

	for rel, f := range files {
		abs := filepath.Join(s.Root, relPath, rel)
		if rel == "" {
			abs = filepath.Join(s.Root, relPath)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return domain.Artifact{}, err
		}
		mode := f.mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(abs, []byte(f.content), mode); err != nil {
			return domain.Artifact{}, err
		}
	}

	a := domain.Artifact{
		ID:     name,
		Kind:   kind,
		Path:   filepath.ToSlash(relPath),
		Origin: "local",
	}
	if err := s.AddArtifact(a); err != nil {
		return domain.Artifact{}, err
	}
	return a, nil
}

type scaffoldFile struct {
	content string
	mode    os.FileMode
}

// CreateSkill writes a skill with the given SKILL.md content (e.g. an
// AI-drafted one that already passed validation) and registers it.
func (s *Store) CreateSkill(name string, skillMD []byte) (domain.Artifact, error) {
	if err := ValidateName(name); err != nil {
		return domain.Artifact{}, err
	}
	if _, exists := s.Artifact(name); exists {
		return domain.Artifact{}, fmt.Errorf("artifact %q already exists", name)
	}
	relPath := filepath.Join(kindDirs[domain.KindSkill], name)
	abs := filepath.Join(s.Root, relPath, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return domain.Artifact{}, err
	}
	if err := os.WriteFile(abs, skillMD, 0o644); err != nil {
		return domain.Artifact{}, err
	}
	a := domain.Artifact{ID: name, Kind: domain.KindSkill, Path: filepath.ToSlash(relPath), Origin: "local"}
	if err := s.AddArtifact(a); err != nil {
		return domain.Artifact{}, err
	}
	return a, nil
}

// CreateMCP writes an MCP server definition with the given mcp.yaml content
// (e.g. built by the web UI's form) and registers it. The content must pass
// domain.ParseMCPSpec — transport shape and the no-literal-secrets rule —
// before anything touches disk.
func (s *Store) CreateMCP(name string, mcpYAML []byte) (domain.Artifact, error) {
	if err := ValidateName(name); err != nil {
		return domain.Artifact{}, err
	}
	if _, err := domain.ParseMCPSpec(mcpYAML); err != nil {
		return domain.Artifact{}, err
	}
	if _, exists := s.Artifact(name); exists {
		return domain.Artifact{}, fmt.Errorf("artifact %q already exists", name)
	}
	relPath := filepath.Join(kindDirs[domain.KindMCP], name)
	abs := filepath.Join(s.Root, relPath, "mcp.yaml")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return domain.Artifact{}, err
	}
	if err := os.WriteFile(abs, mcpYAML, 0o644); err != nil {
		return domain.Artifact{}, err
	}
	a := domain.Artifact{ID: name, Kind: domain.KindMCP, Path: filepath.ToSlash(relPath), Origin: "local"}
	if err := s.AddArtifact(a); err != nil {
		return domain.Artifact{}, err
	}
	return a, nil
}

func skillTemplate(name string) string {
	return fmt.Sprintf(`---
name: %s
description: TODO — one line stating what this skill does and when the agent should use it.
---

# %s

## When to use

Describe the trigger conditions.

## Steps

1. ...
`, name, name)
}

func instructionTemplate(name string) string {
	return fmt.Sprintf(`# %s

TODO — agent-agnostic instructions. This file is installed into each agent's
global instruction file (CLAUDE.md, AGENTS.md, GEMINI.md) inside a managed
block; content outside the block is never touched.
`, name)
}

func commandTemplate(name string) string {
	return fmt.Sprintf(`---
description: TODO — what /%s does.
---

TODO — the prompt executed when the user invokes /%s.
`, name, name)
}

func hookYAMLTemplate() string {
	return `# Canonical hook definition. Adapters map the event to the agent's native
# hook system (claude-code today; other agents report n/a).
# Events: pre-tool | post-tool | prompt-submit | stop
event: post-tool
matcher: ""
timeoutSeconds: 30
script: run.sh
`
}

func hookScriptTemplate(name string) string {
	return fmt.Sprintf(`#!/usr/bin/env bash
# loadout hook: %s
set -euo pipefail

# Hook payload arrives on stdin as JSON (agent-specific shape).
exit 0
`, name)
}

func mcpYAMLTemplate(name string) string {
	return `# Canonical MCP server definition. Adapters install it into each agent's
# native MCP config (claude-code, gemini-cli, cursor today).
#
# Secrets NEVER go in this file: use ${VAR} placeholders — they are passed
# through verbatim and resolve from the environment at agent runtime.
#
# scope: user    → the agent's global config (default)
# scope: project → the project's config (.mcp.json, .cursor/mcp.json, ...);
#                  synced with: loadout sync --project <dir>
transport: stdio
command: npx
args: ["-y", "@modelcontextprotocol/server-` + name + `"]
env:
  # EXAMPLE_TOKEN: ${EXAMPLE_TOKEN}
scope: user

# Remote server form instead:
# transport: http   # or sse
# url: https://mcp.example.com/v1
# headers:
#   Authorization: Bearer ${EXAMPLE_KEY}
`
}

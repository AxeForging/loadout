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

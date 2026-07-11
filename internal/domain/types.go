// Package domain defines the shared vocabulary of loadout: artifacts, the
// store manifest, sync states and hook specs. It imports no other loadout
// package — everything may import domain.
package domain

import (
	"fmt"

	"github.com/goccy/go-yaml"
)

// AgentID identifies a supported coding agent.
type AgentID string

const (
	AgentClaudeCode  AgentID = "claude-code"
	AgentCodex       AgentID = "codex"
	AgentGeminiCLI   AgentID = "gemini-cli"
	AgentAntigravity AgentID = "antigravity"
	AgentCursor      AgentID = "cursor"
)

// ArtifactKind is the type of a canonical artifact.
type ArtifactKind string

const (
	KindSkill       ArtifactKind = "skill"
	KindInstruction ArtifactKind = "instruction"
	KindCommand     ArtifactKind = "command"
	KindHook        ArtifactKind = "hook"
	KindSettings    ArtifactKind = "settings"
	KindMCP         ArtifactKind = "mcp"
)

// TargetAll in an artifact's targets list means "every registered agent".
const TargetAll = "all"

// APIVersion is the manifest schema version this build reads and writes.
const APIVersion = "loadout/v1"

// Artifact is one entry in the store manifest.
type Artifact struct {
	ID      string       `yaml:"id"                   json:"id"`
	Kind    ArtifactKind `yaml:"kind"                 json:"kind"`
	Path    string       `yaml:"path"                 json:"path"`
	Targets []string     `yaml:"targets,omitempty"     json:"targets,omitempty"`
	Origin  string       `yaml:"origin,omitempty"      json:"origin,omitempty"`
	Tags    []string     `yaml:"tags,omitempty"        json:"tags,omitempty"`
}

// Defaults holds manifest-wide fallbacks.
type Defaults struct {
	Targets          []string `yaml:"targets,omitempty"`
	InstructionOrder []string `yaml:"instructionOrder,omitempty"`
}

// Manifest is the loadout.yaml schema.
type Manifest struct {
	APIVersion  string     `yaml:"apiVersion"`
	Name        string     `yaml:"name"`
	Description string     `yaml:"description,omitempty"`
	Defaults    Defaults   `yaml:"defaults,omitempty"`
	Artifacts   []Artifact `yaml:"artifacts"`
}

// TargetsFor resolves the effective targets of an artifact: the artifact's
// own list, else manifest defaults, else "all".
func (m Manifest) TargetsFor(a Artifact) []string {
	if len(a.Targets) > 0 {
		return a.Targets
	}
	if len(m.Defaults.Targets) > 0 {
		return m.Defaults.Targets
	}
	return []string{TargetAll}
}

// Targeted reports whether an artifact targets the given agent.
func (m Manifest) Targeted(a Artifact, id AgentID) bool {
	for _, t := range m.TargetsFor(a) {
		if t == TargetAll || t == string(id) {
			return true
		}
	}
	return false
}

// Content is the raw file set of one artifact, keyed by path relative to the
// artifact root (a single-file artifact uses its base name as the key).
type Content struct {
	Files map[string][]byte
}

// SyncState classifies one artifact on one agent.
type SyncState string

const (
	StateInSync       SyncState = "in-sync"
	StateMissing      SyncState = "missing"
	StateDriftedLocal SyncState = "drifted-local"
	StateDriftedStore SyncState = "drifted-store"
	StateConflict     SyncState = "conflict"
	StateUnsupported  SyncState = "n/a"
)

// SourceRef pins one external git repository providing artifacts. The lock
// list lives in the store (sources.lock.yaml) so a shared store reproduces
// the same sources everywhere.
type SourceRef struct {
	Name string `yaml:"name"                json:"name"`
	URL  string `yaml:"url"                 json:"url"`
	// Ref is a branch or tag; empty means the remote default branch.
	Ref string `yaml:"ref,omitempty"        json:"ref,omitempty"`
	// Commit is the pinned revision artifacts were imported from.
	Commit string `yaml:"commit"               json:"commit"`
	// Subdir limits discovery to a subdirectory of the repository.
	Subdir string `yaml:"subdir,omitempty"     json:"subdir,omitempty"`
}

// OriginSource formats the artifact origin for a named source.
func OriginSource(name string) string { return "source:" + name }

// HookSpec is the canonical hook definition (hooks/<name>/hook.yaml).
type HookSpec struct {
	// Event is the agent-agnostic hook event, e.g. "pre-tool", "post-tool",
	// "stop", "prompt-submit". Adapters map it to their native event name.
	Event   string `yaml:"event"`
	Matcher string `yaml:"matcher,omitempty"`
	// TimeoutSeconds bounds the hook script's runtime (0 = agent default).
	TimeoutSeconds int `yaml:"timeoutSeconds,omitempty"`
	// Script is the entry point within the artifact directory.
	Script string `yaml:"script,omitempty"`
}

// ParseHookSpec decodes a hook.yaml document and applies defaults.
func ParseHookSpec(raw []byte) (HookSpec, error) {
	var h HookSpec
	if err := yaml.Unmarshal(raw, &h); err != nil {
		return h, fmt.Errorf("parse hook.yaml: %w", err)
	}
	if h.Event == "" {
		return h, fmt.Errorf("hook.yaml: event is required")
	}
	if h.Script == "" {
		h.Script = "run.sh"
	}
	return h, nil
}

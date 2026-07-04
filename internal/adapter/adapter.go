// Package adapter defines the contract between loadout's engine and each
// coding agent. Adapters are pure translators: they resolve the agent's home,
// render canonical artifacts into native target files, and never write to
// disk themselves — the engine is the only writer.
package adapter

import (
	"context"
	"io/fs"

	"github.com/AxeForging/loadout/internal/domain"
)

// Detection reports whether an agent is present on this machine.
type Detection struct {
	Installed bool
	// Home is the agent's config root (e.g. ~/.claude). Target file paths are
	// relative to it.
	Home    string
	Version string
}

// MergeStrategy tells the engine how a target file combines with what is
// already on disk.
type MergeStrategy string

const (
	// MergeReplace overwrites the file with the rendered content.
	MergeReplace MergeStrategy = "replace"
	// MergeBlock upserts a sentinel-marked managed block into a text file,
	// leaving everything outside the markers untouched.
	MergeBlock MergeStrategy = "block"
	// MergeJSON deep-merges a JSON fragment into a JSON document (maps merge
	// recursively, arrays union, fragment scalars win).
	MergeJSON MergeStrategy = "json-merge"
)

// TargetFile is one desired file inside an agent home.
type TargetFile struct {
	RelPath string
	Content []byte
	Mode    fs.FileMode
	Merge   MergeStrategy
}

// Adapter translates canonical artifacts for one agent.
type Adapter interface {
	ID() domain.AgentID
	DisplayName() string
	Detect(ctx context.Context) (Detection, error)
	Supports(kind domain.ArtifactKind) bool
	Render(a domain.Artifact, content domain.Content) ([]TargetFile, error)
	// Owned lists paths under the agent home that loadout may manage — used
	// for orphan detection.
	Owned(home string) ([]string, error)
}

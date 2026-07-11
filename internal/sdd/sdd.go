package sdd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// DefaultDir is where specs (and sdd.yaml) live inside a project.
const DefaultDir = "docs/specs"

// ConfigFile is the per-project SDD configuration, committed next to the
// specs it describes.
const ConfigFile = "sdd.yaml"

// Config selects where checklist state lives for this project's specs.
type Config struct {
	// Backend is "local" (checklist in the spec file) or "github" (one
	// issue per spec).
	Backend string `yaml:"backend"`
	// Repo is owner/name; auto-detected from the origin remote when empty.
	Repo string `yaml:"repo,omitempty"`
	// Labels are applied to created issues (github only).
	Labels []string `yaml:"labels,omitempty"`
	// Issues maps spec name (NNN-slug) → issue number. Written by loadout.
	Issues map[string]int `yaml:"issues,omitempty"`
}

const (
	BackendLocal  = "local"
	BackendGitHub = "github"
)

// LoadConfig reads <dir>/sdd.yaml; a missing file means the local backend.
func LoadConfig(dir string) (Config, error) {
	cfg := Config{Backend: BackendLocal}
	raw, err := os.ReadFile(filepath.Join(dir, ConfigFile))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", ConfigFile, err)
	}
	if cfg.Backend == "" {
		cfg.Backend = BackendLocal
	}
	if cfg.Backend != BackendLocal && cfg.Backend != BackendGitHub {
		return cfg, fmt.Errorf("%s: unknown backend %q (local or github)", ConfigFile, cfg.Backend)
	}
	return cfg, nil
}

// Save writes the config back to <dir>/sdd.yaml.
func (c Config) Save(dir string) error {
	raw, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ConfigFile), raw, 0o644)
}

// SpecInfo identifies one spec.
type SpecInfo struct {
	Number int
	Name   string // NNN-slug (file base name without .md)
	Path   string // spec file path
	Issue  int    // github issue number; 0 = none
}

// Backend is where a spec's checklist lives. Both implementations share the
// same markdown parser/mutator — one code path, two storage locations.
type Backend interface {
	// Steps returns the spec's checklist in document order.
	Steps(ctx context.Context, spec SpecInfo) ([]Step, error)
	// SetStep flips one step; idempotent (changed=false when already there).
	SetStep(ctx context.Context, spec SpecInfo, stepID string, done bool) (changed bool, err error)
}

var specFileRe = regexp.MustCompile(`^(\d{3})-([a-z0-9][a-z0-9-]*)\.md$`)

// ListSpecs enumerates NNN-<slug>.md files in dir (plan companions and
// non-conforming names are ignored), annotated with issue numbers from cfg.
func ListSpecs(dir string, cfg Config) ([]SpecInfo, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var specs []SpecInfo
	for _, en := range ents {
		if en.IsDir() {
			continue
		}
		m := specFileRe.FindStringSubmatch(en.Name())
		if m == nil {
			continue
		}
		num, _ := strconv.Atoi(m[1])
		name := strings.TrimSuffix(en.Name(), ".md")
		specs = append(specs, SpecInfo{
			Number: num,
			Name:   name,
			Path:   filepath.Join(dir, en.Name()),
			Issue:  cfg.Issues[name],
		})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].Number < specs[j].Number })
	return specs, nil
}

// Resolve finds one spec by number ("7", "007") or name prefix
// ("sdd", "007-sdd"). Ambiguous prefixes are an error, never a guess.
func Resolve(specs []SpecInfo, ref string) (SpecInfo, error) {
	if n, err := strconv.Atoi(ref); err == nil {
		for _, s := range specs {
			if s.Number == n {
				return s, nil
			}
		}
		return SpecInfo{}, fmt.Errorf("no spec numbered %03d", n)
	}
	var matches []SpecInfo
	for _, s := range specs {
		if s.Name == ref {
			return s, nil
		}
		if strings.Contains(s.Name, ref) {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return SpecInfo{}, fmt.Errorf("no spec matches %q", ref)
	}
	names := make([]string, len(matches))
	for i, s := range matches {
		names[i] = s.Name
	}
	return SpecInfo{}, fmt.Errorf("%q is ambiguous: %s", ref, strings.Join(names, ", "))
}

// NextNumber returns the lowest free spec number.
func NextNumber(specs []SpecInfo) int {
	next := 1
	for _, s := range specs {
		if s.Number >= next {
			next = s.Number + 1
		}
	}
	return next
}

// FindProjectRoot walks up from dir to the enclosing git repository root.
func FindProjectRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(abs, ".git")); err == nil && info.IsDir() {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", fmt.Errorf("not inside a git repository — loadout spec operates on a project")
		}
		abs = parent
	}
}

package sdd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalBackend keeps checklist state in the spec files themselves.
type LocalBackend struct {
	Dir string // specs directory
}

func (b *LocalBackend) Steps(_ context.Context, spec SpecInfo) ([]Step, error) {
	raw, err := os.ReadFile(spec.Path)
	if err != nil {
		return nil, err
	}
	return ParseSteps(raw)
}

func (b *LocalBackend) SetStep(_ context.Context, spec SpecInfo, stepID string, done bool) (bool, error) {
	raw, err := os.ReadFile(spec.Path)
	if err != nil {
		return false, err
	}
	out, changed, err := SetStep(raw, stepID, done)
	if err != nil {
		return false, err
	}
	if !changed {
		return false, nil
	}
	info, err := os.Stat(spec.Path)
	if err != nil {
		return false, err
	}
	return true, os.WriteFile(spec.Path, out, info.Mode())
}

// CreateSpec scaffolds the next free NNN-<slug>.md from the template. It
// refuses to overwrite and returns the new SpecInfo.
func CreateSpec(dir, slug string) (SpecInfo, error) {
	if !slugRe.MatchString(slug) {
		return SpecInfo{}, fmt.Errorf("invalid spec name %q: use lowercase letters, digits and hyphens", slug)
	}
	cfg, err := LoadConfig(dir)
	if err != nil {
		return SpecInfo{}, err
	}
	specs, err := ListSpecs(dir, cfg)
	if err != nil {
		return SpecInfo{}, err
	}
	for _, s := range specs {
		if s.Name[4:] == slug { // NNN-<slug>
			return SpecInfo{}, fmt.Errorf("spec %s already exists — refusing to overwrite", s.Name)
		}
	}
	num := NextNumber(specs)
	name := fmt.Sprintf("%03d-%s", num, slug)
	path := filepath.Join(dir, name+".md")
	if _, err := os.Stat(path); err == nil {
		return SpecInfo{}, fmt.Errorf("%s already exists — refusing to overwrite", path)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SpecInfo{}, err
	}
	if err := os.WriteFile(path, []byte(specTemplate(num, slug)), 0o644); err != nil {
		return SpecInfo{}, err
	}
	return SpecInfo{Number: num, Name: name, Path: path}, nil
}

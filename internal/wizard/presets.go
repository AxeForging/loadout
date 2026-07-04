package wizard

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

//go:embed presets/*.yaml
var presetFS embed.FS

// Presets lists the embedded preset names.
func Presets() []string {
	entries, err := presetFS.ReadDir("presets")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(e.Name(), ".yaml"))
	}
	sort.Strings(out)
	return out
}

// LoadPreset returns a preset's answers.
func LoadPreset(name string) (Answers, error) {
	raw, err := presetFS.ReadFile("presets/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("unknown preset %q (available: %s)", name, strings.Join(Presets(), ", "))
	}
	var a Answers
	if err := yaml.Unmarshal(raw, &a); err != nil {
		return nil, err
	}
	return a.validate()
}

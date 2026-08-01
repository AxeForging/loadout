package engine

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// state records what loadout installed on THIS machine: for every
// artifact × agent × target file, the hash of the rendered fragment at
// install time. It lives in the platform data dir, never in the store repo.
type state struct {
	// Artifacts: artifact ID → agent ID → target rel path → fragment hash.
	Artifacts map[string]map[string]map[string]string `yaml:"artifacts"`
}

func loadState(path string) (state, error) {
	st := state{Artifacts: map[string]map[string]map[string]string{}}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if err := yaml.Unmarshal(raw, &st); err != nil {
		return st, err
	}
	if st.Artifacts == nil {
		st.Artifacts = map[string]map[string]map[string]string{}
	}
	return st, nil
}

func (s state) save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return store.WriteFileAtomic(path, raw, 0o644)
}

func (s state) get(artifact, agent, relPath string) string {
	return s.Artifacts[artifact][agent][relPath]
}

// setGroup replaces the full file map for one artifact × agent.
func (s state) setGroup(artifact, agent string, files map[string]string) {
	if s.Artifacts[artifact] == nil {
		s.Artifacts[artifact] = map[string]map[string]string{}
	}
	s.Artifacts[artifact][agent] = files
}

// removeGroup drops the install record for one artifact × agent entirely,
// pruning the artifact's top-level entry too once it has no agents left.
func (s state) removeGroup(artifact, agent string) {
	if s.Artifacts[artifact] == nil {
		return
	}
	delete(s.Artifacts[artifact], agent)
	if len(s.Artifacts[artifact]) == 0 {
		delete(s.Artifacts, artifact)
	}
}

// artifactIDs returns the sorted, deduplicated artifact IDs that have at
// least one install record for any agent.
func (s state) artifactIDs() []string {
	out := make([]string, 0, len(s.Artifacts))
	for id, agents := range s.Artifacts {
		if len(agents) > 0 {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

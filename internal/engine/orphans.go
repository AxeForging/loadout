package engine

import (
	"sort"

	"github.com/AxeForging/loadout/internal/domain"
)

// Orphan is an install-state record whose artifact no longer exists in the
// manifest — its files may still sit in an agent home.
type Orphan struct {
	Artifact string
	Agent    domain.AgentID
	Files    []string
}

// OrphanedState lists state records for artifacts that left the store.
func (e *Engine) OrphanedState() ([]Orphan, error) {
	st, err := loadState(e.StatePath)
	if err != nil {
		return nil, err
	}
	var out []Orphan
	for artID, agents := range st.Artifacts {
		if _, exists := e.Store.Artifact(artID); exists {
			continue
		}
		for agent, files := range agents {
			rels := make([]string, 0, len(files))
			for rel := range files {
				rels = append(rels, rel)
			}
			sort.Strings(rels)
			out = append(out, Orphan{Artifact: artID, Agent: domain.AgentID(agent), Files: rels})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Artifact != out[j].Artifact {
			return out[i].Artifact < out[j].Artifact
		}
		return out[i].Agent < out[j].Agent
	})
	return out, nil
}

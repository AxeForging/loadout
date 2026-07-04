// Package engine plans and applies synchronization between the canonical
// store and agent homes. It is the ONLY package that writes into agent homes,
// and it never writes without an explicit confirmation callback returning
// true — that invariant is the product's core promise.
package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

// ErrNotConfirmed is returned by Apply when the confirmation gate declines.
var ErrNotConfirmed = errors.New("not confirmed — no changes applied")

type Engine struct {
	Store     *store.Store
	Registry  *adapter.Registry
	StatePath string
}

// Op is the action Apply would take for one target file.
type Op string

const (
	OpNoop   Op = "noop"
	OpCreate Op = "create"
	OpUpdate Op = "update"
)

// Item is one target file of one artifact on one agent.
type Item struct {
	Agent    domain.AgentID
	Artifact domain.Artifact
	RelPath  string
	AbsPath  string
	Mode     fs.FileMode
	Merge    adapter.MergeStrategy
	// Fragment is the rendered content before merging with disk state.
	Fragment []byte
	// Current is what is on disk now; Exists distinguishes empty from absent.
	Current []byte
	Exists  bool
	// Desired is the full file content after merging Fragment into Current.
	Desired []byte
	Op      Op
	// StateHash is the fragment hash recorded at last install ("" = never).
	StateHash string
}

// Skip records an agent the plan left out entirely.
type Skip struct {
	Agent  domain.AgentID
	Reason string
}

// NotApplicable records an artifact kind an agent does not support.
type NotApplicable struct {
	Agent    domain.AgentID
	Artifact string
}

type Plan struct {
	Items         []Item
	Skips         []Skip
	NotApplicable []NotApplicable
}

// Changes returns the items Apply would write.
func (p *Plan) Changes() []Item {
	var out []Item
	for _, it := range p.Items {
		if it.Op != OpNoop {
			out = append(out, it)
		}
	}
	return out
}

// Filter narrows a plan. Empty slices mean "all".
type Filter struct {
	Agents    []domain.AgentID
	Artifacts []string
}

func (f Filter) wantsAgent(id domain.AgentID) bool {
	if len(f.Agents) == 0 {
		return true
	}
	for _, a := range f.Agents {
		if a == id {
			return true
		}
	}
	return false
}

func (f Filter) wantsArtifact(id string) bool {
	if len(f.Artifacts) == 0 {
		return true
	}
	for _, a := range f.Artifacts {
		if a == id {
			return true
		}
	}
	return false
}

// Plan computes desired state for every targeted artifact × installed agent.
// It performs no writes.
func (e *Engine) Plan(ctx context.Context, f Filter) (*Plan, error) {
	st, err := loadState(e.StatePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	p := &Plan{}
	// pending threads the evolving desired content through plan items that
	// merge into the same file (e.g. several instruction blocks in one
	// CLAUDE.md), so later items build on earlier ones instead of clobbering
	// them at apply time.
	pending := map[string][]byte{}

	for _, ad := range e.Registry.All() {
		if !f.wantsAgent(ad.ID()) {
			continue
		}
		det, err := ad.Detect(ctx)
		if err != nil {
			p.Skips = append(p.Skips, Skip{Agent: ad.ID(), Reason: fmt.Sprintf("detect failed: %v", err)})
			continue
		}
		if !det.Installed {
			p.Skips = append(p.Skips, Skip{Agent: ad.ID(), Reason: "not installed (home directory missing)"})
			continue
		}

		for _, art := range e.Store.Manifest.Artifacts {
			if !f.wantsArtifact(art.ID) || !e.Store.Manifest.Targeted(art, ad.ID()) {
				continue
			}
			if !ad.Supports(art.Kind) {
				p.NotApplicable = append(p.NotApplicable, NotApplicable{Agent: ad.ID(), Artifact: art.ID})
				continue
			}
			content, err := e.Store.Content(art)
			if err != nil {
				return nil, err
			}
			targets, err := ad.Render(art, content)
			if err != nil {
				return nil, fmt.Errorf("render %s for %s: %w", art.ID, ad.ID(), err)
			}
			for _, tf := range targets {
				item, err := e.buildItem(ad.ID(), det.Home, art, tf, st, pending)
				if err != nil {
					return nil, err
				}
				p.Items = append(p.Items, item)
			}
		}
	}
	return p, nil
}

func (e *Engine) buildItem(agent domain.AgentID, home string, art domain.Artifact, tf adapter.TargetFile, st state, pending map[string][]byte) (Item, error) {
	abs := filepath.Join(home, filepath.FromSlash(tf.RelPath))
	current, exists := pending[abs]
	if !exists {
		var err error
		current, err = os.ReadFile(abs)
		exists = err == nil
		if err != nil && !os.IsNotExist(err) {
			return Item{}, fmt.Errorf("read %s: %w", abs, err)
		}
	}
	desired, err := merged(tf.Merge, current, tf.Content)
	if err != nil {
		return Item{}, fmt.Errorf("%s (%s): %w", tf.RelPath, art.ID, err)
	}
	pending[abs] = desired

	op := OpNoop
	switch {
	case !exists:
		op = OpCreate
	case !bytes.Equal(current, desired):
		op = OpUpdate
	}

	return Item{
		Agent:     agent,
		Artifact:  art,
		RelPath:   tf.RelPath,
		AbsPath:   abs,
		Mode:      tf.Mode,
		Merge:     tf.Merge,
		Fragment:  tf.Content,
		Current:   current,
		Exists:    exists,
		Desired:   desired,
		Op:        op,
		StateHash: st.get(art.ID, string(agent), tf.RelPath),
	}, nil
}

// Apply writes every non-noop item, then records install state. The confirm
// callback sees the full plan; nil or false means nothing is written.
func (e *Engine) Apply(ctx context.Context, p *Plan, confirm func(*Plan) bool) (int, error) {
	changes := p.Changes()
	if len(changes) == 0 {
		return 0, nil
	}
	if confirm == nil || !confirm(p) {
		return 0, ErrNotConfirmed
	}

	for _, it := range changes {
		if err := os.MkdirAll(filepath.Dir(it.AbsPath), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(it.AbsPath, it.Desired, it.Mode); err != nil {
			return 0, fmt.Errorf("write %s: %w", it.AbsPath, err)
		}
		// WriteFile only applies the mode on creation; enforce it on updates.
		if err := os.Chmod(it.AbsPath, it.Mode); err != nil {
			return 0, err
		}
	}

	st, err := loadState(e.StatePath)
	if err != nil {
		return 0, err
	}
	for key, files := range groupFragments(p.Items) {
		st.setGroup(key.artifact, key.agent, files)
	}
	if err := st.save(e.StatePath); err != nil {
		return 0, fmt.Errorf("save state: %w", err)
	}
	return len(changes), nil
}

type groupKey struct{ artifact, agent string }

// groupFragments collects fragment hashes per artifact × agent, including
// noop items so unchanged files keep their state records.
func groupFragments(items []Item) map[groupKey]map[string]string {
	groups := map[groupKey]map[string]string{}
	for _, it := range items {
		key := groupKey{artifact: it.Artifact.ID, agent: string(it.Agent)}
		if groups[key] == nil {
			groups[key] = map[string]string{}
		}
		groups[key][it.RelPath] = fragmentHash(it.Merge, it.Fragment)
	}
	return groups
}

// fragmentHash normalizes block fragments (trailing newlines) so they compare
// cleanly against blocks extracted from disk.
func fragmentHash(strategy adapter.MergeStrategy, fragment []byte) string {
	if strategy == adapter.MergeBlock {
		fragment = bytes.TrimRight(fragment, "\n")
	}
	return store.HashBytes(fragment)
}

// StatusRow is the drift classification of one artifact on one agent.
type StatusRow struct {
	Artifact string
	Kind     domain.ArtifactKind
	Agent    domain.AgentID
	State    domain.SyncState
}

// Status aggregates the plan into per-artifact × agent drift states. Rows for
// state records whose artifact left the manifest are reported as orphaned
// with kind "?".
func (e *Engine) Status(ctx context.Context, f Filter) ([]StatusRow, *Plan, error) {
	p, err := e.Plan(ctx, f)
	if err != nil {
		return nil, nil, err
	}

	agg := map[groupKey]domain.SyncState{}
	kinds := map[string]domain.ArtifactKind{}
	for _, it := range p.Items {
		key := groupKey{artifact: it.Artifact.ID, agent: string(it.Agent)}
		kinds[it.Artifact.ID] = it.Artifact.Kind
		s := classify(it)
		if worse(s, agg[key]) {
			agg[key] = s
		} else if _, seen := agg[key]; !seen {
			agg[key] = s
		}
	}
	for _, na := range p.NotApplicable {
		agg[groupKey{artifact: na.Artifact, agent: string(na.Agent)}] = domain.StateUnsupported
	}

	rows := make([]StatusRow, 0, len(agg))
	for key, s := range agg {
		kind, ok := kinds[key.artifact]
		if !ok {
			if a, found := e.Store.Artifact(key.artifact); found {
				kind = a.Kind
			}
		}
		rows = append(rows, StatusRow{Artifact: key.artifact, Kind: kind, Agent: domain.AgentID(key.agent), State: s})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Artifact != rows[j].Artifact {
			return rows[i].Artifact < rows[j].Artifact
		}
		return rows[i].Agent < rows[j].Agent
	})
	return rows, p, nil
}

// severity orders sync states from best to worst so aggregation keeps the
// worst file state per artifact.
var severity = map[domain.SyncState]int{
	domain.StateInSync:       0,
	domain.StateUnsupported:  0,
	domain.StateMissing:      1,
	domain.StateDriftedStore: 2,
	domain.StateDriftedLocal: 3,
	domain.StateConflict:     4,
}

func worse(a, b domain.SyncState) bool { return severity[a] > severity[b] }

// classify determines the drift state of a single target file by comparing
// three hashes: the store's rendered fragment, the install-time state record,
// and what is on disk now.
func classify(it Item) domain.SyncState {
	if it.Exists && bytes.Equal(it.Current, it.Desired) {
		return domain.StateInSync
	}
	if !it.Exists {
		return domain.StateMissing
	}

	fragHash := fragmentHash(it.Merge, it.Fragment)
	if it.StateHash == "" {
		// Never installed by loadout, yet the file exists and differs.
		switch it.Merge {
		case adapter.MergeBlock:
			if id, err := adapter.BlockID(it.Fragment); err == nil {
				if adapter.ExtractBlock(it.Current, id) != nil {
					return domain.StateConflict
				}
			}
			return domain.StateMissing
		case adapter.MergeJSON:
			return domain.StateMissing
		default:
			return domain.StateConflict
		}
	}

	if it.StateHash == fragHash {
		// Store unchanged since install — the disk moved.
		return domain.StateDriftedLocal
	}

	// Store changed since install. If the disk still matches what we
	// installed, only the store moved; otherwise both did.
	switch it.Merge {
	case adapter.MergeReplace:
		if store.HashBytes(it.Current) == it.StateHash {
			return domain.StateDriftedStore
		}
		return domain.StateConflict
	case adapter.MergeBlock:
		id, err := adapter.BlockID(it.Fragment)
		if err == nil {
			block := adapter.ExtractBlock(it.Current, id)
			if block != nil && store.HashBytes(bytes.TrimRight(block, "\n")) == it.StateHash {
				return domain.StateDriftedStore
			}
		}
		return domain.StateConflict
	default:
		return domain.StateDriftedStore
	}
}

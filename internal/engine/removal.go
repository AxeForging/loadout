package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
)

// RemovalWarning documents an artifact × agent × relPath the engine could
// not safely remove automatically, and why. Never silent — always surfaced
// to the caller.
type RemovalWarning struct {
	Artifact string
	Agent    domain.AgentID
	RelPath  string
	Reason   string
}

// StateKey identifies one artifact × agent state.yaml record.
type StateKey struct {
	Artifact string
	Agent    domain.AgentID
}

// RemovalPlan is what removing a set of previously-installed artifacts
// would do. Performs no writes.
type RemovalPlan struct {
	// Items are Apply-able changes: OpDelete for MergeReplace targets (a
	// skill's whole directory collapses into one item per artifact×agent),
	// OpUpdate for MergeBlock block-strip rewrites.
	Items []Item
	// ClearState lists artifact×agent pairs whose state.yaml record should
	// be dropped once ApplySwitch runs — including pairs with zero Items
	// (MergeJSON targets left in place; blocks already absent).
	ClearState []StateKey
	Warnings   []RemovalWarning
	Skips      []Skip
}

// PlanRemoval computes what removing a set of previously-installed artifact
// IDs would do. Unlike Plan, it is sourced from state.yaml (not the live
// manifest) for the set of relative paths, so it still works correctly for
// artifact IDs that have since been removed from the store manifest — it
// just can't determine a merge strategy for those, and falls back to a
// Warning (leave the file, clear the state record) rather than guessing.
// Performs no writes.
func (e *Engine) PlanRemoval(ctx context.Context, artifactIDs []string) (*RemovalPlan, error) {
	st, err := loadState(e.StatePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	rp := &RemovalPlan{}
	skippedAgents := map[domain.AgentID]bool{}
	dirQueued := map[string]bool{}

	for _, artID := range artifactIDs {
		agents := st.Artifacts[artID]
		agentIDs := make([]string, 0, len(agents))
		for a := range agents {
			agentIDs = append(agentIDs, a)
		}
		sort.Strings(agentIDs)

		for _, agentStr := range agentIDs {
			agentID := domain.AgentID(agentStr)
			relPaths := agents[agentStr]
			rp.ClearState = append(rp.ClearState, StateKey{Artifact: artID, Agent: agentID})

			ad, ok := e.Registry.Get(agentID)
			if !ok {
				if !skippedAgents[agentID] {
					rp.Skips = append(rp.Skips, Skip{Agent: agentID, Reason: "agent not registered"})
					skippedAgents[agentID] = true
				}
				continue
			}
			det, err := ad.Detect(ctx)
			if err != nil || !det.Installed {
				if !skippedAgents[agentID] {
					reason := "not installed (home directory missing)"
					if err != nil {
						reason = fmt.Sprintf("detect failed: %v", err)
					}
					rp.Skips = append(rp.Skips, Skip{Agent: agentID, Reason: reason})
					skippedAgents[agentID] = true
				}
				continue
			}

			art, found := e.Store.Artifact(artID)
			rendered := map[string]adapter.TargetFile{}
			if found {
				if content, err := e.Store.Content(art); err == nil {
					if targets, err := ad.Render(art, content); err == nil {
						for _, tf := range targets {
							// State records are keyed the same way Plan keys
							// them, so non-default roots resolve here too.
							rendered[e.stateKey(tf.Root, tf.RelPath)] = tf
						}
					}
				}
			}

			relPathList := make([]string, 0, len(relPaths))
			for rel := range relPaths {
				relPathList = append(relPathList, rel)
			}
			sort.Strings(relPathList)

			for _, relPath := range relPathList {
				tf, ok := rendered[relPath]
				if !ok {
					rp.Warnings = append(rp.Warnings, RemovalWarning{
						Artifact: artID, Agent: agentID, RelPath: relPath,
						Reason: "cannot determine how this file was merged (artifact no longer resolves the same way) — left on disk untouched, state record cleared",
					})
					continue
				}
				base, err := e.resolveBase(tf.Root, det.Home)
				if err != nil {
					rp.Warnings = append(rp.Warnings, RemovalWarning{
						Artifact: artID, Agent: agentID, RelPath: relPath,
						Reason: fmt.Sprintf("cannot resolve target root: %v — left on disk untouched, state record cleared", err),
					})
					continue
				}
				abs, err := joinUnder(base, tf.RelPath)
				if err != nil {
					rp.Warnings = append(rp.Warnings, RemovalWarning{
						Artifact: artID, Agent: agentID, RelPath: relPath,
						Reason: fmt.Sprintf("%v — left on disk untouched, state record cleared", err),
					})
					continue
				}

				switch tf.Merge {
				case adapter.MergeReplace:
					owned, err := ad.Owned(det.Home)
					if err != nil {
						rp.Warnings = append(rp.Warnings, RemovalWarning{Artifact: artID, Agent: agentID, RelPath: relPath, Reason: fmt.Sprintf("could not determine ownership: %v", err)})
						continue
					}
					if !ownsPath(owned, relPath) {
						rp.Warnings = append(rp.Warnings, RemovalWarning{Artifact: artID, Agent: agentID, RelPath: relPath, Reason: "target path is outside this adapter's declared ownership — refusing to delete"})
						continue
					}
					if art.Kind == domain.KindSkill {
						dirAbs := filepath.Join(det.Home, "skills", art.ID)
						key := string(agentID) + "|" + dirAbs
						if dirQueued[key] {
							continue
						}
						dirQueued[key] = true
						rp.Items = append(rp.Items, Item{
							Agent: agentID, Artifact: art, RelPath: filepath.ToSlash(filepath.Join("skills", art.ID)),
							AbsPath: dirAbs, Op: OpDelete, IsDir: true, Merge: adapter.MergeReplace,
						})
						continue
					}
					rp.Items = append(rp.Items, Item{
						Agent: agentID, Artifact: art, RelPath: relPath, AbsPath: abs,
						Op: OpDelete, IsDir: false, Merge: adapter.MergeReplace,
					})

				case adapter.MergeBlock:
					current, err := os.ReadFile(abs)
					if err != nil {
						if os.IsNotExist(err) {
							continue // already absent, nothing to do
						}
						rp.Warnings = append(rp.Warnings, RemovalWarning{Artifact: artID, Agent: agentID, RelPath: relPath, Reason: fmt.Sprintf("could not read file: %v", err)})
						continue
					}
					newContent, blockFound, err := adapter.RemoveBlock(current, art.ID)
					if err != nil {
						rp.Warnings = append(rp.Warnings, RemovalWarning{
							Artifact: artID, Agent: agentID, RelPath: relPath,
							Reason: "block is structurally broken (no end marker) — run 'loadout doctor' to fix, then retry",
						})
						continue
					}
					if !blockFound {
						continue // already absent
					}
					mode := tf.Mode
					if info, statErr := os.Stat(abs); statErr == nil {
						mode = info.Mode()
					}
					rp.Items = append(rp.Items, Item{
						Agent: agentID, Artifact: art, RelPath: relPath, AbsPath: abs,
						Mode: mode, Merge: adapter.MergeBlock, Current: current, Exists: true,
						Desired: newContent, Op: OpUpdate,
					})

				case adapter.MergeJSON:
					rp.Warnings = append(rp.Warnings, RemovalWarning{
						Artifact: artID, Agent: agentID, RelPath: relPath,
						Reason: "json-merge fragment left in place — automatic removal isn't supported yet (state record cleared, artifact stays installed until manually cleaned up)",
					})
				}
			}
		}
	}

	return rp, nil
}

func ownsPath(owned []string, relPath string) bool {
	relPath = filepath.ToSlash(relPath)
	for _, prefix := range owned {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

// ApplySwitch performs an install Plan and a RemovalPlan as one write pass
// followed by one state.yaml save, gated by a single confirm call covering
// both — so a crash mid-apply can never leave state.yaml reflecting only
// half of a switch. Mirrors Apply's own crash-safety shape exactly (write
// everything to agent homes first, save state once at the end): a partial
// write leaves state.yaml stale relative to disk, surfaced as drift by
// status/doctor, and re-running the same switch is always safe since both
// Plan and PlanRemoval re-diff from real disk + state.
func (e *Engine) ApplySwitch(ctx context.Context, install *Plan, removal *RemovalPlan, confirm func(*Plan, *RemovalPlan) bool) (installed, removed int, err error) {
	instChanges := install.Changes()
	remChanges := removal.Items
	if len(instChanges) == 0 && len(remChanges) == 0 {
		return 0, 0, nil
	}
	if confirm == nil || !confirm(install, removal) {
		return 0, 0, ErrNotConfirmed
	}

	// Snapshot everything about to be overwritten or deleted before any write.
	if e.BackupDir != "" {
		both := append(append([]Item{}, instChanges...), remChanges...)
		if _, err := takeSnapshot(e.BackupDir, "switch", both, e.now()); err != nil {
			return 0, 0, fmt.Errorf("snapshot before switch: %w", err)
		}
		if err := pruneSnapshots(e.BackupDir, e.KeepBackups); err != nil {
			return 0, 0, err
		}
	}

	for _, it := range instChanges {
		if err := writeItem(it); err != nil {
			return 0, 0, err
		}
	}
	for _, it := range remChanges {
		switch it.Op {
		case OpDelete:
			if it.IsDir {
				if err := os.RemoveAll(it.AbsPath); err != nil {
					return 0, 0, err
				}
			} else if err := os.Remove(it.AbsPath); err != nil && !os.IsNotExist(err) {
				return 0, 0, err
			}
		default: // OpUpdate: block-strip rewrite
			if err := writeItem(it); err != nil {
				return 0, 0, err
			}
		}
	}

	st, err := loadState(e.StatePath)
	if err != nil {
		return 0, 0, err
	}
	for key, files := range groupFragments(install.stateItems()) {
		st.setGroup(key.artifact, key.agent, files)
	}
	for _, k := range removal.ClearState {
		st.removeGroup(k.Artifact, string(k.Agent))
	}
	if err := st.save(e.StatePath); err != nil {
		return 0, 0, fmt.Errorf("save state: %w", err)
	}
	return len(instChanges), len(remChanges), nil
}

// writeItem is the MkdirAll+WriteFile+Chmod sequence Apply and ApplySwitch
// both use for non-delete items.
func writeItem(it Item) error {
	if err := os.MkdirAll(filepath.Dir(it.AbsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(it.AbsPath, it.Desired, it.Mode); err != nil {
		return fmt.Errorf("write %s: %w", it.AbsPath, err)
	}
	// WriteFile only applies the mode on creation; enforce it on updates.
	return os.Chmod(it.AbsPath, it.Mode)
}

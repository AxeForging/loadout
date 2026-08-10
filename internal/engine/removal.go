package engine

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
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

// RemovalOptions tunes PlanRemoval.
type RemovalOptions struct {
	// Force also removes targets whose on-disk content no longer matches what
	// loadout installed (drifted) — mirrors sync's --force conflict semantics.
	// Without it, drifted targets are left in place with a warning.
	Force bool
	// RemoveStore adds the artifact's store files (manifest + content) to the
	// plan's StoreFiles so the pre-write snapshot captures them; the actual
	// store deletion stays with store.RemoveArtifact — the engine never writes
	// into the store.
	RemoveStore bool
}

// StoreFile is a store-side path a removal will delete or rewrite (the
// manifest). Captured in the pre-write snapshot so `loadout restore` can
// resurrect a removed artifact even in a store that isn't git-tracked.
type StoreFile struct {
	RelPath string
	AbsPath string
	IsDir   bool
	Op      Op
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
	// StoreFiles lists store-side files the caller intends to delete after
	// ApplyRemoval (populated by RemovalOptions.RemoveStore). They are
	// snapshotted before any write but never written by the engine.
	StoreFiles []StoreFile
	// RetainState maps artifact×agent groups to the install records that must
	// SURVIVE the state clear: targets skipped because they drifted. Keeping
	// them is what lets a later `rm --force` still recognize the file as
	// loadout-installed instead of having forgotten it.
	RetainState map[StateKey]map[string]string
	Warnings    []RemovalWarning
	Skips       []Skip
}

// PlanRemoval computes what removing a set of previously-installed artifact
// IDs would do. Unlike Plan, it is sourced from state.yaml (not the live
// manifest) for the set of relative paths, so it still works correctly for
// artifact IDs that have since been removed from the store manifest — it
// just can't determine a merge strategy for those, and falls back to a
// Warning (leave the file, clear the state record) rather than guessing.
// Performs no writes.
func (e *Engine) PlanRemoval(ctx context.Context, artifactIDs []string, opts RemovalOptions) (*RemovalPlan, error) {
	st, err := loadState(e.StatePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	rp := &RemovalPlan{RetainState: map[StateKey]map[string]string{}}
	skippedAgents := map[domain.AgentID]bool{}
	dirQueued := map[string]bool{}
	storeTouched := false

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
			skey := StateKey{Artifact: artID, Agent: agentID}
			rp.ClearState = append(rp.ClearState, skey)
			// retain keeps a drift-skipped target's install record alive so a
			// later `rm --force` still recognizes it as loadout-installed.
			retain := func(rels ...string) {
				if rp.RetainState[skey] == nil {
					rp.RetainState[skey] = map[string]string{}
				}
				for _, r := range rels {
					rp.RetainState[skey][r] = relPaths[r]
				}
			}

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
						if !opts.Force {
							if drifted, why := skillDirDrifted(dirAbs, det.Home, relPaths); drifted {
								rp.Warnings = append(rp.Warnings, RemovalWarning{
									Artifact: artID, Agent: agentID, RelPath: filepath.ToSlash(filepath.Join("skills", art.ID)),
									Reason: fmt.Sprintf("%s — left on disk (re-run with --force to remove; a snapshot is taken first)", why),
								})
								prefix := filepath.ToSlash(filepath.Join("skills", art.ID)) + "/"
								for rel := range relPaths {
									if strings.HasPrefix(rel, prefix) {
										retain(rel)
									}
								}
								continue
							}
						}
						rp.Items = append(rp.Items, Item{
							Agent: agentID, Artifact: art, RelPath: filepath.ToSlash(filepath.Join("skills", art.ID)),
							AbsPath: dirAbs, Op: OpDelete, IsDir: true, Merge: adapter.MergeReplace,
						})
						continue
					}
					if !opts.Force {
						if current, err := os.ReadFile(abs); err == nil && store.HashBytes(current) != relPaths[relPath] {
							rp.Warnings = append(rp.Warnings, RemovalWarning{
								Artifact: artID, Agent: agentID, RelPath: relPath,
								Reason: "content changed since loadout installed it — left on disk (re-run with --force to remove; a snapshot is taken first)",
							})
							retain(relPath)
							continue
						}
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
					if !opts.Force {
						block := adapter.ExtractBlock(current, art.ID)
						if block != nil && store.HashBytes(bytes.TrimRight(block, "\n")) != relPaths[relPath] {
							rp.Warnings = append(rp.Warnings, RemovalWarning{
								Artifact: artID, Agent: agentID, RelPath: relPath,
								Reason: "managed block content changed since loadout installed it — left in place (re-run with --force to remove; a snapshot is taken first)",
							})
							retain(relPath)
							continue
						}
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
					current, err := os.ReadFile(abs)
					if err != nil {
						if os.IsNotExist(err) {
							continue // file gone — nothing of ours left
						}
						rp.Warnings = append(rp.Warnings, RemovalWarning{Artifact: artID, Agent: agentID, RelPath: relPath, Reason: fmt.Sprintf("could not read file: %v", err)})
						continue
					}
					newContent, changed, drifted, err := jsonRemove(current, tf.Content, opts.Force)
					if err != nil {
						rp.Warnings = append(rp.Warnings, RemovalWarning{
							Artifact: artID, Agent: agentID, RelPath: relPath,
							Reason: fmt.Sprintf("could not compute JSON removal: %v — left untouched", err),
						})
						continue
					}
					for _, p := range drifted {
						reason := fmt.Sprintf("value at %q differs from what loadout installed — left in place (re-run with --force to remove; a snapshot is taken first)", p)
						if opts.Force {
							reason = fmt.Sprintf("installed entry at %q was edited or already removed by hand — cannot be identified anymore, left in place", p)
						}
						rp.Warnings = append(rp.Warnings, RemovalWarning{Artifact: artID, Agent: agentID, RelPath: relPath, Reason: reason})
					}
					if !changed {
						if len(drifted) > 0 && !opts.Force {
							retain(relPath)
						}
						continue // nothing of ours present (or drift kept it whole)
					}
					mode := tf.Mode
					if info, statErr := os.Stat(abs); statErr == nil {
						mode = info.Mode()
					}
					rp.Items = append(rp.Items, Item{
						Agent: agentID, Artifact: art, RelPath: tf.RelPath, AbsPath: abs,
						Mode: mode, Merge: adapter.MergeJSON, Current: current, Exists: true,
						Desired: newContent, Op: OpUpdate, Root: tf.Root, StateKey: relPath,
					})
				}
			}
		}

		if opts.RemoveStore {
			if art, found := e.Store.Artifact(artID); found {
				storeTouched = true
				abs := filepath.Join(e.Store.Root, art.Path)
				if info, err := os.Stat(abs); err == nil {
					rp.StoreFiles = append(rp.StoreFiles, StoreFile{
						RelPath: filepath.ToSlash(art.Path), AbsPath: abs,
						IsDir: info.IsDir(), Op: OpDelete,
					})
				}
			}
		}
	}

	// Removing any manifest entry rewrites loadout.yaml — capture it too so a
	// restore brings the entry back, not just the content files.
	if storeTouched {
		rp.StoreFiles = append(rp.StoreFiles, StoreFile{
			RelPath: store.ManifestFile,
			AbsPath: filepath.Join(e.Store.Root, store.ManifestFile),
			Op:      OpUpdate,
		})
	}

	return rp, nil
}

// skillDirDrifted reports whether a skill's installed directory differs from
// what loadout recorded at install time: any edited file, or any file loadout
// did not put there. A missing directory is not drift — there is nothing left
// to protect.
func skillDirDrifted(dirAbs, home string, recorded map[string]string) (bool, string) {
	var reason string
	err := filepath.WalkDir(dirAbs, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(home, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		want, ok := recorded[rel]
		if !ok {
			reason = fmt.Sprintf("%s was not installed by loadout", rel)
			return fs.SkipAll
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if store.HashBytes(raw) != want {
			reason = fmt.Sprintf("%s changed since loadout installed it", rel)
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return true, fmt.Sprintf("could not verify directory contents: %v", err)
	}
	return reason != "", reason
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
		if err := removeItem(it); err != nil {
			return 0, 0, err
		}
	}

	st, err := loadState(e.StatePath)
	if err != nil {
		return 0, 0, err
	}
	for key, files := range groupFragments(install.stateItems()) {
		st.setGroup(key.artifact, key.agent, files)
	}
	clearState(st, removal)
	if err := st.save(e.StatePath); err != nil {
		return 0, 0, fmt.Errorf("save state: %w", err)
	}
	return len(instChanges), len(remChanges), nil
}

// removeItem executes one removal item: OpDelete unlinks (idempotently — an
// already-absent path is a safe no-op), anything else is a strip rewrite.
func removeItem(it Item) error {
	switch it.Op {
	case OpDelete:
		if it.IsDir {
			return os.RemoveAll(it.AbsPath)
		}
		if err := os.Remove(it.AbsPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	default: // OpUpdate: block-strip or JSON-removal rewrite
		return writeItem(it)
	}
}

// ApplyRemoval performs a RemovalPlan as one confirmed write pass: snapshot
// (trigger "rm", including the store files the caller is about to delete),
// remove/rewrite every item, clear the state records, save state once. It
// honors the same confirmation invariant as Apply: nil or false confirm
// writes nothing. Store deletion itself stays with store.RemoveArtifact,
// called by the CLI afterwards — a crash in between leaves the artifact in
// the store, and re-running rm converges.
func (e *Engine) ApplyRemoval(ctx context.Context, rp *RemovalPlan, confirm func(*RemovalPlan) bool) (int, error) {
	if len(rp.Items) == 0 && len(rp.ClearState) == 0 && len(rp.StoreFiles) == 0 {
		return 0, nil
	}
	if confirm == nil || !confirm(rp) {
		return 0, ErrNotConfirmed
	}

	if e.BackupDir != "" {
		// Store files ride under the same 'store' pseudo-agent that source
		// updates and bundle imports use (spec 008), so restore round-trips
		// through one consistent snapshot layout.
		targets := itemSnapTargets(rp.Items)
		for _, sf := range rp.StoreFiles {
			targets = append(targets, snapTarget{
				agent: SnapshotAgent, relPath: sf.RelPath, absPath: sf.AbsPath,
				op: sf.Op, mode: 0o644, isDir: sf.IsDir, root: adapter.RootAgentHome,
			})
		}
		if _, err := captureSnapshot(e.BackupDir, "rm", targets, e.now()); err != nil {
			return 0, fmt.Errorf("snapshot before removal: %w", err)
		}
		if err := pruneSnapshots(e.BackupDir, e.KeepBackups); err != nil {
			return 0, err
		}
	}

	for _, it := range rp.Items {
		if err := removeItem(it); err != nil {
			return 0, err
		}
	}

	st, err := loadState(e.StatePath)
	if err != nil {
		return 0, err
	}
	clearState(st, rp)
	if err := st.save(e.StatePath); err != nil {
		return 0, fmt.Errorf("save state: %w", err)
	}
	return len(rp.Items), nil
}

// clearState drops the plan's artifact×agent install records, except for
// drift-skipped targets, whose records shrink to the retained subset instead
// of vanishing — forgetting them would make a later --force removal treat the
// file as never-installed.
func clearState(st state, rp *RemovalPlan) {
	for _, k := range rp.ClearState {
		if kept := rp.RetainState[k]; len(kept) > 0 {
			st.setGroup(k.Artifact, string(k.Agent), kept)
			continue
		}
		st.removeGroup(k.Artifact, string(k.Agent))
	}
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

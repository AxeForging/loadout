package engine

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// Snapshots capture the ORIGINAL content of every agent-home file an Apply or
// switch is about to overwrite or delete, before the write happens, so an
// accidental clobber is always recoverable via `loadout restore`. They live in
// the machine-local data dir (never the shared store) alongside state.yaml.

const snapshotIDFormat = "20060102-150405"

// SnapshotEntry is one captured file (or directory) inside a snapshot.
type SnapshotEntry struct {
	Agent   domain.AgentID `yaml:"agent"`
	RelPath string         `yaml:"relPath"`
	AbsPath string         `yaml:"absPath"`
	Op      Op             `yaml:"op"`
	Mode    fs.FileMode    `yaml:"mode"`
	IsDir   bool           `yaml:"isDir"`
	// Root disambiguates the backup copy location for targets outside the
	// agent home; empty (agent home) keeps the historical layout, so old
	// snapshots load and restore unchanged.
	Root adapter.TargetRoot `yaml:"root,omitempty"`
}

// Snapshot is a timestamped set of pre-write file backups.
type Snapshot struct {
	ID        string          `yaml:"id"`
	CreatedAt time.Time       `yaml:"createdAt"`
	Trigger   string          `yaml:"trigger"` // sync | switch | restore
	Entries   []SnapshotEntry `yaml:"entries"`

	dir string // on-disk snapshot directory; set by List/Load, not serialized
}

// snapTarget is a single path a write pass is about to modify; captureSnapshot
// backs up its current on-disk content if any exists.
type snapTarget struct {
	agent   domain.AgentID
	relPath string
	absPath string
	op      Op
	mode    fs.FileMode
	isDir   bool
	root    adapter.TargetRoot
}

// snapshotRel is where a target's backup copy lives inside the snapshot's
// files/ tree. Non-default roots get their own segment so an agent-home file
// and a project/user-home file with the same rel path never collide.
func snapshotRel(root adapter.TargetRoot, relPath string) string {
	if root == adapter.RootAgentHome {
		return filepath.FromSlash(relPath)
	}
	return filepath.Join("__"+string(root)+"__", filepath.FromSlash(relPath))
}

// takeSnapshot backs up the pre-write content of every item that would
// overwrite or delete existing content. Pure creations (nothing on disk yet)
// are ignored. Returns (nil, nil) when there is nothing to lose.
func takeSnapshot(backupDir, trigger string, items []Item, now time.Time) (*Snapshot, error) {
	targets := make([]snapTarget, 0, len(items))
	for _, it := range items {
		if it.Op != OpUpdate && it.Op != OpDelete {
			continue
		}
		targets = append(targets, snapTarget{
			agent: it.Agent, relPath: it.RelPath, absPath: it.AbsPath,
			op: it.Op, mode: it.Mode, isDir: it.IsDir, root: it.Root,
		})
	}
	return captureSnapshot(backupDir, trigger, targets, now)
}

// captureSnapshot copies each target's current content into a fresh snapshot
// directory. Targets whose path does not exist on disk are silently skipped.
func captureSnapshot(backupDir, trigger string, targets []snapTarget, now time.Time) (*Snapshot, error) {
	snapDir, id, err := reserveSnapshotDir(backupDir, now)
	if err != nil {
		return nil, err
	}

	var entries []SnapshotEntry
	for _, tgt := range targets {
		info, err := os.Stat(tgt.absPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // nothing on disk to preserve
			}
			return nil, fmt.Errorf("snapshot stat %s: %w", tgt.absPath, err)
		}
		dst := filepath.Join(snapDir, "files", string(tgt.agent), snapshotRel(tgt.root, tgt.relPath))
		isDir := tgt.isDir || info.IsDir()
		if isDir {
			if err := store.CopyTree(tgt.absPath, dst); err != nil {
				return nil, fmt.Errorf("snapshot copy %s: %w", tgt.absPath, err)
			}
		} else {
			if err := copyFilePreserving(tgt.absPath, dst, info.Mode()); err != nil {
				return nil, fmt.Errorf("snapshot copy %s: %w", tgt.absPath, err)
			}
		}
		mode := tgt.mode
		if !isDir {
			mode = info.Mode()
		}
		entries = append(entries, SnapshotEntry{
			Agent: tgt.agent, RelPath: tgt.relPath, AbsPath: tgt.absPath,
			Op: tgt.op, Mode: mode, IsDir: isDir, Root: tgt.root,
		})
	}

	if len(entries) == 0 {
		// Nothing was worth preserving — leave no empty snapshot behind.
		_ = os.RemoveAll(snapDir)
		return nil, nil
	}

	snap := &Snapshot{ID: id, CreatedAt: now.UTC(), Trigger: trigger, Entries: entries, dir: snapDir}
	if err := writeSnapshotMeta(snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// SnapshotAgent is the pseudo agent ID under which store-file backups live
// inside a snapshot's files/ tree (spec 008: source updates and bundle
// imports snapshot the store the same way syncs snapshot agent homes).
const SnapshotAgent = domain.AgentID("store")

// SnapshotStorePaths backs up the given store-relative paths (files or whole
// artifact directories) before a store mutation overwrites them, then prunes.
// Paths that do not exist are skipped, so a pure addition yields (nil, nil)
// and no snapshot. Callers must fail closed: no snapshot result on error, no
// write.
func SnapshotStorePaths(backupDir, trigger, storeRoot string, relPaths []string, keep int, now time.Time) (*Snapshot, error) {
	targets := make([]snapTarget, 0, len(relPaths))
	for _, rel := range relPaths {
		abs := filepath.Join(storeRoot, filepath.FromSlash(rel))
		info, err := os.Stat(abs)
		if err != nil {
			continue // nothing on disk to preserve; captureSnapshot re-checks
		}
		targets = append(targets, snapTarget{
			agent: SnapshotAgent, relPath: rel, absPath: abs,
			op: OpUpdate, mode: info.Mode(), isDir: info.IsDir(),
			root: adapter.RootAgentHome,
		})
	}
	snap, err := captureSnapshot(backupDir, trigger, targets, now)
	if err != nil {
		return nil, err
	}
	if err := pruneSnapshots(backupDir, keep); err != nil {
		return nil, err
	}
	return snap, nil
}

// reserveSnapshotDir creates a unique snapshot directory named for the current
// time, disambiguating same-second collisions with a numeric suffix.
func reserveSnapshotDir(backupDir string, now time.Time) (dir, id string, err error) {
	base := now.UTC().Format(snapshotIDFormat)
	for i := 1; ; i++ {
		id = base
		if i > 1 {
			id = fmt.Sprintf("%s-%d", base, i)
		}
		dir = filepath.Join(backupDir, id)
		if err := os.Mkdir(dir, 0o755); err == nil {
			return dir, id, nil
		} else if !os.IsExist(err) {
			if mkErr := os.MkdirAll(backupDir, 0o755); mkErr != nil {
				return "", "", mkErr
			}
			if err := os.Mkdir(dir, 0o755); err == nil {
				return dir, id, nil
			} else if !os.IsExist(err) {
				return "", "", err
			}
		}
	}
}

func writeSnapshotMeta(snap *Snapshot) error {
	raw, err := yaml.Marshal(snap)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(snap.dir, "snapshot.yaml"), raw, 0o644)
}

// pruneSnapshots keeps the newest `keep` snapshots and removes the rest. A
// non-positive keep disables pruning (never deletes).
func pruneSnapshots(backupDir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	snaps, err := ListSnapshots(backupDir)
	if err != nil {
		return err
	}
	for _, s := range snaps[min(keep, len(snaps)):] {
		if err := os.RemoveAll(s.dir); err != nil {
			return err
		}
	}
	return nil
}

// ListSnapshots returns saved snapshots, newest first. A missing backup dir is
// not an error — it just means nothing has been snapshotted yet.
func ListSnapshots(backupDir string) ([]Snapshot, error) {
	ents, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snaps []Snapshot
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		snap, err := LoadSnapshot(backupDir, e.Name())
		if err != nil {
			continue // ignore unreadable/foreign directories
		}
		snaps = append(snaps, *snap)
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].ID > snaps[j].ID })
	return snaps, nil
}

// LoadSnapshot reads one snapshot's metadata by ID.
func LoadSnapshot(backupDir, id string) (*Snapshot, error) {
	dir := filepath.Join(backupDir, id)
	raw, err := os.ReadFile(filepath.Join(dir, "snapshot.yaml"))
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := yaml.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	snap.dir = dir
	return &snap, nil
}

// ApplyRestore writes a snapshot's captured files back to the agent homes they
// came from, gated by the same confirmation invariant as Apply (nil or false
// confirm writes nothing). It first snapshots the current state (trigger
// "restore") so a restore is itself undoable, and deliberately never touches
// state.yaml — after a restore, status honestly reports whatever drift results.
func (e *Engine) ApplyRestore(ctx context.Context, snap *Snapshot, confirm func(*Snapshot) bool) (int, error) {
	if snap == nil || len(snap.Entries) == 0 {
		return 0, nil
	}
	if confirm == nil || !confirm(snap) {
		return 0, ErrNotConfirmed
	}

	if e.BackupDir != "" {
		targets := make([]snapTarget, 0, len(snap.Entries))
		for _, en := range snap.Entries {
			targets = append(targets, snapTarget{
				agent: en.Agent, relPath: en.RelPath, absPath: en.AbsPath,
				op: OpUpdate, mode: en.Mode, isDir: en.IsDir, root: en.Root,
			})
		}
		if _, err := captureSnapshot(e.BackupDir, "restore", targets, e.now()); err != nil {
			return 0, fmt.Errorf("pre-restore snapshot: %w", err)
		}
	}

	srcRoot := filepath.Join(snap.dir, "files")
	for _, en := range snap.Entries {
		src := filepath.Join(srcRoot, string(en.Agent), snapshotRel(en.Root, en.RelPath))
		if en.IsDir {
			if err := os.RemoveAll(en.AbsPath); err != nil {
				return 0, err
			}
			if err := store.CopyTree(src, en.AbsPath); err != nil {
				return 0, fmt.Errorf("restore %s: %w", en.AbsPath, err)
			}
			continue
		}
		if err := copyFilePreserving(src, en.AbsPath, en.Mode); err != nil {
			return 0, fmt.Errorf("restore %s: %w", en.AbsPath, err)
		}
	}
	return len(snap.Entries), nil
}

// copyFilePreserving copies a single file, creating parent directories and
// applying the given mode.
func copyFilePreserving(src, dst string, mode fs.FileMode) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, raw, mode); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}

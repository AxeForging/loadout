package sdd

import "fmt"

// StateDiff is one step whose done-state differs between the local spec
// file and the GitHub issue.
type StateDiff struct {
	ID         string
	LocalDone  bool
	RemoteDone bool
}

// SyncReport is the three-way comparison between a spec file's checklist
// and its issue's.
type SyncReport struct {
	// StructureChanged is true when the step set or wording differs —
	// structure always flows local → github (the file is authored there).
	StructureChanged bool
	// StateDiffs lists check-state divergence. loadout never guesses a
	// direction: the caller resolves with --from-local or --from-github.
	StateDiffs []StateDiff
}

// InSync reports whether nothing needs reconciling.
func (r SyncReport) InSync() bool {
	return !r.StructureChanged && len(r.StateDiffs) == 0
}

// Compare diffs local (spec file) steps against remote (issue) steps.
func Compare(local, remote []Step) SyncReport {
	var rep SyncReport
	remoteByID := map[string]Step{}
	for _, s := range remote {
		remoteByID[s.ID] = s
	}
	if len(local) != len(remote) {
		rep.StructureChanged = true
	}
	for i, l := range local {
		r, ok := remoteByID[l.ID]
		if !ok || l.Text != r.Text || (!rep.StructureChanged && remote[i].ID != l.ID) {
			rep.StructureChanged = true
			if !ok {
				continue
			}
		}
		if l.Done != r.Done {
			rep.StateDiffs = append(rep.StateDiffs, StateDiff{ID: l.ID, LocalDone: l.Done, RemoteDone: r.Done})
		}
	}
	return rep
}

// Describe renders one divergence line for the user.
func (d StateDiff) Describe() string {
	mark := func(done bool) string {
		if done {
			return "[x]"
		}
		return "[ ]"
	}
	return fmt.Sprintf("%s: local %s vs github %s", d.ID, mark(d.LocalDone), mark(d.RemoteDone))
}

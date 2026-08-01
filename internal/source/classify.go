package source

// updateState is the three-way classification of one source-owned skill
// during an update: baseline (content as last imported), store (content now,
// possibly user-edited) and upstream (content at the new commit).
type updateState int

const (
	// stateIdentical: store already matches upstream — nothing to do.
	stateIdentical updateState = iota
	// stateUpstreamChanged: the user never touched it, upstream moved — safe
	// to update.
	stateUpstreamChanged
	// stateLocallyEdited: the user edited it, upstream did not move — never
	// written.
	stateLocallyEdited
	// stateConflict: both moved — written only with an explicit force-discard.
	stateConflict
	// stateLegacy: no baseline is known (pre-008 store and the pinned commit
	// is unreachable) — degrades to the legacy two-way "update" behavior,
	// labeled so the user knows detection was impossible.
	stateLegacy
)

// classifyUpdate decides an update action from the three content hashes.
// An empty baseline means "unknown" (legacy store, pin unreachable).
func classifyUpdate(baseline, store, upstream string) updateState {
	if store == upstream {
		return stateIdentical
	}
	if baseline == "" {
		return stateLegacy
	}
	if baseline == store {
		return stateUpstreamChanged
	}
	if baseline == upstream {
		return stateLocallyEdited
	}
	return stateConflict
}

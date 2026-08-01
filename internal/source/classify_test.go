package source

import "testing"

// TestClassifyUpdate_AllHashPermutations is exhaustive over the meaningful
// equality relations between baseline (B), store (S) and upstream (U).
func TestClassifyUpdate_AllHashPermutations(t *testing.T) {
	tests := []struct {
		name                      string
		baseline, store, upstream string
		want                      updateState
	}{
		{"all equal — nothing changed", "h1", "h1", "h1", stateIdentical},
		{"store equals upstream, baseline stale (edited to match upstream)", "h1", "h2", "h2", stateIdentical},
		{"only upstream moved", "h1", "h1", "h2", stateUpstreamChanged},
		{"only store moved (local edit)", "h1", "h2", "h1", stateLocallyEdited},
		{"all three differ (conflict)", "h1", "h2", "h3", stateConflict},
		{"no baseline, store differs from upstream", "", "h1", "h2", stateLegacy},
		{"no baseline, store equals upstream", "", "h1", "h1", stateIdentical},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyUpdate(tt.baseline, tt.store, tt.upstream); got != tt.want {
				t.Errorf("classifyUpdate(%q,%q,%q) = %v, want %v", tt.baseline, tt.store, tt.upstream, got, tt.want)
			}
		})
	}
}

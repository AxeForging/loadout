package engine

import (
	"github.com/pmezard/go-difflib/difflib"
)

// UnifiedDiff renders the current → desired change for one item.
func UnifiedDiff(it Item) (string, error) {
	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(it.Current)),
		B:        difflib.SplitLines(string(it.Desired)),
		FromFile: it.AbsPath,
		ToFile:   it.AbsPath + " (desired)",
		Context:  3,
	}
	return difflib.GetUnifiedDiffString(diff)
}

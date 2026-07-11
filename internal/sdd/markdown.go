// Package sdd manages spec-driven-development checklists: numbered specs
// with stable step IDs whose completion state lives either in the spec file
// itself (local backend) or in one GitHub issue per spec (github backend).
// It never touches the canonical store, agent homes, or the sync engine —
// specs are project files.
package sdd

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// Step is one checklist item of a spec.
type Step struct {
	ID   string
	Text string
	Done bool
}

// stepLineRe matches a checklist line carrying a stable step marker:
//
//   - [ ] wire the adapter <!-- sdd:wire-adapter -->
//
// at any list depth, with - or * bullets, tolerating [x]/[X] and trailing
// whitespace. Group 1 = prefix up to the checkbox char, 2 = checkbox char,
// 3 = text, 4 = step ID.
var stepLineRe = regexp.MustCompile(`^(\s*[-*] \[)([ xX])(\] .*?)<!-- sdd:([a-z0-9][a-z0-9-]*) -->\s*$`)

// fenceRe matches the opening/closing of fenced code blocks; checkboxes
// inside fences are content, not steps.
var fenceRe = regexp.MustCompile("^\\s*(```|~~~)")

// ParseSteps extracts the marked checklist steps of a spec document in
// order. Duplicate step IDs are an error — check/uncheck would be ambiguous.
func ParseSteps(doc []byte) ([]Step, error) {
	var steps []Step
	seen := map[string]bool{}
	inFence := false
	for _, line := range splitLines(doc) {
		if fenceRe.Match(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := stepLineRe.FindSubmatch(line)
		if m == nil {
			continue
		}
		id := string(m[4])
		if seen[id] {
			return nil, fmt.Errorf("duplicate step id %q — step ids must be unique within a spec", id)
		}
		seen[id] = true
		text := strings.TrimSpace(strings.TrimPrefix(string(m[3]), "] "))
		steps = append(steps, Step{ID: id, Text: text, Done: m[2][0] == 'x' || m[2][0] == 'X'})
	}
	return steps, nil
}

// SetStep toggles exactly one step's checkbox character, preserving every
// other byte of the document (line endings included). Setting a step to the
// state it already has is a no-op success (changed=false, doc returned as
// given). An unknown ID errors with the list of valid IDs.
func SetStep(doc []byte, id string, done bool) (out []byte, changed bool, err error) {
	steps, err := ParseSteps(doc)
	if err != nil {
		return nil, false, err
	}
	valid := make([]string, 0, len(steps))
	var target *Step
	for i := range steps {
		valid = append(valid, steps[i].ID)
		if steps[i].ID == id {
			target = &steps[i]
		}
	}
	if target == nil {
		if len(valid) == 0 {
			return nil, false, fmt.Errorf("unknown step %q — this spec has no marked steps (add `<!-- sdd:<id> -->` markers to checklist lines)", id)
		}
		return nil, false, fmt.Errorf("unknown step %q — valid steps: %s", id, strings.Join(valid, ", "))
	}
	if target.Done == done {
		return doc, false, nil
	}

	box := byte(' ')
	if done {
		box = 'x'
	}
	inFence := false
	lines := splitLines(doc)
	for i, line := range lines {
		if fenceRe.Match(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		m := stepLineRe.FindSubmatchIndex(line)
		if m == nil {
			continue
		}
		if string(line[m[8]:m[9]]) != id {
			continue
		}
		edited := append([]byte{}, line...)
		edited[m[4]] = box // m[4] = start of the single checkbox character
		lines[i] = edited
		return bytes.Join(lines, nil), true, nil
	}
	return nil, false, fmt.Errorf("unknown step %q — valid steps: %s", id, strings.Join(valid, ", "))
}

// splitLines splits after every \n, keeping the delimiters (and any \r
// before them) inside the returned slices so Join reproduces the input
// byte-for-byte.
func splitLines(doc []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range doc {
		if b == '\n' {
			lines = append(lines, doc[start:i+1])
			start = i + 1
		}
	}
	if start < len(doc) {
		lines = append(lines, doc[start:])
	}
	return lines
}

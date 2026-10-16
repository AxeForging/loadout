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

// planCheckboxRe matches any checklist line (plan mode does not require
// sdd markers). Group 1 = prefix up to the checkbox char, 2 = checkbox char,
// 3 = the rest of the line after "] ".
var planCheckboxRe = regexp.MustCompile(`^(\s*[-*] \[)([ xX])\] (.*?)\s*$`)

var (
	sddMarkerRe  = regexp.MustCompile(`<!-- sdd:([a-z0-9][a-z0-9-]*) -->`)
	boldNumberRe = regexp.MustCompile(`^\*\*(\d+)\.`)
)

// Plan section markers used by the issues backend: the plan checklist lives
// between them inside the spec issue body.
const (
	PlanStart = "<!-- plan:start -->"
	PlanEnd   = "<!-- plan:end -->"
)

// locatedStep is a parsed step plus where its checkbox character sits.
type locatedStep struct {
	Step
	line, box int
}

// scanSteps walks a document's checklist lines, skipping fenced code. In
// marked mode (spec files, github backend) only lines carrying an sdd marker
// are steps. In plan mode (issues backend) every checkbox between the plan
// markers is a step; its ID is its sdd marker, else its leading "**N." number,
// else empty (listed, but not addressable by check/uncheck).
func scanSteps(doc []byte, planMode bool) ([]locatedStep, error) {
	var steps []locatedStep
	seen := map[string]bool{}
	inFence, inPlan := false, !planMode
	for i, line := range splitLines(doc) {
		if planMode {
			switch {
			case bytes.Contains(line, []byte(PlanStart)):
				inPlan = true
				continue
			case bytes.Contains(line, []byte(PlanEnd)):
				inPlan = false
				continue
			}
		}
		if fenceRe.Match(line) {
			inFence = !inFence
			continue
		}
		if inFence || !inPlan {
			continue
		}
		var st locatedStep
		if planMode {
			m := planCheckboxRe.FindSubmatchIndex(line)
			if m == nil {
				continue
			}
			rest := string(line[m[6]:m[7]])
			if mk := sddMarkerRe.FindStringSubmatch(rest); mk != nil {
				st.ID = mk[1]
				rest = sddMarkerRe.ReplaceAllString(rest, "")
			} else if n := boldNumberRe.FindStringSubmatch(rest); n != nil {
				st.ID = n[1]
			}
			st.Text = strings.TrimSpace(rest)
			st.box = m[4]
			st.Done = line[m[4]] != ' '
		} else {
			m := stepLineRe.FindSubmatchIndex(line)
			if m == nil {
				continue
			}
			st.ID = string(line[m[8]:m[9]])
			st.Text = strings.TrimSpace(strings.TrimPrefix(string(line[m[6]:m[7]]), "] "))
			st.box = m[4]
			st.Done = line[m[4]] != ' '
		}
		st.line = i
		if st.ID != "" {
			if seen[st.ID] {
				return nil, fmt.Errorf("duplicate step id %q — step ids must be unique within a spec", st.ID)
			}
			seen[st.ID] = true
		}
		steps = append(steps, st)
	}
	return steps, nil
}

func stepsOf(located []locatedStep) []Step {
	out := make([]Step, len(located))
	for i, l := range located {
		out[i] = l.Step
	}
	return out
}

// ParseSteps extracts the marked checklist steps of a spec document in
// order. Duplicate step IDs are an error — check/uncheck would be ambiguous.
func ParseSteps(doc []byte) ([]Step, error) {
	located, err := scanSteps(doc, false)
	if err != nil {
		return nil, err
	}
	return stepsOf(located), nil
}

// ParsePlanSteps extracts the checklist between the plan markers of an
// issues-backend spec body (see scanSteps for how IDs are derived).
func ParsePlanSteps(doc []byte) ([]Step, error) {
	located, err := scanSteps(doc, true)
	if err != nil {
		return nil, err
	}
	return stepsOf(located), nil
}

// SetStep toggles exactly one step's checkbox character, preserving every
// other byte of the document (line endings included). Setting a step to the
// state it already has is a no-op success (changed=false, doc returned as
// given). An unknown ID errors with the list of valid IDs.
func SetStep(doc []byte, id string, done bool) (out []byte, changed bool, err error) {
	return setStep(doc, id, done, false)
}

// SetPlanStep is SetStep for the plan section of an issues-backend body.
func SetPlanStep(doc []byte, id string, done bool) (out []byte, changed bool, err error) {
	return setStep(doc, id, done, true)
}

func setStep(doc []byte, id string, done, planMode bool) ([]byte, bool, error) {
	steps, err := scanSteps(doc, planMode)
	if err != nil {
		return nil, false, err
	}
	valid := make([]string, 0, len(steps))
	var target *locatedStep
	for i := range steps {
		if steps[i].ID == "" {
			continue
		}
		valid = append(valid, steps[i].ID)
		if steps[i].ID == id {
			target = &steps[i]
		}
	}
	if target == nil {
		if len(valid) == 0 {
			if planMode {
				return nil, false, fmt.Errorf("unknown step %q — the plan has no addressable steps (number them \"- [ ] **N. title**\" or add `<!-- sdd:<id> -->` markers between %s and %s)", id, PlanStart, PlanEnd)
			}
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
	lines := splitLines(doc)
	edited := append([]byte{}, lines[target.line]...)
	edited[target.box] = box
	lines[target.line] = edited
	return bytes.Join(lines, nil), true, nil
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

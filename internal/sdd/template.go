package sdd

import (
	"fmt"
	"regexp"
	"strings"
)

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// specTemplate is the team spec skeleton. Checklist steps carry stable
// `<!-- sdd:<id> -->` markers so `loadout spec check` can flip them without
// touching anything else.
func specTemplate(num int, slug string) string {
	title := strings.ReplaceAll(slug, "-", " ")
	number := fmt.Sprintf("%03d", num)
	return fmt.Sprintf(`# %s — %s

**Status:** Draft — pending review

## Context

Why now, what problem, who is affected.

## Requirements

Numbered, testable statements (MUST/SHOULD).

## Non-goals

Explicitly out of scope.

## Design

Approach, alternatives considered and why rejected.

## Data & API changes

Schemas, endpoints, migrations.

## Test plan

Per requirement, how it is verified.

## Checklist

Steps carry stable IDs — check them off with:
`+"`loadout spec check %[1]s <step-id>`"+`

- [ ] draft reviewed (or waived in the PR) <!-- sdd:review -->
- [ ] implementation <!-- sdd:implement -->
- [ ] tests pass locally <!-- sdd:tests -->

## Rollout

Flags, migration order, rollback.
`, number, title)
}

// issueTemplate is the body of a new issues-backend spec: the spec sections
// followed by the plan placeholder that /plan later fills between the
// markers. light keeps the short Problem/Approach form.
func issueTemplate(full bool) string {
	var sections string
	if full {
		sections = `## Context

Why now, what problem, who is affected.

## Requirements

Numbered (R1, R2…), testable statements (MUST/SHOULD).

## Non-goals

Explicitly out of scope.

## Design

Approach, alternatives considered and why rejected.

## Data & API changes

Schemas, endpoints, migrations.

## Test plan

Per requirement, how it is verified.

## Rollout

Flags, migration order, rollback.
`
	} else {
		sections = `## Problem

One paragraph, user-visible framing.

## Approach

The chosen solution in a few bullets, plus what was ruled out.
`
	}
	return sections + `
` + PlanStart + `
## Plan
_Not derived yet — run the plan command after the spec is approved._
` + PlanEnd + `
`
}

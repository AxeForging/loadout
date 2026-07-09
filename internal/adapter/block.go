package adapter

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
)

// Managed-block sentinels. Every adapter that renders into a shared text file
// (CLAUDE.md, AGENTS.md, GEMINI.md) wraps its content with these markers so
// the engine can upsert it without touching user-authored content.

func blockBegin(id string) string { return fmt.Sprintf("<!-- loadout:begin %s -->", id) }
func blockEnd(id string) string   { return fmt.Sprintf("<!-- loadout:end %s -->", id) }

// WrapBlock wraps body in managed-block markers for the given artifact ID.
func WrapBlock(id string, body []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(blockBegin(id))
	buf.WriteByte('\n')
	buf.Write(bytes.TrimRight(body, "\n"))
	buf.WriteByte('\n')
	buf.WriteString(blockEnd(id))
	buf.WriteByte('\n')
	return buf.Bytes()
}

var blockIDRe = regexp.MustCompile(`^<!-- loadout:begin ([a-z0-9-]+) -->`)

// BlockID extracts the artifact ID from a wrapped block.
func BlockID(fragment []byte) (string, error) {
	m := blockIDRe.FindSubmatch(bytes.TrimLeft(fragment, "\n"))
	if m == nil {
		return "", fmt.Errorf("fragment does not start with a loadout block marker")
	}
	return string(m[1]), nil
}

// UpsertBlock replaces the managed block with the fragment's ID inside doc,
// or appends the fragment if no such block exists. Content outside the block
// is preserved byte-for-byte.
func UpsertBlock(doc, fragment []byte) ([]byte, error) {
	id, err := BlockID(fragment)
	if err != nil {
		return nil, err
	}
	fragment = bytes.TrimRight(bytes.TrimLeft(fragment, "\n"), "\n")

	begin := []byte(blockBegin(id))
	end := []byte(blockEnd(id))
	start := bytes.Index(doc, begin)
	if start >= 0 {
		rest := doc[start:]
		endIdx := bytes.Index(rest, end)
		if endIdx < 0 {
			return nil, fmt.Errorf("block %q has a begin marker but no end marker", id)
		}
		var buf bytes.Buffer
		buf.Write(doc[:start])
		buf.Write(fragment)
		buf.Write(rest[endIdx+len(end):])
		return buf.Bytes(), nil
	}

	var buf bytes.Buffer
	buf.Write(doc)
	if len(doc) > 0 && !bytes.HasSuffix(doc, []byte("\n")) {
		buf.WriteByte('\n')
	}
	if len(doc) > 0 {
		buf.WriteByte('\n')
	}
	buf.Write(fragment)
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

var (
	anyBeginRe = regexp.MustCompile(`<!-- loadout:begin ([a-z0-9-]+) -->`)
	anyEndRe   = regexp.MustCompile(`<!-- loadout:end ([a-z0-9-]+) -->`)
)

// CheckBlocks reports structural problems with the managed blocks in a
// document: begin markers without a matching end, ends without a begin, and
// duplicated blocks. An empty result means the document is healthy.
func CheckBlocks(doc []byte) []string {
	begins := map[string]int{}
	ends := map[string]int{}
	for _, m := range anyBeginRe.FindAllSubmatch(doc, -1) {
		begins[string(m[1])]++
	}
	for _, m := range anyEndRe.FindAllSubmatch(doc, -1) {
		ends[string(m[1])]++
	}
	var problems []string
	for id, n := range begins {
		switch {
		case ends[id] < n && ends[id] == 0:
			problems = append(problems, fmt.Sprintf("block %q has no end marker", id))
		case ends[id] < n:
			problems = append(problems, fmt.Sprintf("block %q has %d begin marker(s) but %d end marker(s)", id, n, ends[id]))
		case n > 1:
			problems = append(problems, fmt.Sprintf("block %q appears %d times", id, n))
		}
	}
	for id, n := range ends {
		if begins[id] < n {
			problems = append(problems, fmt.Sprintf("block %q has an end marker without a begin", id))
		}
	}
	sort.Strings(problems)
	return problems
}

// RemoveBlock strips the managed block for id out of doc, collapsing the
// surrounding whitespace to at most one blank-line separator (never a
// double blank line) between whatever remains before and after it. Content
// outside the block is otherwise preserved byte-for-byte. A missing block
// is a no-op (found=false, doc returned unchanged). A begin marker with no
// matching end marker errors — the same "don't guess on structural damage"
// behavior as UpsertBlock and CheckBlocks.
func RemoveBlock(doc []byte, id string) (result []byte, found bool, err error) {
	begin := []byte(blockBegin(id))
	end := []byte(blockEnd(id))
	start := bytes.Index(doc, begin)
	if start < 0 {
		return doc, false, nil
	}
	rest := doc[start:]
	endIdx := bytes.Index(rest, end)
	if endIdx < 0 {
		return nil, false, fmt.Errorf("block %q has a begin marker but no end marker", id)
	}

	before := bytes.TrimRight(doc[:start], "\n")
	after := bytes.TrimLeft(rest[endIdx+len(end):], "\n")

	var buf bytes.Buffer
	buf.Write(before)
	switch {
	case len(before) > 0 && len(after) > 0:
		buf.WriteString("\n\n")
	case len(before) > 0:
		buf.WriteString("\n")
	}
	buf.Write(after)
	return buf.Bytes(), true, nil
}

// ExtractBlock returns the managed block (markers included) for id, or nil.
func ExtractBlock(doc []byte, id string) []byte {
	begin := []byte(blockBegin(id))
	end := []byte(blockEnd(id))
	start := bytes.Index(doc, begin)
	if start < 0 {
		return nil
	}
	rest := doc[start:]
	endIdx := bytes.Index(rest, end)
	if endIdx < 0 {
		return nil
	}
	return rest[:endIdx+len(end)]
}

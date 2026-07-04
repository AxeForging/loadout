package adapter

import (
	"bytes"
	"fmt"
	"regexp"
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

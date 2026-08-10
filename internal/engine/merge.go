package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/AxeForging/loadout/internal/adapter"
)

// merged computes the desired on-disk content for a target file given what is
// currently on disk.
func merged(strategy adapter.MergeStrategy, current, fragment []byte) ([]byte, error) {
	switch strategy {
	case adapter.MergeReplace:
		return fragment, nil
	case adapter.MergeBlock:
		return adapter.UpsertBlock(current, fragment)
	case adapter.MergeJSON:
		return jsonMerge(current, fragment)
	}
	return nil, fmt.Errorf("unknown merge strategy %q", strategy)
}

// jsonMerge deep-merges a JSON fragment into a JSON document surgically: keys
// the fragment doesn't touch keep their exact bytes (order, indentation,
// whitespace), so a shared file like ~/.claude.json — which holds a lot of
// user state loadout has no business touching — stays as the user wrote it.
// Maps merge recursively; arrays union (fragment items appended unless already
// present); fragment scalars win. If the merge is a semantic no-op the
// existing bytes are returned untouched so user formatting never causes
// phantom drift.
func jsonMerge(existing, fragment []byte) ([]byte, error) {
	var frag any
	if err := json.Unmarshal(fragment, &frag); err != nil {
		return nil, fmt.Errorf("invalid JSON fragment: %w", err)
	}
	if len(bytes.TrimSpace(existing)) == 0 {
		out, err := json.MarshalIndent(frag, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(out, '\n'), nil
	}
	// Reject a corrupt existing file loudly — writing over it would destroy
	// state we can't reason about.
	var probe any
	if err := json.Unmarshal(existing, &probe); err != nil {
		return nil, fmt.Errorf("existing file is not valid JSON: %w", err)
	}

	result, changed, err := applyFragment(existing, "", frag)
	if err != nil {
		return nil, err
	}
	if !changed {
		return existing, nil
	}
	return result, nil
}

// applyFragment walks the fragment and issues surgical sjson writes for the
// leaf-most mutations. Map keys recurse (so nested untouched keys keep their
// bytes); arrays and scalars set their whole subtree at once. Returns the new
// bytes plus whether anything actually differs from the input.
func applyFragment(existing []byte, path string, frag any) ([]byte, bool, error) {
	switch v := frag.(type) {
	case map[string]any:
		result := existing
		anyChange := false
		for _, k := range sortedKeys(v) {
			sub := joinPath(path, k)
			existingAtSub := gjson.GetBytes(result, sub)
			// Only recurse when both sides are maps — that's what preserves
			// unrelated sibling keys' bytes. Otherwise (missing, scalar, or
			// array on either side) set the whole subtree.
			if _, isMap := v[k].(map[string]any); isMap && existingAtSub.Exists() && existingAtSub.IsObject() {
				next, changed, err := applyFragment(result, sub, v[k])
				if err != nil {
					return nil, false, err
				}
				result = next
				anyChange = anyChange || changed
				continue
			}
			next, changed, err := setLeaf(result, sub, v[k])
			if err != nil {
				return nil, false, err
			}
			result = next
			anyChange = anyChange || changed
		}
		return result, anyChange, nil
	default:
		return setLeaf(existing, path, frag)
	}
}

// setLeaf writes a subtree (map/array/scalar) at path, with array-union
// semantics for arrays and a semantic no-op check to keep the input bytes.
func setLeaf(existing []byte, path string, value any) ([]byte, bool, error) {
	existingAt := gjson.GetBytes(existing, path)

	if arr, ok := value.([]any); ok && existingAt.Exists() && existingAt.IsArray() {
		var current []any
		if err := json.Unmarshal([]byte(existingAt.Raw), &current); err != nil {
			return nil, false, err
		}
		merged := current
		for _, item := range arr {
			if !containsDeep(merged, item) {
				merged = append(merged, item)
			}
		}
		if reflect.DeepEqual(current, merged) {
			return existing, false, nil
		}
		value = merged
	}

	if existingAt.Exists() && jsonEqual(existingAt.Raw, value) {
		return existing, false, nil
	}

	valueJSON, err := json.Marshal(value)
	if err != nil {
		return nil, false, err
	}
	out, err := sjson.SetRawBytes(existing, path, valueJSON)
	if err != nil {
		return nil, false, fmt.Errorf("merge %q: %w", path, err)
	}
	return out, true, nil
}

// jsonRemove computes the inverse of jsonMerge: the current bytes with exactly
// the fragment's contribution removed, surgically — bytes outside the removed
// keys are preserved, and the file itself is never emptied to nothing the
// caller wouldn't expect (the root object always survives). Returns the drifted
// paths: places where the on-disk value no longer matches the fragment, which
// are left in place unless force is set.
func jsonRemove(existing, fragment []byte, force bool) (out []byte, changed bool, drifted []string, err error) {
	var frag any
	if err := json.Unmarshal(fragment, &frag); err != nil {
		return nil, false, nil, fmt.Errorf("invalid JSON fragment: %w", err)
	}
	if len(bytes.TrimSpace(existing)) == 0 {
		return existing, false, nil, nil
	}
	// Reject a corrupt existing file loudly — same discipline as jsonMerge.
	var probe any
	if err := json.Unmarshal(existing, &probe); err != nil {
		return nil, false, nil, fmt.Errorf("existing file is not valid JSON: %w", err)
	}
	fm, ok := frag.(map[string]any)
	if !ok {
		return nil, false, nil, fmt.Errorf("fragment root must be a JSON object")
	}
	result, changed, drifted, err := removeMapKeys(existing, "", fm, force)
	if err != nil {
		return nil, false, nil, err
	}
	if !changed {
		return existing, false, drifted, nil
	}
	return result, true, drifted, nil
}

// removeMapKeys deletes this map's fragment-defined keys from existing.
// Removal is atomic within the object that directly contains an edited value:
// if any of this map's own keys drifted (value differs and the shapes don't
// allow recursing), every deletion inside this map is rolled back and the
// drift reported — a partially-edited unit (e.g. one MCP server entry) is
// removed whole under force or left whole, never gutted.
func removeMapKeys(existing []byte, path string, frag map[string]any, force bool) ([]byte, bool, []string, error) {
	pre := existing
	result := existing
	anyChange := false
	var drifts []string
	directDrift := false

	for _, k := range sortedKeys(frag) {
		sub := joinPath(path, k)
		existingAt := gjson.GetBytes(result, sub)
		if !existingAt.Exists() {
			continue // already absent — nothing of ours here
		}
		if jsonEqual(existingAt.Raw, frag[k]) {
			next, err := sjson.DeleteBytes(result, sub)
			if err != nil {
				return nil, false, nil, fmt.Errorf("remove %q: %w", sub, err)
			}
			result = next
			anyChange = true
			continue
		}
		if subMap, ok := frag[k].(map[string]any); ok && existingAt.IsObject() {
			next, subChanged, subDrifts, err := removeMapKeys(result, sub, subMap, force)
			if err != nil {
				return nil, false, nil, err
			}
			// Prune the object when our removals emptied it — an empty
			// "mcpServers": {} left behind is noise, not user content.
			if subChanged {
				if at := gjson.GetBytes(next, sub); at.IsObject() && len(at.Map()) == 0 {
					if next, err = sjson.DeleteBytes(next, sub); err != nil {
						return nil, false, nil, fmt.Errorf("prune %q: %w", sub, err)
					}
				}
			}
			result = next
			anyChange = anyChange || subChanged
			drifts = append(drifts, subDrifts...)
			continue
		}
		if arr, ok := frag[k].([]any); ok && existingAt.IsArray() {
			next, subChanged, missing, err := removeArrayItems(result, sub, arr)
			if err != nil {
				return nil, false, nil, err
			}
			result = next
			anyChange = anyChange || subChanged
			if missing {
				// An installed array item was edited or hand-removed; it can't
				// be identified anymore, so it stays — even under force.
				drifts = append(drifts, sub)
				directDrift = true
			}
			continue
		}
		// Value differs and the shapes don't allow recursion: a user edit.
		if force {
			next, err := sjson.DeleteBytes(result, sub)
			if err != nil {
				return nil, false, nil, fmt.Errorf("remove %q: %w", sub, err)
			}
			result = next
			anyChange = true
			continue
		}
		drifts = append(drifts, sub)
		directDrift = true
	}

	if directDrift && !force {
		return pre, false, drifts, nil
	}
	return result, anyChange, drifts, nil
}

// removeArrayItems removes elements deep-equal to the fragment's items — the
// inverse of setLeaf's array union. User-added elements survive; an emptied
// array is deleted outright. missing reports fragment items with no deep-equal
// element left (edited or already removed by hand — unidentifiable).
func removeArrayItems(existing []byte, path string, fragItems []any) (out []byte, changed, missing bool, err error) {
	existingAt := gjson.GetBytes(existing, path)
	var current []any
	if err := json.Unmarshal([]byte(existingAt.Raw), &current); err != nil {
		return nil, false, false, err
	}
	for _, item := range fragItems {
		if !containsDeep(current, item) {
			missing = true
		}
	}
	kept := make([]any, 0, len(current))
	for _, cur := range current {
		if !containsDeep(fragItems, cur) {
			kept = append(kept, cur)
		}
	}
	if len(kept) == len(current) {
		return existing, false, missing, nil
	}
	if len(kept) == 0 {
		out, err := sjson.DeleteBytes(existing, path)
		return out, err == nil, missing, err
	}
	raw, err := json.Marshal(kept)
	if err != nil {
		return nil, false, false, err
	}
	out, err = sjson.SetRawBytes(existing, path, raw)
	if err != nil {
		return nil, false, false, fmt.Errorf("remove from %q: %w", path, err)
	}
	return out, true, missing, nil
}

// jsonEqual reports whether raw and want represent the same JSON value —
// tolerant of whitespace and key order.
func jsonEqual(raw string, want any) bool {
	var got any
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return false
	}
	return reflect.DeepEqual(got, want)
}

// joinPath composes an sjson path, escaping dots inside individual keys so a
// key that contains a "." (rare but legal) isn't treated as a path separator.
func joinPath(prefix, key string) string {
	escaped := strings.ReplaceAll(key, `.`, `\.`)
	if prefix == "" {
		return escaped
	}
	return prefix + "." + escaped
}

// sortedKeys returns m's keys sorted, so repeated merges of the same fragment
// produce a stable byte sequence — otherwise map iteration order would leak
// into diffs.
func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// Ordering matters only for determinism; stable insertion order isn't
	// preservable through Go's map, so alphabetic keeps diffs quiet.
	sort.Strings(out)
	return out
}

func containsDeep(list []any, item any) bool {
	for _, v := range list {
		if reflect.DeepEqual(v, item) {
			return true
		}
	}
	return false
}

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

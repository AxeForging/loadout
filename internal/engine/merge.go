package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"

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

// jsonMerge deep-merges a JSON fragment into a JSON document: maps merge
// recursively, arrays union (fragment items appended unless already present),
// fragment scalars win. If the merge is a semantic no-op the existing bytes
// are returned untouched so user formatting never causes phantom drift.
func jsonMerge(existing, fragment []byte) ([]byte, error) {
	var frag any
	if err := json.Unmarshal(fragment, &frag); err != nil {
		return nil, fmt.Errorf("invalid JSON fragment: %w", err)
	}
	var base any
	if len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &base); err != nil {
			return nil, fmt.Errorf("existing file is not valid JSON: %w", err)
		}
	}

	var before any
	if base != nil {
		// Deep copy via round-trip so we can compare after merging.
		raw, err := json.Marshal(base)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &before); err != nil {
			return nil, err
		}
	}

	result := deepMerge(base, frag)
	if base != nil && reflect.DeepEqual(before, result) {
		return existing, nil
	}

	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func deepMerge(base, frag any) any {
	if bm, ok := base.(map[string]any); ok {
		if fm, ok := frag.(map[string]any); ok {
			for k, v := range fm {
				bm[k] = deepMerge(bm[k], v)
			}
			return bm
		}
	}
	if ba, ok := base.([]any); ok {
		if fa, ok := frag.([]any); ok {
			for _, item := range fa {
				if !containsDeep(ba, item) {
					ba = append(ba, item)
				}
			}
			return ba
		}
	}
	return frag
}

func containsDeep(list []any, item any) bool {
	for _, v := range list {
		if reflect.DeepEqual(v, item) {
			return true
		}
	}
	return false
}

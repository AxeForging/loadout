package source

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
)

// UpstreamDiff renders unified diffs between a source-owned skill's store
// content (possibly locally edited) and its content at the source's remote
// head. Read-only: it refreshes the cache clone but never writes the store.
func (m *Manager) UpstreamDiff(ctx context.Context, skillID string) (string, error) {
	art, ok := m.Store.Artifact(skillID)
	if !ok {
		return "", fmt.Errorf("no artifact %q in the store", skillID)
	}
	srcName := strings.TrimPrefix(art.Origin, "source:")
	if srcName == art.Origin {
		return "", fmt.Errorf("artifact %q has origin %q, not a source — --upstream only applies to skills imported via 'loadout source add'", skillID, orLocal(art.Origin))
	}
	ref, _, err := m.find(srcName)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(m.CacheDir, srcName)
	if _, err := cloneFull(ctx, ref.URL, ref.Ref, dir); err != nil {
		return "", err
	}
	skills, err := DiscoverSkills(dir, ref.Subdir)
	if err != nil {
		return "", err
	}
	var upstream *SkillDir
	for i := range skills {
		if skills[i].Name == skillID {
			upstream = &skills[i]
			break
		}
	}
	if upstream == nil {
		return "", fmt.Errorf("skill %q no longer exists upstream in source %q", skillID, srcName)
	}

	current, err := m.Store.Content(art)
	if err != nil {
		return "", err
	}
	upstreamContent, err := dirContent(upstream.Path)
	if err != nil {
		return "", err
	}

	names := map[string]bool{}
	for p := range current.Files {
		names[p] = true
	}
	for p := range upstreamContent.Files {
		names[p] = true
	}
	paths := make([]string, 0, len(names))
	for p := range names {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var out strings.Builder
	for _, p := range paths {
		local, upstreamRaw := current.Files[p], upstreamContent.Files[p]
		if string(local) == string(upstreamRaw) {
			continue
		}
		text, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(local)),
			B:        difflib.SplitLines(string(upstreamRaw)),
			FromFile: "store/" + art.Path + "/" + p,
			ToFile:   "upstream/" + srcName + "/" + skillID + "/" + p,
			Context:  3,
		})
		if err != nil {
			return "", err
		}
		out.WriteString(text)
	}
	if out.Len() == 0 {
		return "", nil
	}
	return out.String(), nil
}

func orLocal(origin string) string {
	if origin == "" {
		return "local"
	}
	return origin
}

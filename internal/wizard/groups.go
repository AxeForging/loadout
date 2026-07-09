package wizard

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// groupsFile is the store-relative path of the shared library-group
// definitions — named collections of library entry IDs a user can apply all
// at once, distinct from the entries themselves (library.go).
const groupsFile = "library/groups.yaml"

// LibraryGroup is a named collection of library entry IDs.
type LibraryGroup struct {
	Name      string    `json:"name" yaml:"name"`
	EntryIDs  []string  `json:"entryIds" yaml:"entryIds"`
	CreatedAt time.Time `json:"createdAt" yaml:"createdAt"`
}

type groupsDoc struct {
	Groups []LibraryGroup `json:"groups" yaml:"groups"`
}

// ListGroups returns saved library groups, sorted by name. Returns an
// empty (non-nil) slice, not an error, when no groups file exists yet.
func ListGroups(s *store.Store) ([]LibraryGroup, error) {
	doc, err := readGroups(s)
	if err != nil {
		return nil, err
	}
	if doc.Groups == nil {
		return []LibraryGroup{}, nil
	}
	sort.Slice(doc.Groups, func(i, j int) bool { return doc.Groups[i].Name < doc.Groups[j].Name })
	return doc.Groups, nil
}

// SaveGroup creates or replaces (upserts by name) a named group of library
// entry IDs. Every entryID must already exist in the library — SaveGroup
// never guesses or silently drops an unknown one.
func SaveGroup(s *store.Store, name string, entryIDs []string) (LibraryGroup, error) {
	if err := store.ValidateName(name); err != nil {
		return LibraryGroup{}, err
	}
	entries, err := ListLibrary(s)
	if err != nil {
		return LibraryGroup{}, err
	}
	known := make(map[string]bool, len(entries))
	for _, e := range entries {
		known[e.ID] = true
	}
	seen := map[string]bool{}
	ids := make([]string, 0, len(entryIDs))
	for _, id := range entryIDs {
		if !known[id] {
			return LibraryGroup{}, fmt.Errorf("unknown library entry %q", id)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	doc, err := readGroups(s)
	if err != nil {
		return LibraryGroup{}, err
	}
	group := LibraryGroup{Name: name, EntryIDs: ids, CreatedAt: time.Now().UTC()}
	replaced := false
	for i, g := range doc.Groups {
		if g.Name == name {
			doc.Groups[i] = group
			replaced = true
			break
		}
	}
	if !replaced {
		doc.Groups = append(doc.Groups, group)
	}
	if err := writeGroups(s, doc); err != nil {
		return LibraryGroup{}, err
	}
	return group, nil
}

// RemoveGroup deletes a group's definition only — member library entries
// are never touched.
func RemoveGroup(s *store.Store, name string) error {
	doc, err := readGroups(s)
	if err != nil {
		return err
	}
	idx := -1
	for i, g := range doc.Groups {
		if g.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("unknown library group %q", name)
	}
	doc.Groups = append(doc.Groups[:idx], doc.Groups[idx+1:]...)
	return writeGroups(s, doc)
}

func readGroups(s *store.Store) (groupsDoc, error) {
	raw, err := os.ReadFile(filepath.Join(s.Root, groupsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return groupsDoc{}, nil
		}
		return groupsDoc{}, err
	}
	var doc groupsDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return groupsDoc{}, fmt.Errorf("parse library groups: %w", err)
	}
	return doc, nil
}

func writeGroups(s *store.Store, doc groupsDoc) error {
	dir := filepath.Join(s.Root, filepath.Dir(groupsFile))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Root, groupsFile), raw, 0o644)
}

package wizard

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestGroups_ListEmptyWhenFileAbsent(t *testing.T) {
	s := testStore(t)
	got, err := ListGroups(s)
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListGroups() on empty store = %v, want empty", got)
	}
}

func TestGroups_SaveListRoundTrip(t *testing.T) {
	s := testStore(t)
	a, err := AddToLibrary(s, "always write tests first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := AddToLibrary(s, "no coverage theater")
	if err != nil {
		t.Fatal(err)
	}

	group, err := SaveGroup(s, "testing-rules", []string{a.ID, b.ID})
	if err != nil {
		t.Fatalf("SaveGroup() error = %v", err)
	}
	want := []string{a.ID, b.ID}
	sort.Strings(want)
	if !reflect.DeepEqual(group.EntryIDs, want) {
		t.Errorf("EntryIDs = %v, want %v", group.EntryIDs, want)
	}

	list, err := ListGroups(s)
	if err != nil {
		t.Fatalf("ListGroups() error = %v", err)
	}
	if len(list) != 1 || list[0].Name != "testing-rules" {
		t.Fatalf("ListGroups() = %v, want [testing-rules]", list)
	}
}

func TestGroups_SaveUpsertsReplacesMembership(t *testing.T) {
	s := testStore(t)
	a, _ := AddToLibrary(s, "entry a")
	b, _ := AddToLibrary(s, "entry b")

	if _, err := SaveGroup(s, "rules", []string{a.ID}); err != nil {
		t.Fatal(err)
	}
	group, err := SaveGroup(s, "rules", []string{b.ID})
	if err != nil {
		t.Fatalf("SaveGroup() (upsert) error = %v", err)
	}
	if len(group.EntryIDs) != 1 || group.EntryIDs[0] != b.ID {
		t.Errorf("upsert should replace membership, got %v", group.EntryIDs)
	}

	list, err := ListGroups(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("upsert should not create a duplicate group, got %v", list)
	}
}

func TestGroups_SaveDedupesEntryIDs(t *testing.T) {
	s := testStore(t)
	a, _ := AddToLibrary(s, "entry a")

	group, err := SaveGroup(s, "rules", []string{a.ID, a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(group.EntryIDs) != 1 {
		t.Errorf("EntryIDs = %v, want deduped to one", group.EntryIDs)
	}
}

func TestGroups_SaveRejectsUnknownEntryID(t *testing.T) {
	s := testStore(t)
	if _, err := SaveGroup(s, "rules", []string{"does-not-exist"}); err == nil {
		t.Fatal("expected error for an entry ID not in the library")
	}
}

func TestGroups_SaveRejectsInvalidName(t *testing.T) {
	s := testStore(t)
	a, _ := AddToLibrary(s, "entry a")
	if _, err := SaveGroup(s, "Has Spaces", []string{a.ID}); err == nil {
		t.Fatal("expected error for an invalid group name")
	}
}

func TestGroups_RemoveLeavesEntriesUntouched(t *testing.T) {
	s := testStore(t)
	a, _ := AddToLibrary(s, "entry a")
	if _, err := SaveGroup(s, "rules", []string{a.ID}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveGroup(s, "rules"); err != nil {
		t.Fatalf("RemoveGroup() error = %v", err)
	}

	groups, err := ListGroups(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 0 {
		t.Errorf("group should be gone, got %v", groups)
	}
	entries, err := ListLibrary(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != a.ID {
		t.Errorf("removing a group should not touch its entries, got %v", entries)
	}
}

func TestGroups_RemoveUnknownErrors(t *testing.T) {
	s := testStore(t)
	err := RemoveGroup(s, "nope")
	if err == nil {
		t.Fatal("RemoveGroup() with unknown name should error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error should mention the unknown name, got: %v", err)
	}
}

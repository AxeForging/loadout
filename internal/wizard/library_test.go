package wizard

import (
	"strings"
	"testing"
)

func TestLibrary_ListEmptyWhenFileAbsent(t *testing.T) {
	s := testStore(t)
	got, err := ListLibrary(s)
	if err != nil {
		t.Fatalf("ListLibrary() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ListLibrary() on empty store = %v, want empty", got)
	}
}

func TestLibrary_AddListRemove(t *testing.T) {
	s := testStore(t)

	entry, err := AddToLibrary(s, "  Always write tests before fixing a bug  ")
	if err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if entry.Text != "Always write tests before fixing a bug" {
		t.Errorf("AddToLibrary() trimmed text = %q", entry.Text)
	}
	if entry.ID == "" {
		t.Error("AddToLibrary() did not assign an ID")
	}

	list, err := ListLibrary(s)
	if err != nil {
		t.Fatalf("ListLibrary() error = %v", err)
	}
	if len(list) != 1 || list[0].ID != entry.ID {
		t.Fatalf("ListLibrary() = %v, want [%v]", list, entry)
	}

	if err := RemoveFromLibrary(s, entry.ID); err != nil {
		t.Fatalf("RemoveFromLibrary() error = %v", err)
	}
	after, err := ListLibrary(s)
	if err != nil {
		t.Fatalf("ListLibrary() error = %v", err)
	}
	if len(after) != 0 {
		t.Errorf("ListLibrary() after remove = %v, want empty", after)
	}
}

func TestLibrary_AddDeduplicatesByText(t *testing.T) {
	s := testStore(t)

	first, err := AddToLibrary(s, "no coverage theater")
	if err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	second, err := AddToLibrary(s, "  no coverage theater  ")
	if err != nil {
		t.Fatalf("AddToLibrary() error = %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("AddToLibrary() with re-typed whitespace produced a different ID: %v vs %v", first.ID, second.ID)
	}

	list, err := ListLibrary(s)
	if err != nil {
		t.Fatalf("ListLibrary() error = %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListLibrary() = %v, want exactly one deduplicated entry", list)
	}
}

func TestLibrary_AddEmptyTextErrors(t *testing.T) {
	s := testStore(t)
	if _, err := AddToLibrary(s, "   "); err == nil {
		t.Error("AddToLibrary() with blank text should error")
	}
}

func TestLibrary_RemoveUnknownErrors(t *testing.T) {
	s := testStore(t)
	err := RemoveFromLibrary(s, "does-not-exist")
	if err == nil {
		t.Fatal("RemoveFromLibrary() with unknown id should error")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should mention the unknown id, got: %v", err)
	}
}

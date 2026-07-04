package adapter

import (
	"strings"
	"testing"
)

func TestUpsertBlock_PreservesUserContentOutsideMarkers(t *testing.T) {
	doc := []byte("# My own notes\n\ndo not touch this\n")
	fragment := WrapBlock("style", []byte("use tabs"))

	out, err := UpsertBlock(doc, fragment)
	if err != nil {
		t.Fatalf("UpsertBlock() error = %v", err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "# My own notes\n\ndo not touch this\n") {
		t.Errorf("user content not preserved:\n%s", s)
	}
	if !strings.Contains(s, "<!-- loadout:begin style -->\nuse tabs\n<!-- loadout:end style -->") {
		t.Errorf("block not appended:\n%s", s)
	}
}

func TestUpsertBlock_ReplacesExistingBlockInPlace(t *testing.T) {
	doc, err := UpsertBlock([]byte("before\n"), WrapBlock("style", []byte("old content")))
	if err != nil {
		t.Fatal(err)
	}
	doc = append(doc, []byte("\nafter\n")...)

	out, err := UpsertBlock(doc, WrapBlock("style", []byte("new content")))
	if err != nil {
		t.Fatalf("UpsertBlock() error = %v", err)
	}
	s := string(out)
	if strings.Contains(s, "old content") {
		t.Errorf("old block content should be replaced:\n%s", s)
	}
	if !strings.Contains(s, "new content") {
		t.Errorf("new block content missing:\n%s", s)
	}
	if !strings.HasPrefix(s, "before\n") || !strings.HasSuffix(s, "\nafter\n") {
		t.Errorf("content around the block was modified:\n%s", s)
	}
	if strings.Count(s, "loadout:begin style") != 1 {
		t.Errorf("block duplicated:\n%s", s)
	}
}

func TestUpsertBlock_KeepsOtherBlocksIntact(t *testing.T) {
	doc, _ := UpsertBlock(nil, WrapBlock("one", []byte("first")))
	doc, _ = UpsertBlock(doc, WrapBlock("two", []byte("second")))

	out, err := UpsertBlock(doc, WrapBlock("one", []byte("first updated")))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "first updated") || !strings.Contains(s, "second") {
		t.Errorf("sibling block lost:\n%s", s)
	}
}

func TestUpsertBlock_FailsOnUnterminatedBlock(t *testing.T) {
	doc := []byte("<!-- loadout:begin style -->\nbroken\n")
	if _, err := UpsertBlock(doc, WrapBlock("style", []byte("x"))); err == nil {
		t.Fatal("expected error for missing end marker")
	}
}

func TestExtractBlock_ReturnsNilWhenAbsent(t *testing.T) {
	if got := ExtractBlock([]byte("nothing here"), "style"); got != nil {
		t.Errorf("ExtractBlock() = %q, want nil", got)
	}
}

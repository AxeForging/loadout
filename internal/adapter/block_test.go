package adapter

import (
	"bytes"
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

func TestCheckBlocks_ReportsUnbalancedAndDuplicateMarkers(t *testing.T) {
	healthy, _ := UpsertBlock(nil, WrapBlock("ok", []byte("fine")))
	if issues := CheckBlocks(healthy); len(issues) != 0 {
		t.Errorf("healthy doc reported issues: %v", issues)
	}

	broken := []byte("<!-- loadout:begin dangling -->\nno end\n")
	issues := CheckBlocks(broken)
	if len(issues) != 1 || !strings.Contains(issues[0], "dangling") {
		t.Errorf("missing-end not reported: %v", issues)
	}

	orphanEnd := []byte("text\n<!-- loadout:end ghost -->\n")
	issues = CheckBlocks(orphanEnd)
	if len(issues) != 1 || !strings.Contains(issues[0], "ghost") {
		t.Errorf("end-without-begin not reported: %v", issues)
	}
}

func TestExtractBlock_ReturnsNilWhenAbsent(t *testing.T) {
	if got := ExtractBlock([]byte("nothing here"), "style"); got != nil {
		t.Errorf("ExtractBlock() = %q, want nil", got)
	}
}

func TestRemoveBlock_PreservesContentOutsideMarkers(t *testing.T) {
	doc, _ := UpsertBlock([]byte("before\n"), WrapBlock("style", []byte("content")))
	doc = append(doc, []byte("after\n")...)

	out, found, err := RemoveBlock(doc, "style")
	if err != nil {
		t.Fatalf("RemoveBlock() error = %v", err)
	}
	if !found {
		t.Fatal("RemoveBlock() found = false, want true")
	}
	s := string(out)
	if !strings.HasPrefix(s, "before") {
		t.Errorf("content before the block not preserved:\n%s", s)
	}
	if !strings.HasSuffix(s, "after\n") {
		t.Errorf("content after the block not preserved:\n%s", s)
	}
	if strings.Contains(s, "loadout:begin style") || strings.Contains(s, "content") {
		t.Errorf("block not removed:\n%s", s)
	}
}

func TestRemoveBlock_NoDoubleBlankLineLeftBehind(t *testing.T) {
	doc, _ := UpsertBlock([]byte("before\n"), WrapBlock("style", []byte("content")))
	doc = append(doc, []byte("\nafter\n")...)

	out, _, err := RemoveBlock(doc, "style")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "\n\n\n") {
		t.Errorf("triple newline (double blank line) left behind:\n%q", out)
	}
	want := "before\n\nafter\n"
	if string(out) != want {
		t.Errorf("RemoveBlock() = %q, want %q", out, want)
	}
}

func TestRemoveBlock_MissingBlockIsNoop(t *testing.T) {
	doc := []byte("nothing to see here\n")
	out, found, err := RemoveBlock(doc, "style")
	if err != nil {
		t.Fatalf("RemoveBlock() error = %v", err)
	}
	if found {
		t.Error("RemoveBlock() found = true for an absent block")
	}
	if !bytes.Equal(out, doc) {
		t.Errorf("RemoveBlock() on a missing block = %q, want byte-identical %q", out, doc)
	}
}

func TestRemoveBlock_ErrorsOnUnterminatedBlock(t *testing.T) {
	doc := []byte("<!-- loadout:begin style -->\nbroken\n")
	if _, _, err := RemoveBlock(doc, "style"); err == nil {
		t.Fatal("expected error for missing end marker")
	}
}

func TestRemoveBlock_WholeFileWasJustTheBlock_ResultIsEmpty(t *testing.T) {
	doc, _ := UpsertBlock(nil, WrapBlock("style", []byte("content")))
	out, found, err := RemoveBlock(doc, "style")
	if err != nil {
		t.Fatalf("RemoveBlock() error = %v", err)
	}
	if !found {
		t.Fatal("RemoveBlock() found = false, want true")
	}
	if len(out) != 0 {
		t.Errorf("RemoveBlock() = %q, want empty", out)
	}
}

func TestRemoveBlock_KeepsOtherBlocksIntact(t *testing.T) {
	doc, _ := UpsertBlock(nil, WrapBlock("one", []byte("first")))
	doc, _ = UpsertBlock(doc, WrapBlock("two", []byte("second")))

	out, found, err := RemoveBlock(doc, "one")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("RemoveBlock() found = false, want true")
	}
	s := string(out)
	if strings.Contains(s, "first") || strings.Contains(s, "loadout:begin one") {
		t.Errorf("removed block still present:\n%s", s)
	}
	if !strings.Contains(s, "second") || !strings.Contains(s, "loadout:begin two") {
		t.Errorf("sibling block lost:\n%s", s)
	}
}

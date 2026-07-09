package wizard

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/store"
	"github.com/goccy/go-yaml"
)

// libraryFile is the store-relative path of the shared instruction library —
// custom-instruction text a user has saved for reuse across future wizard
// runs. Distinct from profiles (whole named answer sets).
const libraryFile = "library/instructions.yaml"

// LibraryEntry is one saved instruction string.
type LibraryEntry struct {
	ID        string    `json:"id" yaml:"id"`
	Text      string    `json:"text" yaml:"text"`
	CreatedAt time.Time `json:"createdAt" yaml:"createdAt"`
}

type libraryDoc struct {
	Instructions []LibraryEntry `json:"instructions" yaml:"instructions"`
}

// ListLibrary returns saved instruction entries in the order they were
// added. Returns an empty (non-nil) slice, not an error, when no library
// file exists yet.
func ListLibrary(s *store.Store) ([]LibraryEntry, error) {
	doc, err := readLibrary(s)
	if err != nil {
		return nil, err
	}
	if doc.Instructions == nil {
		return []LibraryEntry{}, nil
	}
	return doc.Instructions, nil
}

// AddToLibrary saves text as a new library entry, or returns the existing
// entry unchanged if the same text (after trimming) was already saved —
// entries are content-addressed, so re-adding is a safe no-op.
func AddToLibrary(s *store.Store, text string) (LibraryEntry, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return LibraryEntry{}, fmt.Errorf("instruction text must not be empty")
	}
	id := libraryID(text)

	doc, err := readLibrary(s)
	if err != nil {
		return LibraryEntry{}, err
	}
	for _, e := range doc.Instructions {
		if e.ID == id {
			return e, nil
		}
	}
	entry := LibraryEntry{ID: id, Text: text, CreatedAt: time.Now().UTC()}
	doc.Instructions = append(doc.Instructions, entry)
	if err := writeLibrary(s, doc); err != nil {
		return LibraryEntry{}, err
	}
	return entry, nil
}

// RemoveFromLibrary deletes a saved entry by ID.
func RemoveFromLibrary(s *store.Store, id string) error {
	doc, err := readLibrary(s)
	if err != nil {
		return err
	}
	idx := -1
	for i, e := range doc.Instructions {
		if e.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("unknown library entry %q", id)
	}
	doc.Instructions = append(doc.Instructions[:idx], doc.Instructions[idx+1:]...)
	return writeLibrary(s, doc)
}

func libraryID(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

func readLibrary(s *store.Store) (libraryDoc, error) {
	raw, err := os.ReadFile(filepath.Join(s.Root, libraryFile))
	if err != nil {
		if os.IsNotExist(err) {
			return libraryDoc{}, nil
		}
		return libraryDoc{}, err
	}
	var doc libraryDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return libraryDoc{}, fmt.Errorf("parse instruction library: %w", err)
	}
	return doc, nil
}

func writeLibrary(s *store.Store, doc libraryDoc) error {
	dir := filepath.Join(s.Root, filepath.Dir(libraryFile))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.Root, libraryFile), raw, 0o644)
}

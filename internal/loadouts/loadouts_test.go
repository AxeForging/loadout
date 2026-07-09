package loadouts

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Init(filepath.Join(t.TempDir(), "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func idsOf(l Loadout) []string {
	out := make([]string, 0, len(l.Artifacts))
	for _, a := range l.Artifacts {
		out = append(out, a.ID)
	}
	return out
}

func TestSaveListLoadRoundTrip(t *testing.T) {
	s := testStore(t)

	if names, err := ListLoadouts(s); err != nil || len(names) != 0 {
		t.Fatalf("ListLoadouts() on empty store = %v, %v", names, err)
	}

	if _, err := SaveLoadout(s, "frontend", []string{"demo", "style"}); err != nil {
		t.Fatalf("SaveLoadout() error = %v", err)
	}

	names, err := ListLoadouts(s)
	if err != nil {
		t.Fatalf("ListLoadouts() error = %v", err)
	}
	if len(names) != 1 || names[0] != "frontend" {
		t.Fatalf("ListLoadouts() = %v, want [frontend]", names)
	}

	loaded, err := LoadLoadout(s, "frontend")
	if err != nil {
		t.Fatalf("LoadLoadout() error = %v", err)
	}
	if !reflect.DeepEqual(idsOf(loaded), []string{"demo", "style"}) {
		t.Errorf("loaded artifacts = %v", idsOf(loaded))
	}
}

func TestList_EmptyWhenNoLoadoutsDir(t *testing.T) {
	s := testStore(t)
	names, err := ListLoadouts(s)
	if err != nil {
		t.Fatalf("ListLoadouts() error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("ListLoadouts() = %v, want empty", names)
	}
}

func TestSave_RejectsDefaultName(t *testing.T) {
	s := testStore(t)
	if _, err := SaveLoadout(s, "default", []string{"demo"}); err == nil {
		t.Fatal("saving over 'default' should be rejected")
	}
}

func TestRemove_RejectsDefaultName(t *testing.T) {
	s := testStore(t)
	if err := EnsureDefault(s); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLoadout(s, "default"); err == nil {
		t.Fatal("removing 'default' should be rejected")
	}
}

func TestRemove_UnknownNameErrors(t *testing.T) {
	s := testStore(t)
	if err := RemoveLoadout(s, "nope"); err == nil {
		t.Fatal("removing an unknown loadout should error")
	}
}

func TestRemove_DeletesDefinitionOnly(t *testing.T) {
	s := testStore(t)
	if _, err := SaveLoadout(s, "temp", []string{"demo"}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveLoadout(s, "temp"); err != nil {
		t.Fatalf("RemoveLoadout() error = %v", err)
	}
	if _, err := LoadLoadout(s, "temp"); err == nil {
		t.Error("loadout should be gone after RemoveLoadout")
	}
}

func TestLoad_UnknownNameListsAvailable(t *testing.T) {
	s := testStore(t)
	if _, err := SaveLoadout(s, "backend", []string{"demo"}); err != nil {
		t.Fatal(err)
	}
	_, err := LoadLoadout(s, "nope")
	if err == nil {
		t.Fatal("expected error for unknown loadout")
	}
	if !strings.Contains(err.Error(), "backend") {
		t.Errorf("error should list available loadouts, got: %v", err)
	}
}

func TestEnsureDefault_CreatesOnceFromCurrentManifest(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindSkill, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindInstruction, "style"); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDefault(s); err != nil {
		t.Fatalf("EnsureDefault() error = %v", err)
	}
	def, err := LoadLoadout(s, DefaultName)
	if err != nil {
		t.Fatalf("LoadLoadout(default) error = %v", err)
	}
	if !def.Protected {
		t.Error("default loadout should be marked protected")
	}
	got := idsOf(def)
	if len(got) != 2 {
		t.Fatalf("default loadout artifacts = %v, want 2", got)
	}
}

func TestEnsureDefault_NoopIfAlreadyExists(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindSkill, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefault(s); err != nil {
		t.Fatal(err)
	}
	first, err := LoadLoadout(s, DefaultName)
	if err != nil {
		t.Fatal(err)
	}

	// A second call, even after the manifest gained a new artifact, must not
	// touch the already-created baseline.
	if _, err := s.New(domain.KindSkill, "extra"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefault(s); err != nil {
		t.Fatal(err)
	}
	second, err := LoadLoadout(s, DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(idsOf(first), idsOf(second)) {
		t.Errorf("EnsureDefault() overwrote the baseline: %v -> %v", idsOf(first), idsOf(second))
	}
}

func TestEnsureDefault_FrozenSnapshot_LaterManifestChangesDontRetroactivelyJoin(t *testing.T) {
	s := testStore(t)
	if _, err := s.New(domain.KindSkill, "demo"); err != nil {
		t.Fatal(err)
	}
	if err := EnsureDefault(s); err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindSkill, "added-later"); err != nil {
		t.Fatal(err)
	}
	def, err := LoadLoadout(s, DefaultName)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range idsOf(def) {
		if id == "added-later" {
			t.Fatal("default loadout should not retroactively include artifacts added after it was created")
		}
	}
}

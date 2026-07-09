package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/wizard"
)

func TestExportFlat_ImportFlat_RoundTrip(t *testing.T) {
	src := testStore(t, "team-kit")
	if _, err := src.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	if err := wizard.SaveProfile(src, "acme-backend", wizard.Answers{"toolchain.docker": true}); err != nil {
		t.Fatal(err)
	}
	a, err := wizard.AddToLibrary(src, "always write tests first")
	if err != nil {
		t.Fatal(err)
	}
	b, err := wizard.AddToLibrary(src, "no coverage theater")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wizard.SaveGroup(src, "testing-rules", []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}

	raw, err := ExportFlat(src, "test", Selection{
		ArtifactIDs:     []string{"review"},
		ProfileNames:    []string{"acme-backend"},
		LibraryEntryIDs: []string{a.ID, b.ID},
	})
	if err != nil {
		t.Fatalf("ExportFlat() error = %v", err)
	}

	// The whole point of the flat format is human-readable text — file
	// content should appear as plain YAML block scalars, never base64.
	if !strings.Contains(string(raw), "apiVersion: loadout-config/v1") {
		t.Errorf("flat document missing apiVersion:\n%s", raw)
	}
	if !strings.Contains(string(raw), "name: review") {
		t.Errorf("flat document should contain readable skill content, not encoded bytes:\n%s", raw)
	}

	fresh := testStore(t, "mine")
	plan, err := ImportFlat(fresh, raw)
	if err != nil {
		t.Fatalf("ImportFlat() error = %v", err)
	}
	defer plan.Close()
	if len(plan.New) != 1 || plan.New[0].ID != "review" {
		t.Fatalf("plan.New = %v, want only [review]", plan.New)
	}
	if len(plan.NewProfiles) != 1 || plan.NewProfiles[0] != "acme-backend" {
		t.Fatalf("plan.NewProfiles = %v, want [acme-backend]", plan.NewProfiles)
	}
	if len(plan.NewLibraryEntries) != 2 {
		t.Fatalf("plan.NewLibraryEntries = %v, want 2 entries", plan.NewLibraryEntries)
	}

	if _, err := Apply(fresh, plan, false); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(fresh.Root, "skills", "review", "SKILL.md")); err != nil {
		t.Errorf("skill content not imported: %v", err)
	}
	loadedAnswers, err := wizard.LoadProfile(fresh, "acme-backend")
	if err != nil {
		t.Fatal(err)
	}
	if !loadedAnswers.Bool("toolchain.docker") {
		t.Error("profile answers lost across the flat round trip")
	}
	entries, err := wizard.ListLibrary(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("library entries = %v, want 2", entries)
	}
	groups, err := wizard.ListGroups(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Name != "testing-rules" {
		t.Errorf("groups = %v, want [testing-rules]", groups)
	}
}

func TestExportFlat_GroupOnlyIncludedWhenAllMembersSelected(t *testing.T) {
	src := testStore(t, "team-kit")
	a, err := wizard.AddToLibrary(src, "entry a")
	if err != nil {
		t.Fatal(err)
	}
	b, err := wizard.AddToLibrary(src, "entry b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wizard.SaveGroup(src, "both", []string{a.ID, b.ID}); err != nil {
		t.Fatal(err)
	}

	// Only entry a selected — the group should be excluded, since it also
	// references b, which the document wouldn't carry.
	partial, err := ExportFlat(src, "test", Selection{LibraryEntryIDs: []string{a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(partial), "libraryGroups") {
		t.Errorf("group should be excluded when not all members are selected:\n%s", partial)
	}

	// Both entries selected — the group should now be included.
	full, err := ExportFlat(src, "test", Selection{LibraryEntryIDs: []string{a.ID, b.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(full), "name: both") {
		t.Errorf("group should be included when all members are selected:\n%s", full)
	}
}

func TestOpenBundle_DetectsFlatYAML(t *testing.T) {
	src := testStore(t, "team-kit")
	if _, err := src.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	raw, err := ExportFlat(src, "test", Selection{ArtifactIDs: []string{"review"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	fresh := testStore(t, "mine")
	plan, err := PlanImport(fresh, path)
	if err != nil {
		t.Fatalf("PlanImport() on a flat YAML file path error = %v", err)
	}
	defer plan.Close()
	if len(plan.New) != 1 || plan.New[0].ID != "review" {
		t.Fatalf("plan.New = %v, want only [review]", plan.New)
	}
}

func TestOpenBundle_RejectsUnrecognizedContent(t *testing.T) {
	fresh := testStore(t, "mine")
	path := filepath.Join(t.TempDir(), "not-a-bundle.txt")
	if err := os.WriteFile(path, []byte("just some text, not yaml or gzip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := PlanImport(fresh, path); err == nil {
		t.Fatal("expected an error for unrecognized content, not a panic or silent success")
	}
}

func TestUnpackFlat_RejectsWrongAPIVersion(t *testing.T) {
	if _, _, err := unpackFlat([]byte("apiVersion: something-else/v1\n")); err == nil {
		t.Fatal("expected an error for a mismatched apiVersion")
	}
}

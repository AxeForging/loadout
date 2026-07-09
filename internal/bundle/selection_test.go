package bundle

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
	"github.com/AxeForging/loadout/internal/wizard"
)

func TestExportSelected_OnlyIncludesSelectedArtifacts(t *testing.T) {
	src := testStore(t, "team-kit")
	if _, err := src.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	if _, err := src.New(domain.KindSkill, "extra"); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "exported")
	meta, err := ExportSelected(src, dest, "test", Selection{ArtifactIDs: []string{"review"}})
	if err != nil {
		t.Fatalf("ExportSelected() error = %v", err)
	}
	if meta.Artifacts != 1 {
		t.Errorf("meta.Artifacts = %d, want 1", meta.Artifacts)
	}

	fresh := testStore(t, "mine")
	plan, err := PlanImport(fresh, dest)
	if err != nil {
		t.Fatalf("PlanImport() error = %v", err)
	}
	defer plan.Close()
	if len(plan.New) != 1 || plan.New[0].ID != "review" {
		t.Fatalf("plan.New = %v, want only [review]", plan.New)
	}

	if _, err := Apply(fresh, plan, false); err != nil {
		t.Fatal(err)
	}
	if _, ok := fresh.Artifact("extra"); ok {
		t.Error("unselected artifact 'extra' leaked into the export")
	}
	if _, err := os.Stat(filepath.Join(dest, "skills", "extra")); !os.IsNotExist(err) {
		t.Error("unselected artifact's files were copied into the export")
	}
}

func TestExportSelected_IncludesSelectedProfilesAndLibrary(t *testing.T) {
	src := testStore(t, "team-kit")
	if err := wizard.SaveProfile(src, "acme-backend", wizard.Answers{"toolchain.docker": true}); err != nil {
		t.Fatal(err)
	}
	if err := wizard.SaveProfile(src, "other-profile", wizard.Answers{}); err != nil {
		t.Fatal(err)
	}
	entry, err := wizard.AddToLibrary(src, "always write tests first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wizard.AddToLibrary(src, "unselected entry"); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "exported")
	_, err = ExportSelected(src, dest, "test", Selection{
		ProfileNames:    []string{"acme-backend"},
		LibraryEntryIDs: []string{entry.ID},
	})
	if err != nil {
		t.Fatalf("ExportSelected() error = %v", err)
	}

	exported, err := store.Open(dest)
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := wizard.ListProfiles(exported)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 || profiles[0] != "acme-backend" {
		t.Errorf("exported profiles = %v, want only [acme-backend]", profiles)
	}
	entries, err := wizard.ListLibrary(exported)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != entry.ID {
		t.Errorf("exported library = %v, want only the selected entry", entries)
	}
}

func TestExportSelected_SourcesOptIn(t *testing.T) {
	src := testStore(t, "team-kit")
	sourcesYAML := "sources:\n  - name: team-skills\n    url: https://example.com/team-skills.git\n    commit: abc123\n"
	if err := os.WriteFile(filepath.Join(src.Root, "sources.lock.yaml"), []byte(sourcesYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	withoutSources := filepath.Join(t.TempDir(), "no-sources")
	if _, err := ExportSelected(src, withoutSources, "test", Selection{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(withoutSources, "sources.lock.yaml")); !os.IsNotExist(err) {
		t.Error("sources.lock.yaml should be absent when Sources is false")
	}

	withSources := filepath.Join(t.TempDir(), "with-sources")
	if _, err := ExportSelected(src, withSources, "test", Selection{Sources: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(withSources, "sources.lock.yaml")); err != nil {
		t.Errorf("sources.lock.yaml should be present when Sources is true: %v", err)
	}
}

func TestExportSelected_RejectsUnknownIDs(t *testing.T) {
	src := testStore(t, "team-kit")
	dest := filepath.Join(t.TempDir(), "exported")

	if _, err := ExportSelected(src, dest, "test", Selection{ArtifactIDs: []string{"nope"}}); err == nil {
		t.Error("expected error for unknown artifact ID")
	}
	if _, err := ExportSelected(src, dest+"2", "test", Selection{ProfileNames: []string{"nope"}}); err == nil {
		t.Error("expected error for unknown profile name")
	}
	if _, err := ExportSelected(src, dest+"3", "test", Selection{LibraryEntryIDs: []string{"nope"}}); err == nil {
		t.Error("expected error for unknown library entry ID")
	}
}

func TestExportSelected_DestinationMustNotExist(t *testing.T) {
	src := testStore(t, "team-kit")
	dest := t.TempDir() // already exists
	if _, err := ExportSelected(src, dest, "test", Selection{}); err == nil {
		t.Error("expected error when destination already exists")
	}
}

func TestExportSelectedArchive_RoundTrip(t *testing.T) {
	src := testStore(t, "team-kit")
	if _, err := src.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "team.loadout.tar.gz")
	if _, err := ExportSelectedArchive(src, archive, "test", Selection{ArtifactIDs: []string{"review"}}); err != nil {
		t.Fatalf("ExportSelectedArchive() error = %v", err)
	}

	fresh := testStore(t, "mine")
	plan, err := PlanImport(fresh, archive)
	if err != nil {
		t.Fatalf("PlanImport(archive) error = %v", err)
	}
	defer plan.Close()
	if len(plan.New) != 1 || plan.New[0].ID != "review" {
		t.Fatalf("plan.New = %v, want only [review]", plan.New)
	}
}

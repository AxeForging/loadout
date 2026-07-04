package wizard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfiles_SaveListLoadRoundTrip(t *testing.T) {
	s := testStore(t)

	if names, err := ListProfiles(s); err != nil || len(names) != 0 {
		t.Fatalf("ListProfiles() on empty store = %v, %v", names, err)
	}

	a := Answers{"profile.appTypes": []string{"cli"}, "toolchain.docker": true}
	if err := SaveProfile(s, "acme-cli", a); err != nil {
		t.Fatalf("SaveProfile() error = %v", err)
	}

	names, err := ListProfiles(s)
	if err != nil {
		t.Fatalf("ListProfiles() error = %v", err)
	}
	if len(names) != 1 || names[0] != "acme-cli" {
		t.Fatalf("ListProfiles() = %v, want [acme-cli]", names)
	}

	loaded, err := LoadProfile(s, "acme-cli")
	if err != nil {
		t.Fatalf("LoadProfile() error = %v", err)
	}
	if !loaded.Bool("toolchain.docker") {
		t.Errorf("loaded profile lost toolchain.docker = true")
	}
	if got := loaded.List("profile.appTypes"); len(got) != 1 || got[0] != "cli" {
		t.Errorf("loaded profile appTypes = %v, want [cli]", got)
	}

	// Saved as a plain file in the store, so it's shareable via git/bundle.
	if _, err := os.Stat(filepath.Join(s.Root, "profiles", "acme-cli.yaml")); err != nil {
		t.Errorf("profile not written to store: %v", err)
	}
}

func TestProfiles_LoadUnknownReportsAvailableNames(t *testing.T) {
	s := testStore(t)
	if err := SaveProfile(s, "web-team", Answers{}); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProfile(s, "nope")
	if err == nil {
		t.Fatal("expected error for unknown profile")
	}
	if !strings.Contains(err.Error(), "web-team") {
		t.Errorf("error should list available profiles, got: %v", err)
	}
}

func TestProfiles_RemoveDeletesFile(t *testing.T) {
	s := testStore(t)
	if err := SaveProfile(s, "temp", Answers{}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveProfile(s, "temp"); err != nil {
		t.Fatalf("RemoveProfile() error = %v", err)
	}
	if _, err := LoadProfile(s, "temp"); err == nil {
		t.Error("profile should be gone after RemoveProfile")
	}
	if err := RemoveProfile(s, "temp"); err == nil {
		t.Error("removing an already-removed profile should error")
	}
}

func TestProfiles_RejectsInvalidNames(t *testing.T) {
	s := testStore(t)
	if err := SaveProfile(s, "../escape", Answers{}); err == nil {
		t.Error("path-traversal-style name should be rejected")
	}
	if err := SaveProfile(s, "Has Spaces", Answers{}); err == nil {
		t.Error("name with spaces should be rejected")
	}
}

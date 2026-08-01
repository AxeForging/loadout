package source

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

// upstream is a local git repository standing in for a remote skills repo.
type upstream struct {
	t   *testing.T
	dir string
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()
	u := &upstream{t: t, dir: filepath.Join(t.TempDir(), "skills-repo")}
	if err := os.MkdirAll(u.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	u.git("init", "-b", "main")
	u.git("config", "user.email", "test@example.com")
	u.git("config", "user.name", "test")
	return u
}

func (u *upstream) git(args ...string) string {
	u.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = u.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		u.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (u *upstream) writeSkill(name, body string) {
	u.t.Helper()
	dir := filepath.Join(u.dir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		u.t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: test skill\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		u.t.Fatal(err)
	}
}

func (u *upstream) commit(msg string) string {
	u.t.Helper()
	u.git("add", "-A")
	u.git("commit", "-m", msg)
	return u.git("rev-parse", "HEAD")
}

func newManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	return &Manager{Store: s, CacheDir: filepath.Join(dir, "cache")}
}

func TestAdd_ImportsSkillsAndPinsCommit(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "review things")
	u.writeSkill("deploy", "deploy things")
	commit := u.commit("initial")

	m := newManager(t)
	res, err := m.Add(context.Background(), "team-skills", u.dir, "main", "", nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if res.Ref.Commit != commit {
		t.Errorf("pinned %s, want %s", res.Ref.Commit, commit)
	}
	if len(res.Added) != 2 {
		t.Fatalf("Added = %v, want 2 skills", res.Added)
	}

	art, ok := m.Store.Artifact("review")
	if !ok {
		t.Fatal("review not in manifest")
	}
	if art.Origin != "source:team-skills" {
		t.Errorf("origin = %q", art.Origin)
	}
	if _, err := os.Stat(filepath.Join(m.Store.Root, art.Path, "SKILL.md")); err != nil {
		t.Errorf("skill content not copied: %v", err)
	}

	refs, err := m.Lock()
	if err != nil || len(refs) != 1 {
		t.Fatalf("lock = %v, err %v", refs, err)
	}
}

func TestAdd_SkipsArtifactsThatAlreadyExist(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "upstream version")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Store.New(domain.KindSkill, "review"); err != nil {
		t.Fatal(err)
	}
	res, err := m.Add(context.Background(), "src", u.dir, "main", "", nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	if len(res.Skipped) != 1 || res.Skipped[0] != "review" {
		t.Errorf("Skipped = %v", res.Skipped)
	}
	art, _ := m.Store.Artifact("review")
	if art.Origin != "local" {
		t.Errorf("existing artifact origin must stay local, got %q", art.Origin)
	}
}

func TestCheck_ReportsUpdateOnlyWhenRemoteMoved(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", nil); err != nil {
		t.Fatal(err)
	}

	results, err := m.Check(context.Background())
	if err != nil || len(results) != 1 {
		t.Fatalf("Check() = %v, err %v", results, err)
	}
	if results[0].HasUpdate {
		t.Error("no update expected right after add")
	}

	u.writeSkill("review", "v2")
	u.commit("update review")

	results, _ = m.Check(context.Background())
	if !results[0].HasUpdate {
		t.Error("update expected after upstream commit")
	}
}

func TestUpdate_AppliesChangedAndAddedSkillsAndMovesPin(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", nil); err != nil {
		t.Fatal(err)
	}

	u.writeSkill("review", "v2 improved")
	u.writeSkill("fresh", "brand new")
	newCommit := u.commit("improvements")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatalf("PlanUpdate() error = %v", err)
	}
	if plan.UpToDate {
		t.Fatal("plan should not be up-to-date")
	}
	if len(plan.Changed) != 1 || plan.Changed[0].Name != "review" {
		t.Errorf("Changed = %v", plan.Changed)
	}
	if len(plan.Added) != 1 || plan.Added[0].Name != "fresh" {
		t.Errorf("Added = %v", plan.Added)
	}
	if len(plan.Log) == 0 {
		t.Error("changelog should span the old pin")
	}

	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatalf("ApplyUpdate() error = %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "review", "SKILL.md"))
	if !strings.Contains(string(raw), "v2 improved") {
		t.Errorf("changed skill not refreshed:\n%s", raw)
	}
	if _, ok := m.Store.Artifact("fresh"); !ok {
		t.Error("new skill not imported")
	}
	refs, _ := m.Lock()
	if refs[0].Commit != newCommit {
		t.Errorf("pin = %s, want %s", refs[0].Commit, newCommit)
	}
}

func TestUpdate_NeverTouchesUserOwnedArtifactsAndRelabelsRemoved(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "v1")
	u.writeSkill("goner", "will vanish")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", nil); err != nil {
		t.Fatal(err)
	}

	// User takes ownership of "review" by editing it in the store.
	storeFile := filepath.Join(m.Store.Root, "skills", "review", "SKILL.md")
	if err := os.WriteFile(storeFile, []byte("my heavily customized version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// It stays origin source:src but is now locally edited — updates must
	// leave it alone. A user-owned (origin local) artifact must never be
	// overwritten either:
	if _, err := m.Store.New(domain.KindSkill, "mine"); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Join(u.dir, "goner")); err != nil {
		t.Fatal(err)
	}
	u.writeSkill("mine", "upstream impostor") // same name as the user's local skill
	u.commit("v2")

	plan, err := m.PlanUpdate(context.Background(), "src")
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range plan.Added {
		if sk.Name == "mine" {
			t.Error("upstream skill colliding with a local artifact must not be planned as added")
		}
	}
	for _, sk := range plan.Changed {
		if sk.Name == "mine" {
			t.Error("local artifact must never be planned as changed")
		}
	}
	if len(plan.RemovedIDs) != 1 || plan.RemovedIDs[0] != "goner" {
		t.Errorf("RemovedIDs = %v", plan.RemovedIDs)
	}

	if err := m.ApplyUpdate(plan, false); err != nil {
		t.Fatal(err)
	}
	art, _ := m.Store.Artifact("goner")
	if art.Origin != "local" {
		t.Errorf("removed-upstream artifact origin = %q, want local", art.Origin)
	}
	raw, _ := os.ReadFile(filepath.Join(m.Store.Root, "skills", "mine", "SKILL.md"))
	if strings.Contains(string(raw), "impostor") {
		t.Error("user-owned skill was overwritten by upstream")
	}
}

func TestRemove_PurgeDeletesImportedArtifacts(t *testing.T) {
	u := newUpstream(t)
	u.writeSkill("review", "v1")
	u.commit("initial")

	m := newManager(t)
	if _, err := m.Add(context.Background(), "src", u.dir, "main", "", nil); err != nil {
		t.Fatal(err)
	}
	removed, err := m.Remove("src", true)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if len(removed) != 1 || removed[0] != "review" {
		t.Errorf("removed = %v", removed)
	}
	if _, ok := m.Store.Artifact("review"); ok {
		t.Error("purged artifact still in manifest")
	}
	refs, _ := m.Lock()
	if len(refs) != 0 {
		t.Errorf("lock should be empty, got %v", refs)
	}
}

func TestDiscoverSkills_FindsNestedSkillsAndHonorsSubdir(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"skills/alpha", "skills/nested/beta", "other/gamma"} {
		dir := filepath.Join(root, p)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	all, err := DiscoverSkills(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Errorf("found %v, want alpha+beta+gamma", all)
	}

	scoped, err := DiscoverSkills(root, "skills")
	if err != nil {
		t.Fatal(err)
	}
	if len(scoped) != 2 {
		t.Errorf("subdir scope found %v, want alpha+beta", scoped)
	}
}

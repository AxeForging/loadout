package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/bundle"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/source"
	"github.com/AxeForging/loadout/internal/store"
)

func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindSkill, "demo"); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "claude-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(claudecode.NewWithHome(home))
	eng := &engine.Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")}
	return New(Options{Engine: eng}), home
}

func newTestServerWithSources(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	eng := &engine.Engine{Store: s, Registry: adapter.NewRegistry(), StatePath: filepath.Join(dir, "state.yaml")}
	mgr := &source.Manager{Store: s, CacheDir: filepath.Join(dir, "sources-cache")}
	return New(Options{Engine: eng, Sources: mgr})
}

// gitIn runs git in dir for building a fake upstream skills repo.
func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// makeUpstream builds a local git repo (cloneable by path) with one skill,
// standing in for an external skills repository.
func makeUpstream(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "team-skills")
	skill := filepath.Join(dir, "code-review")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: code-review\ndescription: review code\n---\n\nreview the diff\n"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-b", "main")
	gitIn(t, dir, "config", "user.email", "t@example.com")
	gitIn(t, dir, "config", "user.name", "t")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "initial")
	return dir
}

func TestAPI_SourcesAdd(t *testing.T) {
	// No Sources configured — the endpoint fails loudly instead of a silent no-op.
	srv, _ := newTestServer(t)
	rec, _ := doJSON(t, srv.Handler(), "POST", "/api/sources/add", map[string]string{"url": "https://example.com/x.git"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no sources configured = %d, want 503", rec.Code)
	}

	withSources := newTestServerWithSources(t)
	h := withSources.Handler()

	// Missing url is rejected before any clone is attempted.
	rec, _ = doJSON(t, h, "POST", "/api/sources/add", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing url = %d, want 400", rec.Code)
	}

	// Happy path: a real local repo is cloned, its skill imported, and pinned.
	upstream := makeUpstream(t)
	rec, out := doJSON(t, h, "POST", "/api/sources/add", map[string]string{"name": "team-skills", "url": upstream})
	if rec.Code != http.StatusOK {
		t.Fatalf("sources/add = %d: %s", rec.Code, rec.Body)
	}
	ref := out["ref"].(map[string]any)
	if ref["name"] != "team-skills" {
		t.Errorf("ref = %v, want name team-skills", ref)
	}
	added := out["added"].([]any)
	if len(added) != 1 || added[0] != "code-review" {
		t.Errorf("added = %v, want [code-review]", added)
	}
	// Skipped must be `[]`, never JSON null, or a naive frontend `.length`
	// access on an empty result blows up.
	skipped, ok := out["skipped"].([]any)
	if !ok || len(skipped) != 0 {
		t.Errorf("skipped = %#v, want empty array (not null)", out["skipped"])
	}

	req := httptest.NewRequest("GET", "/api/sources", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("sources list decode: %v: %s", err, rec.Body)
	}
	if len(list) != 1 || list[0]["name"] != "team-skills" {
		t.Errorf("sources list = %v, want one entry named team-skills", list)
	}
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestAPI_OverviewReportsPendingChanges(t *testing.T) {
	srv, _ := newTestServer(t)
	rec, out := doJSON(t, srv.Handler(), "GET", "/api/overview", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if out["pending"] != float64(1) {
		t.Errorf("pending = %v, want 1", out["pending"])
	}
	agents := out["agents"].([]any)
	if len(agents) != 1 {
		t.Errorf("agents = %v", agents)
	}
}

func TestAPI_TwoPhaseSyncGate(t *testing.T) {
	srv, home := newTestServer(t)
	h := srv.Handler()

	// Apply without a token is rejected.
	rec, _ := doJSON(t, h, "POST", "/api/sync/apply", map[string]string{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("apply without token = %d, want 400", rec.Code)
	}
	// Apply with a bogus token is rejected.
	rec, _ = doJSON(t, h, "POST", "/api/sync/apply", map[string]string{"token": "bogus"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply with bogus token = %d, want 409", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(home, "skills")); !os.IsNotExist(err) {
		t.Fatal("agent home written without a valid plan token")
	}

	// Plan → apply with the token works exactly once.
	rec, out := doJSON(t, h, "POST", "/api/sync/plan", map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	token := out["token"].(string)
	if len(out["changes"].([]any)) != 1 {
		t.Fatalf("changes = %v", out["changes"])
	}
	rec, out = doJSON(t, h, "POST", "/api/sync/apply", map[string]string{"token": token})
	if rec.Code != http.StatusOK || out["applied"] != float64(1) {
		t.Fatalf("apply = %d %v", rec.Code, out)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("skill not installed: %v", err)
	}
	// Token is single-use.
	rec, _ = doJSON(t, h, "POST", "/api/sync/apply", map[string]string{"token": token})
	if rec.Code != http.StatusConflict {
		t.Fatalf("token reuse = %d, want 409", rec.Code)
	}
}

func TestSyncApply_WritesConflictsOnlyWhenOverwriteRequested(t *testing.T) {
	srv, home := newTestServer(t)
	h := srv.Handler()

	// The user already has their own skills/demo/SKILL.md that loadout never installed.
	preexisting := filepath.Join(home, "skills", "demo", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(preexisting), 0o755); err != nil {
		t.Fatal(err)
	}
	original := "my own demo skill\n"
	if err := os.WriteFile(preexisting, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	// Plan surfaces it as a conflict, not a change.
	_, out := doJSON(t, h, "POST", "/api/sync/plan", map[string]any{})
	if len(out["changes"].([]any)) != 0 {
		t.Fatalf("changes = %v, want none", out["changes"])
	}
	if len(out["conflicts"].([]any)) != 1 {
		t.Fatalf("conflicts = %v, want 1", out["conflicts"])
	}

	// Apply WITHOUT overwrite leaves the file untouched.
	token := out["token"].(string)
	rec, out := doJSON(t, h, "POST", "/api/sync/apply", map[string]any{"token": token})
	if rec.Code != http.StatusOK || out["applied"] != float64(0) {
		t.Fatalf("apply = %d %v, want applied 0", rec.Code, out)
	}
	if raw, _ := os.ReadFile(preexisting); string(raw) != original {
		t.Errorf("default apply modified the conflicting file: %q", raw)
	}

	// Re-plan, then apply WITH overwrite writes it.
	_, out = doJSON(t, h, "POST", "/api/sync/plan", map[string]any{})
	token = out["token"].(string)
	rec, out = doJSON(t, h, "POST", "/api/sync/apply", map[string]any{"token": token, "overwriteConflicts": true})
	if rec.Code != http.StatusOK || out["applied"] != float64(1) {
		t.Fatalf("forced apply = %d %v, want applied 1", rec.Code, out)
	}
	if raw, _ := os.ReadFile(preexisting); string(raw) == original {
		t.Error("forced apply did not overwrite the conflicting file")
	}
}

func TestWizardPlan_ReturnsReservedNameWarnings(t *testing.T) {
	srv, _ := newTestServer(t)
	// sdd.enable produces a /plan command, which shadows Claude Code's built-in.
	_, out := doJSON(t, srv.Handler(), "POST", "/api/wizard/plan",
		map[string]any{"answers": map[string]any{"sdd.enable": true}})
	warnings, ok := out["warnings"].([]any)
	if !ok || len(warnings) == 0 {
		t.Fatalf("warnings = %v, want a reserved-name warning for /plan", out["warnings"])
	}
}

func TestAPI_ArtifactDetailReturnsFiles(t *testing.T) {
	srv, _ := newTestServer(t)
	rec, out := doJSON(t, srv.Handler(), "GET", "/api/artifacts/demo", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	files := out["files"].(map[string]any)
	if !strings.Contains(files["SKILL.md"].(string), "name: demo") {
		t.Errorf("files = %v", files)
	}

	rec, _ = doJSON(t, srv.Handler(), "GET", "/api/artifacts/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown artifact = %d, want 404", rec.Code)
	}
}

func TestAPI_WizardPlanAndGenerate(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec, out := doJSON(t, h, "POST", "/api/wizard/plan", map[string]any{"preset": "go-cli"})
	if rec.Code != http.StatusOK {
		t.Fatalf("wizard plan = %d: %s", rec.Code, rec.Body)
	}
	if len(out["create"].([]any)) == 0 {
		t.Fatalf("wizard plan should propose artifacts: %v", out)
	}

	rec, out = doJSON(t, h, "POST", "/api/wizard/generate", map[string]any{"preset": "go-cli"})
	if rec.Code != http.StatusOK {
		t.Fatalf("wizard generate = %d: %s", rec.Code, rec.Body)
	}
	if len(out["created"].([]any)) == 0 {
		t.Fatalf("wizard generate created nothing: %v", out)
	}

	// Invalid answers fail loudly.
	rec, _ = doJSON(t, h, "POST", "/api/wizard/plan", map[string]any{"answers": map[string]any{"conv.branching": "chaos"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid answers = %d, want 400", rec.Code)
	}

	// List answers survive JSON decode + validation and produce the
	// conditional custom-instructions artifact.
	rec, out = doJSON(t, h, "POST", "/api/wizard/generate", map[string]any{
		"answers": map[string]any{"standards.custom": []string{"always test", "no coverage theater"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("wizard generate with list answer = %d: %s", rec.Code, rec.Body)
	}
	created := out["created"].([]any)
	found := false
	for _, id := range created {
		if id == "custom-instructions" {
			found = true
		}
	}
	if !found {
		t.Errorf("wizard generate should create custom-instructions, got created=%v", created)
	}
}

func TestAPI_ProfilesSaveListGetDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	var names []string
	req := httptest.NewRequest("GET", "/api/profiles", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &names); err != nil || len(names) != 0 {
		t.Fatalf("profiles list on empty store = %v, %v", names, err)
	}

	rec, _ = doJSON(t, h, "POST", "/api/profiles", map[string]any{
		"name":    "acme-cli",
		"answers": map[string]any{"toolchain.docker": true},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save profile = %d: %s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest("GET", "/api/profiles", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &names)
	if len(names) != 1 || names[0] != "acme-cli" {
		t.Fatalf("profiles list after save = %v", names)
	}

	rec, out := doJSON(t, h, "GET", "/api/profiles/acme-cli", nil)
	if rec.Code != http.StatusOK || out["toolchain.docker"] != true {
		t.Fatalf("get profile = %d %v", rec.Code, out)
	}

	rec, _ = doJSON(t, h, "GET", "/api/profiles/nope", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown profile = %d, want 404", rec.Code)
	}

	req = httptest.NewRequest("DELETE", "/api/profiles/acme-cli", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete profile = %d: %s", rec.Code, rec.Body)
	}
	rec, _ = doJSON(t, h, "GET", "/api/profiles/acme-cli", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("profile should be gone after delete, got %d", rec.Code)
	}
}

func TestAPI_LibraryAddListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	req := httptest.NewRequest("GET", "/api/library/instructions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 0 {
		t.Fatalf("library list on empty store = %v, %v", list, err)
	}

	rec, out := doJSON(t, h, "POST", "/api/library/instructions", map[string]string{"text": "always write tests first"})
	if rec.Code != http.StatusOK {
		t.Fatalf("add library entry = %d: %s", rec.Code, rec.Body)
	}
	id, _ := out["id"].(string)
	if id == "" || out["text"] != "always write tests first" {
		t.Fatalf("add library entry response = %v", out)
	}

	// Adding identical text again returns the same entry, not a duplicate.
	rec, out = doJSON(t, h, "POST", "/api/library/instructions", map[string]string{"text": "always write tests first"})
	if rec.Code != http.StatusOK || out["id"] != id {
		t.Fatalf("duplicate add = %d %v, want same id %q", rec.Code, out, id)
	}

	rec, _ = doJSON(t, h, "POST", "/api/library/instructions", map[string]string{"text": "   "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("add empty text = %d, want 400", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/library/instructions", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("library list after adds = %v, want exactly one entry", list)
	}

	req = httptest.NewRequest("DELETE", "/api/library/instructions/"+id, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete library entry = %d: %s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest("DELETE", "/api/library/instructions/"+id, nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete already-deleted entry = %d, want 404", rec.Code)
	}
}

func TestAPI_LibraryGroupsSaveListDelete(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec, out := doJSON(t, h, "POST", "/api/library/instructions", map[string]string{"text": "always write tests first"})
	if rec.Code != http.StatusOK {
		t.Fatalf("add library entry = %d: %s", rec.Code, rec.Body)
	}
	entryID, _ := out["id"].(string)

	req := httptest.NewRequest("GET", "/api/library/groups", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var list []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 0 {
		t.Fatalf("groups list on empty store = %v, %v", list, err)
	}

	rec, out = doJSON(t, h, "POST", "/api/library/groups", map[string]any{
		"name": "testing-rules", "entryIds": []string{entryID},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("save group = %d: %s", rec.Code, rec.Body)
	}
	if out["name"] != "testing-rules" {
		t.Fatalf("save group response = %v", out)
	}

	// Unknown entry ID is rejected.
	rec, _ = doJSON(t, h, "POST", "/api/library/groups", map[string]any{
		"name": "bad-group", "entryIds": []string{"does-not-exist"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("save group with unknown entry = %d, want 400", rec.Code)
	}

	req = httptest.NewRequest("GET", "/api/library/groups", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["name"] != "testing-rules" {
		t.Fatalf("groups list after save = %v", list)
	}

	req = httptest.NewRequest("DELETE", "/api/library/groups/testing-rules", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete group = %d: %s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest("DELETE", "/api/library/groups/testing-rules", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete already-deleted group = %d, want 404", rec.Code)
	}

	// Deleting the group must not have touched the underlying entry.
	req = httptest.NewRequest("GET", "/api/library/instructions", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var entries []map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &entries)
	if len(entries) != 1 {
		t.Fatalf("library entries after group delete = %v, want the entry untouched", entries)
	}
}

func TestAPI_BundleExportSelected(t *testing.T) {
	srv, _ := newTestServer(t)
	// newTestServer seeds "demo"; add a second artifact to prove filtering.
	if _, err := srv.opts.Engine.Store.New(domain.KindSkill, "extra"); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{"artifactIds": []string{"demo"}})
	req := httptest.NewRequest("POST", "/api/bundle/export", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export selected = %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/gzip" {
		t.Errorf("Content-Type = %q, want application/gzip", ct)
	}

	archive := filepath.Join(t.TempDir(), "selected.loadout.tar.gz")
	if err := os.WriteFile(archive, rec.Body.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh, err := store.Init(filepath.Join(t.TempDir(), "store"), "fresh")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := bundle.PlanImport(fresh, archive)
	if err != nil {
		t.Fatalf("PlanImport() on the streamed archive error = %v", err)
	}
	defer plan.Close()
	if len(plan.New) != 1 || plan.New[0].ID != "demo" {
		t.Fatalf("plan.New = %v, want only [demo] — 'extra' should not have been included", plan.New)
	}
}

func TestAPI_BundleExportSelected_UnknownArtifactRejected(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{"artifactIds": []string{"does-not-exist"}})
	req := httptest.NewRequest("POST", "/api/bundle/export", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("export with unknown artifact = %d, want 400", rec.Code)
	}
}

func TestAPI_BundleExportFlat(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]any{"artifactIds": []string{"demo"}})
	req := httptest.NewRequest("POST", "/api/bundle/export/flat", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("export flat = %d: %s", rec.Code, rec.Body)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got["yaml"], "apiVersion: loadout-config/v1") {
		t.Errorf("flat export missing apiVersion:\n%s", got["yaml"])
	}
	if !strings.Contains(got["yaml"], "id: demo") {
		t.Errorf("flat export missing the selected artifact:\n%s", got["yaml"])
	}
}

func TestAPI_BundleImportPlan_AcceptsPasteYAML(t *testing.T) {
	source, _ := newTestServer(t) // seeds "demo"
	raw, err := bundle.ExportFlat(source.opts.Engine.Store, "test", bundle.Selection{ArtifactIDs: []string{"demo"}})
	if err != nil {
		t.Fatal(err)
	}

	// Import into a different, empty store — newTestServer's own store
	// already has "demo" seeded, which would make this a no-op "identical"
	// import instead of exercising the "new" path.
	srv := newTestServerWithSources(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"yaml": string(raw)})
	req := httptest.NewRequest("POST", "/api/bundle/import/plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("import plan (paste YAML) = %d: %s", rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("expected a plan token")
	}
	newArtifacts, _ := out["new"].([]any)
	if len(newArtifacts) != 1 {
		t.Fatalf("plan.new = %v, want one artifact", out["new"])
	}

	applyBody, _ := json.Marshal(map[string]any{"token": token})
	applyReq := httptest.NewRequest("POST", "/api/bundle/import/apply", bytes.NewReader(applyBody))
	applyRec := httptest.NewRecorder()
	h.ServeHTTP(applyRec, applyReq)
	if applyRec.Code != http.StatusOK {
		t.Fatalf("import apply (paste YAML plan) = %d: %s", applyRec.Code, applyRec.Body)
	}
}

func TestAPI_BundleImportPlan_RejectsEmptyPasteYAML(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	body, _ := json.Marshal(map[string]string{"yaml": "   "})
	req := httptest.NewRequest("POST", "/api/bundle/import/plan", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("import plan with empty yaml = %d, want 400", rec.Code)
	}
}

func TestSPA_FallbackServesIndexForClientRoutes(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, path := range []string{"/", "/agents", "/store/some-artifact"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<html") {
			t.Errorf("%s: code %d, body %q", path, rec.Code, rec.Body.String()[:min(80, rec.Body.Len())])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

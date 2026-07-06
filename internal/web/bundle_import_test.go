package web

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AxeForging/loadout/internal/bundle"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

// makeBundleArchive builds a .loadout.tar.gz from a fresh store populated by
// setup, standing in for a teammate's exported bundle.
func makeBundleArchive(t *testing.T, name string, setup func(s *store.Store)) string {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "src-store"), name)
	if err != nil {
		t.Fatal(err)
	}
	setup(s)
	archive := filepath.Join(dir, "bundle.loadout.tar.gz")
	if _, err := bundle.ExportArchive(s, archive, "test"); err != nil {
		t.Fatal(err)
	}
	return archive
}

func doMultipartUpload(t *testing.T, h http.Handler, path, fieldName string, content []byte) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(fieldName, "bundle.loadout.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec, out
}

func TestAPI_BundleImportPlan_ValidBundleReturnsDiffWithoutWriting(t *testing.T) {
	srv, _ := newTestServer(t) // store already has artifact "demo"
	h := srv.Handler()

	archive := makeBundleArchive(t, "team-kit", func(s *store.Store) {
		if _, err := s.New(domain.KindSkill, "extra"); err != nil {
			t.Fatal(err)
		}
	})
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}

	rec, out := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	if out["token"] == "" || out["token"] == nil {
		t.Fatalf("missing token: %v", out)
	}
	// Meta must serialize with the same camelCase JSON keys the frontend
	// (BundleMeta in api.ts) expects — bundle.Meta only had yaml tags before
	// this feature, so a naive json.Marshal produced Go field names instead.
	meta, ok := out["meta"].(map[string]any)
	if !ok || meta["name"] != "team-kit" {
		t.Fatalf("meta = %#v, want camelCase name=team-kit", out["meta"])
	}
	if meta["createdBy"] != "test" {
		t.Errorf("meta.createdBy = %v, want %q", meta["createdBy"], "test")
	}
	newArtifacts := out["new"].([]any)
	if len(newArtifacts) != 1 {
		t.Fatalf("new = %v, want 1 artifact", newArtifacts)
	}
	if out["conflicts"].([]any) == nil || len(out["conflicts"].([]any)) != 0 {
		t.Errorf("conflicts = %v, want empty", out["conflicts"])
	}

	// Nothing written yet — plan-only.
	if _, ok := srv.opts.Engine.Store.Artifact("extra"); ok {
		t.Fatal("artifact written to store before apply")
	}
}

func TestAPI_BundleImportPlan_RejectsNonGzipUpload(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec, out := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", []byte("not a real archive"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-gzip upload = %d, want 400: %v", rec.Code, out)
	}
}

func TestAPI_BundleImportPlan_RejectsBundleWithoutMeta(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	// A gzip'd tar with no bundle.yaml entry (i.e. not a loadout bundle at all).
	var gzBuf bytes.Buffer
	gz := gzip.NewWriter(&gzBuf)
	tw := tar.NewWriter(gz)
	body := []byte("hello")
	if err := tw.WriteHeader(&tar.Header{Name: "plain.txt", Size: int64(len(body)), Mode: 0o644}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}

	rec, out := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", gzBuf.Bytes())
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("archive missing bundle.yaml = %d, want 400: %v", rec.Code, out)
	}
}

func TestAPI_BundleImportPlan_RejectsOversizedUpload(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	oversized := make([]byte, maxBundleUploadSize+1024)
	rec, _ := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", oversized)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized upload = %d, want 413", rec.Code)
	}
}

func TestAPI_BundleImportApply_TokenLifecycle(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	// Apply without a token is rejected.
	rec, _ := doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("apply without token = %d, want 400", rec.Code)
	}
	// Apply with a bogus token is rejected.
	rec, _ = doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]string{"token": "bogus"})
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply with bogus token = %d, want 409", rec.Code)
	}

	archive := makeBundleArchive(t, "team-kit", func(s *store.Store) {
		if _, err := s.New(domain.KindSkill, "extra"); err != nil {
			t.Fatal(err)
		}
	})
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	rec, planOut := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	token := planOut["token"].(string)

	rec, applyOut := doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]any{"token": token, "overwrite": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d: %s", rec.Code, rec.Body)
	}
	applied := applyOut["applied"].([]any)
	if len(applied) != 1 || applied[0] != "extra" {
		t.Fatalf("applied = %v", applied)
	}
	if _, ok := srv.opts.Engine.Store.Artifact("extra"); !ok {
		t.Fatal("artifact not written into store after apply")
	}

	// Token is single-use.
	rec, _ = doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]any{"token": token})
	if rec.Code != http.StatusConflict {
		t.Fatalf("token reuse = %d, want 409", rec.Code)
	}
}

func TestAPI_BundleImportApply_ConflictSkipVsOverwrite(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	localSkill := filepath.Join(srv.opts.Engine.Store.Root, "skills", "demo", "SKILL.md")
	if err := os.WriteFile(localSkill, []byte("---\nname: demo\ndescription: local\n---\n\nlocal version\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := makeBundleArchive(t, "team-kit", func(s *store.Store) {
		if _, err := s.New(domain.KindSkill, "demo"); err != nil {
			t.Fatal(err)
		}
		skill := filepath.Join(s.Root, "skills", "demo", "SKILL.md")
		if err := os.WriteFile(skill, []byte("---\nname: demo\ndescription: bundle\n---\n\nbundle version\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	})
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}

	// Plan/apply with overwrite=false: conflict is left alone.
	rec, planOut := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	conflicts := planOut["conflicts"].([]any)
	if len(conflicts) != 1 {
		t.Fatalf("conflicts = %v, want 1", conflicts)
	}
	token := planOut["token"].(string)
	rec, applyOut := doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]any{"token": token, "overwrite": false})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply(skip) = %d: %s", rec.Code, rec.Body)
	}
	if applied := applyOut["applied"].([]any); len(applied) != 0 {
		t.Fatalf("applied with skip policy = %v, want none", applied)
	}
	raw, _ := os.ReadFile(localSkill)
	if !bytes.Contains(raw, []byte("local version")) {
		t.Fatal("skip policy overwrote local content")
	}

	// Re-plan and apply with overwrite=true: conflict replaced.
	rec, planOut = doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-plan = %d: %s", rec.Code, rec.Body)
	}
	token = planOut["token"].(string)
	rec, applyOut = doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]any{"token": token, "overwrite": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply(overwrite) = %d: %s", rec.Code, rec.Body)
	}
	if applied := applyOut["applied"].([]any); len(applied) != 1 || applied[0] != "demo" {
		t.Fatalf("applied with overwrite policy = %v", applied)
	}
	raw, _ = os.ReadFile(localSkill)
	if !bytes.Contains(raw, []byte("bundle version")) {
		t.Fatal("overwrite policy did not replace local content")
	}
}

func TestAPI_BundleImportPlan_EvictsExpiredPlansAndReleasesTempFiles(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	archive := makeBundleArchive(t, "team-kit", func(s *store.Store) {
		if _, err := s.New(domain.KindSkill, "extra"); err != nil {
			t.Fatal(err)
		}
	})
	content, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}

	rec, planOut := doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	staleToken := planOut["token"].(string)

	srv.mu.Lock()
	pending := srv.bundlePlans[staleToken]
	pending.created = pending.created.Add(-11 * time.Minute)
	srv.bundlePlans[staleToken] = pending
	tempFile := pending.tempFile
	srv.mu.Unlock()

	// A second plan call triggers the eviction sweep.
	rec, _ = doMultipartUpload(t, h, "/api/bundle/import/plan", "bundle", content)
	if rec.Code != http.StatusOK {
		t.Fatalf("second plan = %d: %s", rec.Code, rec.Body)
	}

	rec, _ = doJSON(t, h, "POST", "/api/bundle/import/apply", map[string]string{"token": staleToken})
	if rec.Code != http.StatusConflict {
		t.Fatalf("apply with evicted token = %d, want 409", rec.Code)
	}
	if _, err := os.Stat(tempFile); !os.IsNotExist(err) {
		t.Fatal("evicted plan's temp upload file was not removed")
	}
}

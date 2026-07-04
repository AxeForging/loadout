package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
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

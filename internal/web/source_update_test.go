package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// editStoreSkillFile rewrites a skill's SKILL.md inside the server's store.
func editStoreSkillFile(t *testing.T, srv *Server, id, body string) {
	t.Helper()
	path := filepath.Join(srv.opts.Engine.Store.Root, "skills", id, "SKILL.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commitUpstream(t *testing.T, dir, skill, body string) {
	t.Helper()
	content := "---\nname: " + skill + "\ndescription: x\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, skill, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-m", "upstream change")
}

func TestAPI_SourcesUpdate_PlanClassifiesAndApplyRespectsForceDiscard(t *testing.T) {
	srv := newTestServerWithSources(t)
	h := srv.Handler()
	upstream := makeUpstream(t)
	if rec, _ := doJSON(t, h, "POST", "/api/sources/add", map[string]string{"name": "team", "url": upstream}); rec.Code != http.StatusOK {
		t.Fatalf("add = %d: %s", rec.Code, rec.Body)
	}

	// Local edit + upstream change = conflict.
	editStoreSkillFile(t, srv, "code-review", "MY local version")
	commitUpstream(t, upstream, "code-review", "upstream v2")

	rec, out := doJSON(t, h, "POST", "/api/sources/update/plan", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d: %s", rec.Code, rec.Body)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatal("plan should return a token")
	}
	plans := out["plans"].([]any)
	if len(plans) != 1 {
		t.Fatalf("plans = %v", plans)
	}
	conflicts := plans[0].(map[string]any)["conflicts"].([]any)
	if len(conflicts) != 1 || conflicts[0] != "code-review" {
		t.Fatalf("conflicts = %v, want [code-review]", conflicts)
	}

	// Apply WITHOUT forceDiscard: local edit survives.
	rec, _ = doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{"token": token})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply = %d: %s", rec.Code, rec.Body)
	}
	raw, _ := os.ReadFile(filepath.Join(srv.opts.Engine.Store.Root, "skills", "code-review", "SKILL.md"))
	if string(raw) != "MY local version" {
		t.Fatalf("apply without forceDiscard clobbered the local edit:\n%s", raw)
	}

	// Re-plan (pin now up-to-date, conflict still reported), apply WITH force.
	rec, out = doJSON(t, h, "POST", "/api/sources/update/plan", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("re-plan = %d: %s", rec.Code, rec.Body)
	}
	token = out["token"].(string)
	plans = out["plans"].([]any)
	if got := plans[0].(map[string]any)["conflicts"].([]any); len(got) != 1 {
		t.Fatalf("conflict lost after pin move: %v", got)
	}
	rec, _ = doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{"token": token, "forceDiscard": true})
	if rec.Code != http.StatusOK {
		t.Fatalf("forced apply = %d: %s", rec.Code, rec.Body)
	}
	raw, _ = os.ReadFile(filepath.Join(srv.opts.Engine.Store.Root, "skills", "code-review", "SKILL.md"))
	if !strings.Contains(string(raw), "upstream v2") {
		t.Fatalf("forced apply should take upstream:\n%s", raw)
	}
}

func TestAPI_SourcesUpdate_TokenGate(t *testing.T) {
	srv := newTestServerWithSources(t)
	h := srv.Handler()

	rec, _ := doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("apply without token = %d, want 400", rec.Code)
	}
	rec, _ = doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{"token": "bogus"})
	if rec.Code != http.StatusConflict {
		t.Errorf("apply with unknown token = %d, want 409", rec.Code)
	}

	upstream := makeUpstream(t)
	if rec, _ := doJSON(t, h, "POST", "/api/sources/add", map[string]string{"name": "team", "url": upstream}); rec.Code != http.StatusOK {
		t.Fatalf("add failed: %s", rec.Body)
	}
	commitUpstream(t, upstream, "code-review", "v2")
	_, out := doJSON(t, h, "POST", "/api/sources/update/plan", nil)
	token := out["token"].(string)

	if rec, _ := doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{"token": token}); rec.Code != http.StatusOK {
		t.Fatalf("first apply = %d", rec.Code)
	}
	// One-shot: the same token must not apply twice.
	if rec, _ := doJSON(t, h, "POST", "/api/sources/update/apply", map[string]any{"token": token}); rec.Code != http.StatusConflict {
		t.Errorf("token reuse = %d, want 409", rec.Code)
	}
}

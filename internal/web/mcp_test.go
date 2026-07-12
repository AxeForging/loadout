package web

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPI_MCPCreateAndList(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	rec, out := doJSON(t, h, "POST", "/api/mcp", map[string]any{
		"name":      "context7",
		"transport": "stdio",
		"command":   "npx",
		"args":      []string{"-y", "@upstash/context7-mcp"},
		"env":       map[string]string{"API_TOKEN": "${CONTEXT7_TOKEN}", "": "  "},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body)
	}
	if out["id"] != "context7" {
		t.Fatalf("created id = %v", out["id"])
	}

	rec, _ = doJSON(t, h, "GET", "/api/mcp", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	for _, want := range []string{`"id":"context7"`, `"transport":"stdio"`, `"scope":"user"`, "${CONTEXT7_TOKEN}"} {
		if !strings.Contains(body, want) {
			t.Errorf("list missing %s in %s", want, body)
		}
	}

	// Duplicate names are rejected — nothing silently overwritten.
	rec, out = doJSON(t, h, "POST", "/api/mcp", map[string]any{
		"name": "context7", "transport": "stdio", "command": "npx",
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(out["error"].(string), "already exists") {
		t.Errorf("duplicate = %d %v, want 400 already-exists", rec.Code, out)
	}
}

func TestAPI_MCPCreateRejectsLiteralSecrets(t *testing.T) {
	srv, _ := newTestServer(t)
	rec, out := doJSON(t, srv.Handler(), "POST", "/api/mcp", map[string]any{
		"name": "leaky", "transport": "http", "url": "https://mcp.example.com",
		"headers": map[string]string{"Authorization": "Bearer sk-live-1234"},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("literal secret accepted: %d %s", rec.Code, rec.Body)
	}
	if !strings.Contains(out["error"].(string), "placeholder") {
		t.Errorf("error should teach the ${VAR} rule, got %v", out["error"])
	}
	// Nothing landed in the store.
	rec, _ = doJSON(t, srv.Handler(), "GET", "/api/mcp", nil)
	if strings.Contains(rec.Body.String(), "leaky") {
		t.Error("rejected MCP definition still appeared in the list")
	}
}

func TestAPI_MCPCreateValidatesTransportShape(t *testing.T) {
	srv, _ := newTestServer(t)
	// stdio without a command is invalid; so is http without a url.
	for _, body := range []map[string]any{
		{"name": "no-cmd", "transport": "stdio"},
		{"name": "no-url", "transport": "http"},
		{"name": "Bad Name", "transport": "stdio", "command": "npx"},
	} {
		rec, _ := doJSON(t, srv.Handler(), "POST", "/api/mcp", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %v = %d, want 400", body, rec.Code)
		}
	}
}

package web

import (
	"net/http"
	"os"
	"path/filepath"
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

func TestAPI_MCPDiscoverAndAdoptFromAgentConfig(t *testing.T) {
	srv, _ := newTestServer(t)
	h := srv.Handler()

	// The fake user home carries a hand-configured ~/.claude.json with two
	// servers — one with a literal secret that must never reach the browser.
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	claudeJSON := `{
	  "mcpServers": {
	    "My_GitHub": {
	      "type": "stdio", "command": "npx",
	      "args": ["-y", "@modelcontextprotocol/server-github"],
	      "env": {"GITHUB_TOKEN": "ghp_live_secret"}
	    },
	    "docs": {"type": "http", "url": "https://mcp.docs.dev"}
	  },
	  "someOtherUserSetting": true
	}`
	if err := os.WriteFile(filepath.Join(userHome, ".claude.json"), []byte(claudeJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	rec, _ := doJSON(t, h, "GET", "/api/mcp/agents", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("discover = %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "ghp_live_secret") {
		t.Fatal("literal secret leaked into the discovery response")
	}
	for _, want := range []string{`"name":"My_GitHub"`, `"storeName":"my-github"`, "${GITHUB_TOKEN}", `"name":"docs"`} {
		if !strings.Contains(body, want) {
			t.Errorf("discovery missing %s in %s", want, body)
		}
	}

	// Adopt one entry: it lands in the store (sanitized name, masked secret)...
	rec, out := doJSON(t, h, "POST", "/api/mcp/adopt", map[string]string{"agent": "claude-code", "name": "My_GitHub"})
	if rec.Code != http.StatusOK {
		t.Fatalf("adopt = %d: %s", rec.Code, rec.Body)
	}
	if out["id"] != "my-github" {
		t.Errorf("adopted id = %v", out["id"])
	}
	rec, _ = doJSON(t, h, "GET", "/api/mcp", nil)
	if !strings.Contains(rec.Body.String(), `"id":"my-github"`) || !strings.Contains(rec.Body.String(), "${GITHUB_TOKEN}") {
		t.Errorf("adopted server not in store list: %s", rec.Body)
	}
	// ...and the agent's own config file was NOT touched (read-only adopt).
	raw, err := os.ReadFile(filepath.Join(userHome, ".claude.json"))
	if err != nil || string(raw) != claudeJSON {
		t.Error("adopt must not modify the agent's config file")
	}

	// Unknown entries 404 instead of silently inventing something.
	rec, _ = doJSON(t, h, "POST", "/api/mcp/adopt", map[string]string{"agent": "claude-code", "name": "nope"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("adopt unknown = %d, want 404", rec.Code)
	}
}

package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/AxeForging/loadout/internal/domain"
)

// MCP management endpoints. Creating an MCP definition writes into the
// canonical STORE only — installing it into agent configs still goes through
// the two-phase sync gate like every other artifact.

// mcpSpecJSON mirrors domain.MCPSpec on the wire (the domain type carries
// yaml tags for mcp.yaml; the API speaks camelCase JSON like the rest).
type mcpSpecJSON struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Scope     string            `json:"scope"`
}

type mcpServerJSON struct {
	ID     string       `json:"id"`
	Path   string       `json:"path"`
	Origin string       `json:"origin"`
	Spec   *mcpSpecJSON `json:"spec,omitempty"`
	// Error carries a parse failure for a hand-edited mcp.yaml, so the UI
	// can still list the artifact instead of hiding it.
	Error string `json:"error,omitempty"`
}

func specToJSON(m domain.MCPSpec) *mcpSpecJSON {
	return &mcpSpecJSON{
		Transport: m.Transport,
		Command:   m.Command,
		Args:      m.Args,
		Env:       m.Env,
		URL:       m.URL,
		Headers:   m.Headers,
		Scope:     m.Scope,
	}
}

func (s *Server) handleMCPList(w http.ResponseWriter, r *http.Request) {
	st := s.opts.Engine.Store
	out := []mcpServerJSON{}
	for _, art := range st.Manifest.Artifacts {
		if art.Kind != domain.KindMCP {
			continue
		}
		row := mcpServerJSON{ID: art.ID, Path: art.Path, Origin: art.Origin}
		content, err := st.Content(art)
		if err != nil {
			row.Error = err.Error()
			out = append(out, row)
			continue
		}
		spec, err := domain.ParseMCPSpec(content.Files["mcp.yaml"])
		if err != nil {
			row.Error = err.Error()
		} else {
			row.Spec = specToJSON(spec)
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleMCPCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		mcpSpecJSON
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return
	}
	spec := domain.MCPSpec{
		Transport: req.Transport,
		Command:   strings.TrimSpace(req.Command),
		Args:      req.Args,
		Env:       dropEmptyPairs(req.Env),
		URL:       strings.TrimSpace(req.URL),
		Headers:   dropEmptyPairs(req.Headers),
		Scope:     req.Scope,
	}
	// Written files carry the defaults explicitly — a shared store should
	// read the same on every machine without knowing the defaulting rules.
	if spec.Transport == "" {
		spec.Transport = domain.MCPTransportStdio
	}
	if spec.Scope == "" {
		spec.Scope = domain.MCPScopeUser
	}
	raw, err := yaml.Marshal(spec)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	mcpYAML := []byte(mcpFileHeader + string(raw))
	art, err := s.opts.Engine.Store.CreateMCP(strings.TrimSpace(req.Name), mcpYAML)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":   art.ID,
		"path": art.Path,
	})
}

const mcpFileHeader = `# Canonical MCP server definition (created via the loadout web UI).
# Secrets NEVER go in this file: use ${VAR} placeholders — they resolve from
# the environment at agent runtime, so the store stays shareable.
`

// dropEmptyPairs removes entries whose key or value trimmed to nothing —
// blank form rows must not become "": "" pairs in a committed mcp.yaml.
func dropEmptyPairs(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

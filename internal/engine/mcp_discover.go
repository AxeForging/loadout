package engine

import (
	"context"
	"encoding/json"
	"os"
	"sort"

	"github.com/AxeForging/loadout/internal/domain"
)

// UnmanagedMCPServer is an MCP entry found in an installed agent's native
// config that no store artifact manages — adoptable gear, mirroring what
// `loadout add` does for skills. Secret-looking literal values are already
// masked to ${VAR} placeholders before this struct exists, so they never
// reach a caller (or a browser).
type UnmanagedMCPServer struct {
	Agent      domain.AgentID
	Name       string
	Spec       domain.MCPSpec
	File       string
	MaskedKeys []string
}

// probeMCP renders a synthetic user-scope MCP artifact through an adapter to
// learn where that agent keeps its MCP config — adapters own the path
// knowledge; nothing here hardcodes per-agent layouts.
var probeMCP = struct {
	art     domain.Artifact
	content domain.Content
}{
	art: domain.Artifact{ID: "loadout-probe", Kind: domain.KindMCP},
	content: domain.Content{Files: map[string][]byte{
		"mcp.yaml": []byte("transport: stdio\ncommand: loadout-probe\nscope: user\n"),
	}},
}

// DiscoverUnmanagedMCP reads each installed, MCP-capable agent's user-scope
// MCP config and returns entries the store does not manage. Read-only —
// nothing is written anywhere.
func (e *Engine) DiscoverUnmanagedMCP(ctx context.Context) ([]UnmanagedMCPServer, error) {
	managed := map[string]bool{}
	for _, art := range e.Store.Manifest.Artifacts {
		if art.Kind == domain.KindMCP {
			managed[art.ID] = true
		}
	}

	out := []UnmanagedMCPServer{}
	for _, ad := range e.Registry.All() {
		if !ad.Supports(domain.KindMCP) {
			continue
		}
		det, err := ad.Detect(ctx)
		if err != nil || !det.Installed {
			continue
		}
		targets, err := ad.Render(probeMCP.art, probeMCP.content)
		if err != nil || len(targets) == 0 {
			continue
		}
		base, err := e.resolveBase(targets[0].Root, det.Home)
		if err != nil {
			continue
		}
		abs, err := joinUnder(base, targets[0].RelPath)
		if err != nil {
			continue
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			continue // no MCP config on this machine — nothing to adopt
		}
		var doc struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			continue // unparseable config: discovery stays quiet, doctor is the loud path
		}
		for name, entry := range doc.MCPServers {
			if managed[name] {
				continue
			}
			spec, err := mcpSpecFromAgentEntry(entry)
			if err != nil {
				continue
			}
			spec, masked := spec.MaskSecretValues()
			out = append(out, UnmanagedMCPServer{
				Agent: ad.ID(), Name: name, Spec: spec, File: abs, MaskedKeys: masked,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Agent != out[j].Agent {
			return out[i].Agent < out[j].Agent
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// mcpSpecFromAgentEntry reverse-maps the common agent server object
// (command/args/env for stdio; type/url/headers for remote, including
// gemini's httpUrl variant) onto the canonical spec. Adoption re-validates
// through domain.ParseMCPSpec before anything is written.
func mcpSpecFromAgentEntry(raw json.RawMessage) (domain.MCPSpec, error) {
	var entry struct {
		Type    string            `json:"type"`
		Command string            `json:"command"`
		Args    []string          `json:"args"`
		Env     map[string]string `json:"env"`
		URL     string            `json:"url"`
		HTTPURL string            `json:"httpUrl"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return domain.MCPSpec{}, err
	}
	spec := domain.MCPSpec{
		Command: entry.Command,
		Args:    entry.Args,
		Env:     entry.Env,
		URL:     entry.URL,
		Headers: entry.Headers,
		Scope:   domain.MCPScopeUser,
	}
	if spec.URL == "" {
		spec.URL = entry.HTTPURL
	}
	switch {
	case entry.Type != "":
		spec.Transport = entry.Type
	case spec.Command != "":
		spec.Transport = domain.MCPTransportStdio
	default:
		spec.Transport = domain.MCPTransportHTTP
	}
	return spec, nil
}

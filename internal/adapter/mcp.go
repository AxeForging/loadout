package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/AxeForging/loadout/internal/domain"
)

// ParseMCP decodes and validates an mcp artifact's mcp.yaml.
func ParseMCP(art domain.Artifact, content domain.Content) (domain.MCPSpec, error) {
	raw, ok := content.Files["mcp.yaml"]
	if !ok {
		return domain.MCPSpec{}, fmt.Errorf("mcp %s: missing mcp.yaml", art.ID)
	}
	spec, err := domain.ParseMCPSpec(raw)
	if err != nil {
		return domain.MCPSpec{}, fmt.Errorf("mcp %s: %w", art.ID, err)
	}
	return spec, nil
}

// MCPServersFragment renders the {"mcpServers": {<id>: <server>}} JSON
// fragment shared by every mcpServers-style agent config.
func MCPServersFragment(id string, server map[string]any) ([]byte, error) {
	return json.Marshal(map[string]any{
		"mcpServers": map[string]any{id: server},
	})
}

// MCPServerObject builds the common JSON server object most agents share:
// command/args/env for stdio, url/headers otherwise. includeType adds a
// "type" field carrying the transport (claude-code's format). Agents with
// diverging field names (gemini's httpUrl) build their own map instead.
func MCPServerObject(spec domain.MCPSpec, includeType bool) map[string]any {
	server := map[string]any{}
	if includeType {
		server["type"] = spec.Transport
	}
	if spec.Transport == domain.MCPTransportStdio {
		server["command"] = spec.Command
		if len(spec.Args) > 0 {
			server["args"] = spec.Args
		}
		if len(spec.Env) > 0 {
			server["env"] = spec.Env
		}
		return server
	}
	server["url"] = spec.URL
	if len(spec.Headers) > 0 {
		server["headers"] = spec.Headers
	}
	return server
}

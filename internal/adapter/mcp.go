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

package domain

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/goccy/go-yaml"
)

// MCP transport values.
const (
	MCPTransportStdio = "stdio"
	MCPTransportHTTP  = "http"
	MCPTransportSSE   = "sse"
)

// MCP scope values.
const (
	MCPScopeUser    = "user"
	MCPScopeProject = "project"
)

// MCPSpec is the canonical MCP server definition (mcps/<name>/mcp.yaml).
type MCPSpec struct {
	// Transport is stdio (local command), http or sse (remote URL).
	Transport string            `yaml:"transport,omitempty"`
	Command   string            `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	URL       string            `yaml:"url,omitempty"`
	Headers   map[string]string `yaml:"headers,omitempty"`
	// Scope is user (agent's global config) or project (per-repo config).
	Scope string `yaml:"scope,omitempty"`
}

var (
	secretKeyRe   = regexp.MustCompile(`(?i)(key|token|secret|password|authorization)`)
	placeholderRe = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`)
)

// ParseMCPSpec decodes an mcp.yaml document, applies defaults and validates
// it, including the no-literal-secrets rule (the store must stay shareable).
func ParseMCPSpec(raw []byte) (MCPSpec, error) {
	var m MCPSpec
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("parse mcp.yaml: %w", err)
	}
	if m.Transport == "" {
		m.Transport = MCPTransportStdio
	}
	if m.Scope == "" {
		m.Scope = MCPScopeUser
	}
	if err := m.validate(); err != nil {
		return m, err
	}
	return m, nil
}

func (m MCPSpec) validate() error {
	switch m.Transport {
	case MCPTransportStdio:
		if m.Command == "" {
			return fmt.Errorf("mcp.yaml: stdio transport requires command")
		}
		if m.URL != "" || len(m.Headers) > 0 {
			return fmt.Errorf("mcp.yaml: stdio transport does not take url or headers")
		}
	case MCPTransportHTTP, MCPTransportSSE:
		if m.URL == "" {
			return fmt.Errorf("mcp.yaml: %s transport requires url", m.Transport)
		}
		if m.Command != "" || len(m.Args) > 0 {
			return fmt.Errorf("mcp.yaml: %s transport does not take command or args", m.Transport)
		}
	default:
		return fmt.Errorf("mcp.yaml: unknown transport %q (stdio, http or sse)", m.Transport)
	}
	switch m.Scope {
	case MCPScopeUser, MCPScopeProject:
	default:
		return fmt.Errorf("mcp.yaml: unknown scope %q (user or project)", m.Scope)
	}
	for k, v := range m.Env {
		if err := checkSecretValue("env", k, v); err != nil {
			return err
		}
	}
	for k, v := range m.Headers {
		if err := checkSecretValue("header", k, v); err != nil {
			return err
		}
	}
	return nil
}

// checkSecretValue enforces the placeholder rule: a secret-looking key may
// only carry ${VAR} references (plus a plain-word prefix such as "Bearer"),
// never a literal value — secrets must resolve from the environment at agent
// runtime, not live in the store.
func checkSecretValue(kind, key, value string) error {
	if !secretKeyRe.MatchString(key) || value == "" {
		return nil
	}
	if !placeholderRe.MatchString(value) {
		return literalSecretError(kind, key)
	}
	rest := placeholderRe.ReplaceAllString(value, "")
	for _, r := range rest {
		if !isPlainWordRune(r) {
			return literalSecretError(kind, key)
		}
	}
	return nil
}

func isPlainWordRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == ' ', r == '\t':
		return true
	}
	return false
}

func literalSecretError(kind, key string) error {
	return fmt.Errorf(
		"mcp.yaml: %s %q looks like a literal secret; use a ${VAR} placeholder (e.g. %s: ${%s}) so the value resolves from the environment and never enters the store",
		kind, key, key, strings.ToUpper(strings.ReplaceAll(key, "-", "_")))
}

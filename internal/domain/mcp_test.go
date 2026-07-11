package domain

import (
	"strings"
	"testing"
)

func TestParseMCPSpec_ValidStdio(t *testing.T) {
	spec, err := ParseMCPSpec([]byte(`
command: npx
args: ["-y", "@modelcontextprotocol/server-github"]
env:
  GITHUB_TOKEN: ${GITHUB_TOKEN}
  DEBUG: "1"
`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Transport != MCPTransportStdio {
		t.Errorf("transport = %q, want default stdio", spec.Transport)
	}
	if spec.Scope != MCPScopeUser {
		t.Errorf("scope = %q, want default user", spec.Scope)
	}
	if spec.Command != "npx" || len(spec.Args) != 2 {
		t.Errorf("command/args not parsed: %+v", spec)
	}
	if spec.Env["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" {
		t.Errorf("placeholder not preserved verbatim: %q", spec.Env["GITHUB_TOKEN"])
	}
}

func TestParseMCPSpec_ValidRemote(t *testing.T) {
	for _, transport := range []string{"http", "sse"} {
		spec, err := ParseMCPSpec([]byte(`
transport: ` + transport + `
url: https://mcp.example.com/v1
headers:
  Authorization: Bearer ${EXAMPLE_KEY}
scope: project
`))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", transport, err)
		}
		if spec.Scope != MCPScopeProject {
			t.Errorf("%s: scope = %q, want project", transport, spec.Scope)
		}
		if spec.Headers["Authorization"] != "Bearer ${EXAMPLE_KEY}" {
			t.Errorf("%s: header not preserved: %q", transport, spec.Headers["Authorization"])
		}
	}
}

func TestParseMCPSpec_RejectsInvalidShapes(t *testing.T) {
	cases := map[string]string{
		"stdio without command": `env: {DEBUG: "1"}`,
		"stdio with url":        "command: npx\nurl: https://x.example",
		"http without url":      `transport: http`,
		"http with command":     "transport: http\nurl: https://x.example\ncommand: npx",
		"unknown transport":     "transport: websocket\nurl: https://x.example",
		"unknown scope":         "command: npx\nscope: global",
	}
	for name, doc := range cases {
		if _, err := ParseMCPSpec([]byte(doc)); err == nil {
			t.Errorf("%s: expected error, got none", name)
		}
	}
}

func TestParseMCPSpec_SecretGate(t *testing.T) {
	rejected := map[string]string{
		"literal env token":      "command: npx\nenv: {GITHUB_TOKEN: ghp_abc123}",
		"literal api key":        "command: npx\nenv: {MY_API_KEY: sk-live-abc}",
		"literal password":       "command: npx\nenv: {DB_PASSWORD: hunter2}",
		"literal auth header":    "transport: http\nurl: https://x.example\nheaders: {Authorization: Bearer ghp_abc123}",
		"mixed literal + var":    "command: npx\nenv: {API_KEY: 'abc-${SUFFIX}'}",
		"secret without any var": "transport: http\nurl: https://x.example\nheaders: {X-Api-Key: '12345'}",
	}
	for name, doc := range rejected {
		_, err := ParseMCPSpec([]byte(doc))
		if err == nil {
			t.Errorf("%s: expected rejection, got none", name)
			continue
		}
		if !strings.Contains(err.Error(), "${") {
			t.Errorf("%s: error should point at placeholder usage, got: %v", name, err)
		}
	}

	accepted := map[string]string{
		"placeholder env":         "command: npx\nenv: {GITHUB_TOKEN: '${GITHUB_TOKEN}'}",
		"bearer placeholder":      "transport: http\nurl: https://x.example\nheaders: {Authorization: 'Bearer ${K}'}",
		"empty secret value":      "command: npx\nenv: {API_KEY: ''}",
		"non-secret literal":      "command: npx\nenv: {DEBUG: '1', PORT: '8080'}",
		"non-secret header":       "transport: http\nurl: https://x.example\nheaders: {Accept: application/json}",
		"secret-free plain value": "command: npx\nenv: {TOKENIZER: '${MODE}'}",
	}
	for name, doc := range accepted {
		if _, err := ParseMCPSpec([]byte(doc)); err != nil {
			t.Errorf("%s: expected acceptance, got: %v", name, err)
		}
	}
}

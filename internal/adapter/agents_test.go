package adapter_test

// Cross-adapter contract tests: every adapter must map the shared canonical
// kinds to its native layout, and reject what it declares unsupported.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/antigravity"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/adapter/codex"
	"github.com/AxeForging/loadout/internal/adapter/cursor"
	"github.com/AxeForging/loadout/internal/adapter/geminicli"
	"github.com/AxeForging/loadout/internal/domain"
)

func skillContent() domain.Content {
	return domain.Content{Files: map[string][]byte{"SKILL.md": []byte("---\nname: demo\n---\n")}}
}

func commandContent() domain.Content {
	return domain.Content{Files: map[string][]byte{"plan.md": []byte("---\ndescription: Plan it.\n---\n\nGo plan.\n")}}
}

func instructionContent() domain.Content {
	return domain.Content{Files: map[string][]byte{"style.md": []byte("Use tabs.\n")}}
}

func TestCodex_RendersSkillInstructionAndCommandAsSkill(t *testing.T) {
	a := codex.New()
	skill, err := a.Render(domain.Artifact{ID: "demo", Kind: domain.KindSkill}, skillContent())
	if err != nil || skill[0].RelPath != "skills/demo/SKILL.md" {
		t.Errorf("skill target = %v, err %v", skill, err)
	}

	instr, err := a.Render(domain.Artifact{ID: "style", Kind: domain.KindInstruction}, instructionContent())
	if err != nil || instr[0].RelPath != "AGENTS.md" || instr[0].Merge != adapter.MergeBlock {
		t.Errorf("instruction target = %v, err %v", instr, err)
	}

	cmdT, err := a.Render(domain.Artifact{ID: "plan", Kind: domain.KindCommand}, commandContent())
	if err != nil || cmdT[0].RelPath != "skills/plan/SKILL.md" {
		t.Fatalf("command target = %v, err %v", cmdT, err)
	}
	if !strings.Contains(string(cmdT[0].Content), "description: Plan it.") {
		t.Errorf("command→skill lost description:\n%s", cmdT[0].Content)
	}

	if a.Supports(domain.KindHook) || a.Supports(domain.KindSettings) {
		t.Error("codex must not support hooks or settings yet")
	}
}

func TestGeminiCLI_RendersAllSupportedKinds(t *testing.T) {
	a := geminicli.New()
	instr, err := a.Render(domain.Artifact{ID: "style", Kind: domain.KindInstruction}, instructionContent())
	if err != nil || instr[0].RelPath != "GEMINI.md" || instr[0].Merge != adapter.MergeBlock {
		t.Errorf("instruction target = %v, err %v", instr, err)
	}

	settings, err := a.Render(domain.Artifact{ID: "prefs", Kind: domain.KindSettings},
		domain.Content{Files: map[string][]byte{"gemini-cli.yaml": []byte("theme: dark\n")}})
	if err != nil || settings[0].RelPath != "settings.json" || settings[0].Merge != adapter.MergeJSON {
		t.Errorf("settings target = %v, err %v", settings, err)
	}

	// A fragment addressed to another agent renders nothing.
	other, err := a.Render(domain.Artifact{ID: "prefs2", Kind: domain.KindSettings},
		domain.Content{Files: map[string][]byte{"claude-code.yaml": []byte("a: 1\n")}})
	if err != nil || other != nil {
		t.Errorf("foreign settings fragment should render nil, got %v, err %v", other, err)
	}
}

func TestAntigravity_SkillsUnderAntigravityCLIAndSharedGeminiMD(t *testing.T) {
	a := antigravity.New()
	skill, err := a.Render(domain.Artifact{ID: "demo", Kind: domain.KindSkill}, skillContent())
	if err != nil || skill[0].RelPath != "antigravity-cli/skills/demo/SKILL.md" {
		t.Errorf("skill target = %v, err %v", skill, err)
	}
	instr, err := a.Render(domain.Artifact{ID: "style", Kind: domain.KindInstruction}, instructionContent())
	if err != nil || instr[0].RelPath != "GEMINI.md" {
		t.Errorf("instruction target = %v, err %v", instr, err)
	}
	if a.Supports(domain.KindCommand) {
		t.Error("antigravity commands (workflows) are project-scoped — not supported yet")
	}
}

func TestAntigravity_DetectRequiresAntigravityCLIDir(t *testing.T) {
	home := t.TempDir() // a ~/.gemini without antigravity-cli
	a := antigravity.NewWithHome(home)
	det, err := a.Detect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if det.Installed {
		t.Error("antigravity must not detect on a plain ~/.gemini")
	}

	if err := os.MkdirAll(filepath.Join(home, "antigravity-cli"), 0o755); err != nil {
		t.Fatal(err)
	}
	det, _ = a.Detect(context.Background())
	if !det.Installed {
		t.Error("antigravity should detect once antigravity-cli exists")
	}
}

func TestCursor_RendersSkillsAndCommandsOnly(t *testing.T) {
	a := cursor.New()
	skill, err := a.Render(domain.Artifact{ID: "demo", Kind: domain.KindSkill}, skillContent())
	if err != nil || skill[0].RelPath != "skills/demo/SKILL.md" {
		t.Errorf("skill target = %v, err %v", skill, err)
	}
	cmdT, err := a.Render(domain.Artifact{ID: "plan", Kind: domain.KindCommand}, commandContent())
	if err != nil || cmdT[0].RelPath != "skills/plan/SKILL.md" {
		t.Errorf("command target = %v, err %v", cmdT, err)
	}
	if a.Supports(domain.KindInstruction) {
		t.Error("cursor global instructions (.mdc rules) are project-scoped — not supported yet")
	}
}

// --- MCP rendering (spec 006) ---

func mcpStdioContent() domain.Content {
	return domain.Content{Files: map[string][]byte{"mcp.yaml": []byte(
		"command: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-github\"]\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\n")}}
}

func mcpRemoteContent(transport, scope string) domain.Content {
	return domain.Content{Files: map[string][]byte{"mcp.yaml": []byte(
		"transport: " + transport + "\nurl: https://mcp.example.com/v1\nheaders:\n  Authorization: Bearer ${EXAMPLE_KEY}\nscope: " + scope + "\n")}}
}

func TestClaudeCode_RendersMCPUserScopeIntoUserHomeClaudeJSON(t *testing.T) {
	a := claudecode.New()
	got, err := a.Render(domain.Artifact{ID: "github", Kind: domain.KindMCP}, mcpStdioContent())
	if err != nil || len(got) != 1 {
		t.Fatalf("render = %v, err %v", got, err)
	}
	tf := got[0]
	if tf.RelPath != ".claude.json" || tf.Root != adapter.RootUserHome || tf.Merge != adapter.MergeJSON {
		t.Errorf("target = %+v, want .claude.json under user home via json-merge", tf)
	}
	body := string(tf.Content)
	for _, want := range []string{`"mcpServers"`, `"github"`, `"command":"npx"`, `"${GITHUB_TOKEN}"`, `"type":"stdio"`} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment missing %s:\n%s", want, body)
		}
	}
}

func TestClaudeCode_RendersMCPProjectScopeIntoDotMCPJSON(t *testing.T) {
	a := claudecode.New()
	got, err := a.Render(domain.Artifact{ID: "search", Kind: domain.KindMCP}, mcpRemoteContent("http", "project"))
	if err != nil || len(got) != 1 {
		t.Fatalf("render = %v, err %v", got, err)
	}
	tf := got[0]
	if tf.RelPath != ".mcp.json" || tf.Root != adapter.RootProject {
		t.Errorf("target = %+v, want .mcp.json under project root", tf)
	}
	body := string(tf.Content)
	for _, want := range []string{`"type":"http"`, `"url":"https://mcp.example.com/v1"`, `"Bearer ${EXAMPLE_KEY}"`} {
		if !strings.Contains(body, want) {
			t.Errorf("fragment missing %s:\n%s", want, body)
		}
	}
}

func TestGeminiCLI_RendersMCPWithTransportSpecificURLKeys(t *testing.T) {
	a := geminicli.New()
	stdio, err := a.Render(domain.Artifact{ID: "github", Kind: domain.KindMCP}, mcpStdioContent())
	if err != nil || stdio[0].RelPath != "settings.json" || stdio[0].Root != adapter.RootAgentHome {
		t.Fatalf("stdio target = %v, err %v", stdio, err)
	}
	httpT, err := a.Render(domain.Artifact{ID: "search", Kind: domain.KindMCP}, mcpRemoteContent("http", "user"))
	if err != nil || !strings.Contains(string(httpT[0].Content), `"httpUrl"`) {
		t.Errorf("gemini http transport must use httpUrl: %s (err %v)", httpT[0].Content, err)
	}
	sseT, err := a.Render(domain.Artifact{ID: "events", Kind: domain.KindMCP}, mcpRemoteContent("sse", "project"))
	if err != nil || !strings.Contains(string(sseT[0].Content), `"url"`) {
		t.Errorf("gemini sse transport must use url: %s (err %v)", sseT[0].Content, err)
	}
	if sseT[0].RelPath != ".gemini/settings.json" || sseT[0].Root != adapter.RootProject {
		t.Errorf("project scope target = %+v", sseT[0])
	}
}

func TestCursor_RendersMCPBothScopes(t *testing.T) {
	a := cursor.New()
	user, err := a.Render(domain.Artifact{ID: "github", Kind: domain.KindMCP}, mcpStdioContent())
	if err != nil || user[0].RelPath != "mcp.json" || user[0].Root != adapter.RootAgentHome {
		t.Fatalf("user target = %v, err %v", user, err)
	}
	proj, err := a.Render(domain.Artifact{ID: "search", Kind: domain.KindMCP}, mcpRemoteContent("sse", "project"))
	if err != nil || proj[0].RelPath != ".cursor/mcp.json" || proj[0].Root != adapter.RootProject {
		t.Fatalf("project target = %v, err %v", proj, err)
	}
}

func TestCodexAndAntigravity_DoNotSupportMCPYet(t *testing.T) {
	if codex.New().Supports(domain.KindMCP) {
		t.Error("codex config.toml needs a comment-preserving merge before MCP support — see ROADMAP")
	}
	if antigravity.New().Supports(domain.KindMCP) {
		t.Error("antigravity MCP config location is unverified — see ROADMAP")
	}
}

func TestMCPRender_RejectsLiteralSecretAtRenderTime(t *testing.T) {
	bad := domain.Content{Files: map[string][]byte{"mcp.yaml": []byte("command: npx\nenv:\n  GITHUB_TOKEN: ghp_livetoken\n")}}
	if _, err := claudecode.New().Render(domain.Artifact{ID: "github", Kind: domain.KindMCP}, bad); err == nil {
		t.Fatal("render must refuse literal secrets so sync can never install them")
	}
}

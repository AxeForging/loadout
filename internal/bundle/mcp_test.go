package bundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/store"
)

func mcpStore(t *testing.T) (*store.Store, domain.Artifact) {
	t.Helper()
	s := testStore(t, "mcp-kit")
	a, err := s.New(domain.KindMCP, "github")
	if err != nil {
		t.Fatal(err)
	}
	spec := "command: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-github\"]\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN}\nscope: user\n"
	if err := os.WriteFile(filepath.Join(s.Root, a.Path, "mcp.yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	return s, a
}

func TestBundle_MCPRoundTripsThroughDirTarGzAndFlatYAML(t *testing.T) {
	src, art := mcpStore(t)
	want, err := os.ReadFile(filepath.Join(src.Root, art.Path, "mcp.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	importInto := func(t *testing.T, bundlePath string) *store.Store {
		t.Helper()
		fresh := testStore(t, "fresh-"+filepath.Base(bundlePath))
		plan, err := PlanImport(fresh, bundlePath)
		if err != nil {
			t.Fatalf("PlanImport(%s) error = %v", bundlePath, err)
		}
		defer plan.Close()
		if len(plan.New) != 1 || plan.New[0].Kind != domain.KindMCP {
			t.Fatalf("plan.New = %v, want one mcp artifact", plan.New)
		}
		if _, err := Apply(fresh, plan, false); err != nil {
			t.Fatalf("Apply() error = %v", err)
		}
		return fresh
	}

	assertIdentical := func(t *testing.T, s *store.Store) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(s.Root, "mcps", "github", "mcp.yaml"))
		if err != nil {
			t.Fatalf("imported mcp.yaml missing: %v", err)
		}
		if string(got) != string(want) {
			t.Errorf("mcp.yaml not byte-identical after round trip:\ngot:\n%s\nwant:\n%s", got, want)
		}
	}

	t.Run("directory", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "bundle")
		if _, err := Export(src, dest, "test"); err != nil {
			t.Fatal(err)
		}
		assertIdentical(t, importInto(t, dest))
	})

	t.Run("tar.gz", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "kit.loadout.tar.gz")
		if _, err := ExportArchive(src, dest, "test"); err != nil {
			t.Fatal(err)
		}
		assertIdentical(t, importInto(t, dest))
	})

	t.Run("flat yaml", func(t *testing.T) {
		raw, err := ExportFlat(src, "test", Selection{ArtifactIDs: []string{"github"}})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "kind: mcp") {
			t.Errorf("flat document missing mcp artifact:\n%s", raw)
		}
		fresh := testStore(t, "flat-fresh")
		plan, err := ImportFlat(fresh, raw)
		if err != nil {
			t.Fatalf("ImportFlat() error = %v", err)
		}
		defer plan.Close()
		if _, err := Apply(fresh, plan, false); err != nil {
			t.Fatal(err)
		}
		assertIdentical(t, fresh)
	})
}

func TestImportFlat_RejectsMCPWithLiteralSecret(t *testing.T) {
	src, art := mcpStore(t)
	raw, err := ExportFlat(src, "test", Selection{ArtifactIDs: []string{art.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a tampered (or hand-authored) bundle carrying a real token.
	tampered := strings.Replace(string(raw), "${GITHUB_TOKEN}", "ghp_realtoken123", 1)
	if tampered == string(raw) {
		t.Fatal("test setup: placeholder not found in flat export")
	}

	fresh := testStore(t, "victim")
	if _, err := ImportFlat(fresh, []byte(tampered)); err == nil {
		t.Fatal("import must reject a bundle whose mcp spec carries a literal secret")
	} else if !strings.Contains(err.Error(), "${") {
		t.Errorf("rejection should explain the placeholder rule, got: %v", err)
	}
}

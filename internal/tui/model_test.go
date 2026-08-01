package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AxeForging/loadout/internal/adapter"
	"github.com/AxeForging/loadout/internal/adapter/claudecode"
	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel wires a real engine over a temp store + fake claude home and
// runs the model's snapshot command synchronously.
func newTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Init(filepath.Join(dir, "store"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.New(domain.KindSkill, "demo"); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "claude-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := adapter.NewRegistry()
	reg.Register(claudecode.NewWithHome(home))
	eng := &engine.Engine{Store: s, Registry: reg, StatePath: filepath.Join(dir, "state.yaml")}

	m := NewModel(Deps{Engine: eng})
	m.width, m.height = 100, 30
	msg := m.loadSnapshot()()
	updated, _ := m.Update(msg)
	return updated.(*Model), home
}

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestModel_SnapshotRendersPendingChangesOnOverview(t *testing.T) {
	m, _ := newTestModel(t)
	view := m.View()
	if !strings.Contains(view, "pending change(s)") {
		t.Errorf("overview should show pending changes:\n%s", view)
	}
	if !strings.Contains(view, "claude-code") {
		t.Errorf("overview should list the agent:\n%s", view)
	}
}

func TestModel_TabSwitchingReachesEveryTab(t *testing.T) {
	m, _ := newTestModel(t)
	for i := 0; i < int(tabCount); i++ {
		if view := m.View(); view == "" {
			t.Fatalf("tab %d rendered empty", m.tab)
		}
		updated, _ := m.Update(key("tab"))
		m = updated.(*Model)
	}
	if m.tab != tabOverview {
		t.Errorf("tab cycling should wrap, ended on %v", m.tab)
	}
}

func TestModel_SyncDeclineWritesNothing(t *testing.T) {
	m, home := newTestModel(t)
	// Go to Agents tab and open the sync modal.
	updated, _ := m.Update(key("3"))
	m = updated.(*Model)
	updated, _ = m.Update(key("s"))
	m = updated.(*Model)
	if m.mode != modeConfirm {
		t.Fatal("s should open the confirm modal")
	}
	view := m.View()
	if !strings.Contains(view, "Apply 1 change(s)?") {
		t.Errorf("modal should show the plan:\n%s", view)
	}

	updated, cmd := m.Update(key("n"))
	m = updated.(*Model)
	if cmd != nil {
		t.Fatal("declining must not dispatch any command")
	}
	if m.mode != modeNormal {
		t.Error("declining should close the modal")
	}
	if _, err := os.Stat(filepath.Join(home, "skills")); !os.IsNotExist(err) {
		t.Fatal("agent home written despite declined modal")
	}
}

func TestModel_SyncConfirmAppliesAndRefreshes(t *testing.T) {
	m, home := newTestModel(t)
	updated, _ := m.Update(key("3"))
	m = updated.(*Model)
	updated, _ = m.Update(key("s"))
	m = updated.(*Model)

	updated, cmd := m.Update(key("y"))
	m = updated.(*Model)
	if cmd == nil {
		t.Fatal("confirming should dispatch the apply command")
	}
	msg := cmd() // run apply synchronously
	applied, ok := msg.(appliedMsg)
	if !ok || applied.err != nil || applied.applied != 1 {
		t.Fatalf("apply result = %+v", msg)
	}
	if _, err := os.Stat(filepath.Join(home, "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("skill not installed after confirmed sync: %v", err)
	}

	// Feed the result back: model refreshes and the log records it.
	updated, refresh := m.Update(msg)
	m = updated.(*Model)
	if refresh == nil {
		t.Fatal("appliedMsg should trigger a snapshot refresh")
	}
	updated, _ = m.Update(refresh())
	m = updated.(*Model)
	if len(m.plan.Changes()) != 0 {
		t.Errorf("after apply the plan should be clean, got %v", m.plan.Changes())
	}
	logView := strings.Join(m.logLines, "\n")
	if !strings.Contains(logView, "applied 1 change(s)") {
		t.Errorf("log should record the apply: %v", m.logLines)
	}
}

func TestModel_DiffViewShowsSelectedRow(t *testing.T) {
	m, _ := newTestModel(t)
	updated, _ := m.Update(key("3"))
	m = updated.(*Model)
	updated, _ = m.Update(key("d"))
	m = updated.(*Model)
	if m.mode != modeDiff {
		t.Fatal("d should open the diff view")
	}
	view := m.View()
	if !strings.Contains(view, "SKILL.md") {
		t.Errorf("diff should reference the target file:\n%s", view)
	}
	updated, _ = m.Update(key("esc"))
	m = updated.(*Model)
	if m.mode != modeNormal {
		t.Error("esc should close the diff view")
	}
}

func TestModel_OverviewShowsCachedUpdateHint(t *testing.T) {
	m, _ := newTestModel(t)

	// No cached count → no hint line.
	if strings.Contains(m.View(), "source update(s) available") {
		t.Error("hint rendered without a cached update count")
	}

	// Cached count present (as loadSnapshot would deliver it) → hint line.
	updated, _ := m.Update(snapshotMsg{rows: m.rows, plan: m.plan, agents: m.agents, sources: m.sources, updateHint: 2})
	m = updated.(*Model)
	view := m.View()
	if !strings.Contains(view, "2 source update(s) available — run: loadout update") {
		t.Errorf("overview should show the cached update hint:\n%s", view)
	}
}

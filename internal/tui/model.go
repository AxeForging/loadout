package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/AxeForging/loadout/internal/engine"
	"github.com/AxeForging/loadout/internal/source"
	tea "github.com/charmbracelet/bubbletea"
)

type tab int

const (
	tabOverview tab = iota
	tabStore
	tabAgents
	tabSources
	tabLog
	tabCount
)

var tabNames = []string{"Overview", "Store", "Agents", "Sources", "Log"}

// mode is the interaction state layered over the tabs.
type mode int

const (
	modeNormal mode = iota
	modeDiff
	modeConfirm
)

type agentInfo struct {
	ID        domain.AgentID
	Name      string
	Installed bool
	Home      string
}

// Messages produced by commands.
type snapshotMsg struct {
	rows    []engine.StatusRow
	plan    *engine.Plan
	agents  []agentInfo
	sources []domain.SourceRef
	err     error
}

type checkMsg struct {
	results []source.CheckResult
	err     error
}

type appliedMsg struct {
	applied int
	err     error
}

type Model struct {
	deps Deps

	tab    tab
	mode   mode
	width  int
	height int

	rows    []engine.StatusRow
	plan    *engine.Plan
	agents  []agentInfo
	sources []domain.SourceRef
	checks  map[string]source.CheckResult

	storeCursor  int
	agentCursor  int
	sourceCursor int

	diffLines  []string
	diffOffset int

	logLines []string
	loading  bool
	checking bool
	err      error
}

func NewModel(deps Deps) *Model {
	return &Model{deps: deps, checks: map[string]source.CheckResult{}, loading: true}
}

func (m *Model) Init() tea.Cmd { return m.loadSnapshot() }

// loadSnapshot rebuilds the full status view (plan, rows, agents, sources).
func (m *Model) loadSnapshot() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx := context.Background()
		rows, plan, err := deps.Engine.Status(ctx, engine.Filter{})
		if err != nil {
			return snapshotMsg{err: err}
		}
		var agents []agentInfo
		for _, ad := range deps.Engine.Registry.All() {
			det, derr := ad.Detect(ctx)
			info := agentInfo{ID: ad.ID(), Name: ad.DisplayName(), Home: det.Home, Installed: det.Installed && derr == nil}
			agents = append(agents, info)
		}
		var sources []domain.SourceRef
		if deps.Sources != nil {
			sources, _ = deps.Sources.Lock()
		}
		return snapshotMsg{rows: rows, plan: plan, agents: agents, sources: sources}
	}
}

func (m *Model) runCheck() tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		results, err := deps.Sources.Check(ctx)
		return checkMsg{results: results, err: err}
	}
}

// applyPlan is only ever dispatched from the confirm modal's "yes".
func (m *Model) applyPlan(plan *engine.Plan) tea.Cmd {
	deps := m.deps
	return func() tea.Msg {
		n, err := deps.Engine.Apply(context.Background(), plan, func(*engine.Plan) bool { return true })
		return appliedMsg{applied: n, err: err}
	}
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case snapshotMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.rows, m.plan, m.agents, m.sources = msg.rows, msg.plan, msg.agents, msg.sources
			m.clampCursors()
		}
		return m, nil

	case checkMsg:
		m.checking = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		for _, r := range msg.results {
			m.checks[r.Name] = r
		}
		m.log("checked %d source(s)", len(msg.results))
		return m, nil

	case appliedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.log("sync failed: %v", msg.err)
		} else {
			m.log("applied %d change(s)", msg.applied)
		}
		m.loading = true
		return m, m.loadSnapshot()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.mode {
	case modeDiff:
		switch key {
		case "q", "esc", "d":
			m.mode = modeNormal
		case "j", "down":
			if m.diffOffset < len(m.diffLines)-1 {
				m.diffOffset++
			}
		case "k", "up":
			if m.diffOffset > 0 {
				m.diffOffset--
			}
		}
		return m, nil

	case modeConfirm:
		switch key {
		case "y", "Y":
			m.mode = modeNormal
			plan := m.plan
			m.log("confirmed — applying %d change(s)", len(plan.Changes()))
			return m, m.applyPlan(plan)
		case "n", "N", "esc", "q":
			m.mode = modeNormal
			m.log("sync declined — nothing written")
		}
		return m, nil
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab", "right", "l":
		m.tab = (m.tab + 1) % tabCount
	case "shift+tab", "left", "h":
		m.tab = (m.tab + tabCount - 1) % tabCount
	case "1", "2", "3", "4", "5":
		m.tab = tab(key[0] - '1')
	case "r":
		m.loading = true
		return m, m.loadSnapshot()
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "d":
		if m.tab == tabAgents {
			m.openDiff()
		}
	case "s":
		if (m.tab == tabAgents || m.tab == tabOverview) && m.plan != nil && len(m.plan.Changes()) > 0 {
			m.mode = modeConfirm
		}
	case "c":
		if m.tab == tabSources && m.deps.Sources != nil && !m.checking {
			m.checking = true
			return m, m.runCheck()
		}
	}
	return m, nil
}

func (m *Model) moveCursor(delta int) {
	switch m.tab {
	case tabStore:
		m.storeCursor = clamp(m.storeCursor+delta, 0, len(m.deps.Engine.Store.Manifest.Artifacts)-1)
	case tabAgents:
		m.agentCursor = clamp(m.agentCursor+delta, 0, len(m.rows)-1)
	case tabSources:
		m.sourceCursor = clamp(m.sourceCursor+delta, 0, len(m.sources)-1)
	}
}

func (m *Model) clampCursors() {
	m.storeCursor = clamp(m.storeCursor, 0, len(m.deps.Engine.Store.Manifest.Artifacts)-1)
	m.agentCursor = clamp(m.agentCursor, 0, len(m.rows)-1)
	m.sourceCursor = clamp(m.sourceCursor, 0, len(m.sources)-1)
}

// openDiff loads unified diffs for the selected artifact × agent row.
func (m *Model) openDiff() {
	if m.plan == nil || len(m.rows) == 0 || m.agentCursor >= len(m.rows) {
		return
	}
	row := m.rows[m.agentCursor]
	var lines []string
	for _, it := range m.plan.Items {
		if it.Artifact.ID != row.Artifact || it.Agent != row.Agent || it.Op == engine.OpNoop {
			continue
		}
		text, err := engine.UnifiedDiff(it)
		if err != nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("── %s", it.RelPath))
		lines = append(lines, strings.Split(strings.TrimRight(text, "\n"), "\n")...)
		lines = append(lines, "")
	}
	if len(lines) == 0 {
		lines = []string{fmt.Sprintf("%s on %s — nothing to change (%s)", row.Artifact, row.Agent, row.State)}
	}
	m.diffLines = lines
	m.diffOffset = 0
	m.mode = modeDiff
}

func (m *Model) log(format string, args ...any) {
	line := fmt.Sprintf("%s  %s", time.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
	m.logLines = append(m.logLines, line)
	if len(m.logLines) > 200 {
		m.logLines = m.logLines[len(m.logLines)-200:]
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

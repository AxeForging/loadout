package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/AxeForging/loadout/internal/domain"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleTabActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Padding(0, 1)
	styleTab       = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Padding(0, 1)
	styleTitle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styleDim       = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleCursor    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	styleErr       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleModal     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("212")).Padding(1, 2)

	stateStyles = map[domain.SyncState]lipgloss.Style{
		domain.StateInSync:       lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		domain.StateMissing:      lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		domain.StateDriftedLocal: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		domain.StateDriftedStore: lipgloss.NewStyle().Foreground(lipgloss.Color("39")),
		domain.StateConflict:     lipgloss.NewStyle().Foreground(lipgloss.Color("203")),
		domain.StateUnsupported:  styleDim,
	}
)

func (m *Model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	var b strings.Builder
	b.WriteString(m.renderTabs())
	b.WriteString("\n\n")

	switch m.mode {
	case modeDiff:
		b.WriteString(m.renderDiff())
	case modeConfirm:
		b.WriteString(m.renderConfirm())
	default:
		switch m.tab {
		case tabOverview:
			b.WriteString(m.renderOverview())
		case tabStore:
			b.WriteString(m.renderStore())
		case tabAgents:
			b.WriteString(m.renderAgents())
		case tabSources:
			b.WriteString(m.renderSources())
		case tabLog:
			b.WriteString(m.renderLog())
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m *Model) renderTabs() string {
	parts := make([]string, 0, tabCount)
	for i, name := range tabNames {
		label := fmt.Sprintf("%d %s", i+1, name)
		if tab(i) == m.tab {
			parts = append(parts, styleTabActive.Render(label))
		} else {
			parts = append(parts, styleTab.Render(label))
		}
	}
	return " " + strings.Join(parts, "·")
}

func (m *Model) renderFooter() string {
	if m.err != nil {
		return " " + styleErr.Render("error: "+m.err.Error())
	}
	help := "q quit · tab/1-5 switch · r refresh · j/k move"
	switch m.mode {
	case modeDiff:
		help = "j/k scroll · d/esc close"
	case modeConfirm:
		help = "y apply · n cancel"
	default:
		switch m.tab {
		case tabAgents:
			help += " · d diff · s sync"
		case tabOverview:
			help += " · s sync"
		case tabSources:
			help += " · c check remotes"
		}
	}
	if m.loading {
		help += " · loading…"
	}
	if m.checking {
		help += " · checking…"
	}
	return " " + styleDim.Render(help)
}

func (m *Model) renderOverview() string {
	var b strings.Builder
	b.WriteString(" " + styleTitle.Render(fmt.Sprintf("Store %q — %d artifact(s)", m.deps.Engine.Store.Manifest.Name, len(m.deps.Engine.Store.Manifest.Artifacts))) + "\n\n")

	counts := map[domain.AgentID]map[domain.SyncState]int{}
	for _, r := range m.rows {
		if counts[r.Agent] == nil {
			counts[r.Agent] = map[domain.SyncState]int{}
		}
		counts[r.Agent][r.State]++
	}
	for _, a := range m.agents {
		status := styleDim.Render("not installed")
		if a.Installed {
			c := counts[a.ID]
			parts := []string{}
			for _, s := range []domain.SyncState{domain.StateInSync, domain.StateMissing, domain.StateDriftedLocal, domain.StateDriftedStore, domain.StateConflict} {
				if c[s] > 0 {
					parts = append(parts, stateStyles[s].Render(fmt.Sprintf("%d %s", c[s], s)))
				}
			}
			if len(parts) == 0 {
				parts = append(parts, styleDim.Render("no artifacts targeted"))
			}
			status = strings.Join(parts, styleDim.Render(" · "))
		}
		fmt.Fprintf(&b, "  %-14s %s\n", a.ID, status)
	}

	if m.plan != nil {
		changes := len(m.plan.Changes())
		b.WriteString("\n")
		if changes > 0 {
			fmt.Fprintf(&b, "  %s\n", styleTitle.Render(fmt.Sprintf("%d pending change(s) — press s to sync (asks first)", changes)))
		} else {
			b.WriteString("  " + styleDim.Render("everything in sync") + "\n")
		}
	}
	return b.String()
}

func (m *Model) renderStore() string {
	arts := m.deps.Engine.Store.Manifest.Artifacts
	if len(arts) == 0 {
		return " " + styleDim.Render("store is empty — 'loadout new' or 'loadout wizard' to create artifacts")
	}
	var list strings.Builder
	for i, a := range arts {
		cursor := "  "
		line := fmt.Sprintf("%-24s %-12s %s", a.ID, a.Kind, styleDim.Render(a.Origin))
		if i == m.storeCursor {
			cursor = styleCursor.Render("> ")
			line = styleCursor.Render(fmt.Sprintf("%-24s %-12s ", a.ID, a.Kind)) + styleDim.Render(a.Origin)
		}
		list.WriteString(" " + cursor + line + "\n")
	}

	preview := ""
	if m.storeCursor < len(arts) {
		preview = m.previewArtifact(arts[m.storeCursor])
	}
	left := lipgloss.NewStyle().Width(m.width/2 - 2).Render(list.String())
	right := styleDim.Render(preview)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m *Model) previewArtifact(a domain.Artifact) string {
	content, err := m.deps.Engine.Store.Content(a)
	if err != nil {
		return "preview unavailable: " + err.Error()
	}
	paths := make([]string, 0, len(content.Files))
	for p := range content.Files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return "(empty)"
	}
	lines := strings.Split(string(content.Files[paths[0]]), "\n")
	maxLines := m.height - 10
	if maxLines < 5 {
		maxLines = 5
	}
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "…")
	}
	return paths[0] + "\n\n" + strings.Join(lines, "\n")
}

func (m *Model) renderAgents() string {
	if len(m.rows) == 0 {
		return " " + styleDim.Render("no artifact × agent rows — is anything installed and targeted?")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "   %-24s %-12s %-14s %s\n", "ARTIFACT", "KIND", "AGENT", "STATE")
	for i, r := range m.rows {
		cursor := "  "
		if i == m.agentCursor {
			cursor = styleCursor.Render("> ")
		}
		state := stateStyles[r.State].Render(string(r.State))
		fmt.Fprintf(&b, " %s%-24s %-12s %-14s %s\n", cursor, r.Artifact, r.Kind, r.Agent, state)
	}
	return b.String()
}

func (m *Model) renderSources() string {
	if len(m.sources) == 0 {
		return " " + styleDim.Render("no sources registered — 'loadout source add <url>'")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "   %-18s %-10s %-12s %s\n", "SOURCE", "REF", "PINNED", "REMOTE")
	for i, s := range m.sources {
		cursor := "  "
		if i == m.sourceCursor {
			cursor = styleCursor.Render("> ")
		}
		ref := s.Ref
		if ref == "" {
			ref = "(default)"
		}
		remote := styleDim.Render("press c to check")
		if r, ok := m.checks[s.Name]; ok {
			switch {
			case r.Err != nil:
				remote = styleErr.Render("error: " + r.Err.Error())
			case r.HasUpdate:
				remote = stateStyles[domain.StateDriftedStore].Render("update available — 'loadout update'")
			default:
				remote = stateStyles[domain.StateInSync].Render("up-to-date")
			}
		}
		fmt.Fprintf(&b, " %s%-18s %-10s %-12.10s %s\n", cursor, s.Name, ref, s.Commit, remote)
	}
	return b.String()
}

func (m *Model) renderLog() string {
	if len(m.logLines) == 0 {
		return " " + styleDim.Render("no activity yet this session")
	}
	visible := m.logLines
	maxLines := m.height - 6
	if maxLines > 0 && len(visible) > maxLines {
		visible = visible[len(visible)-maxLines:]
	}
	return " " + strings.Join(visible, "\n ")
}

func (m *Model) renderDiff() string {
	maxLines := m.height - 6
	if maxLines < 5 {
		maxLines = 5
	}
	end := m.diffOffset + maxLines
	if end > len(m.diffLines) {
		end = len(m.diffLines)
	}
	body := strings.Join(m.diffLines[m.diffOffset:end], "\n")
	position := fmt.Sprintf(" %d-%d/%d", m.diffOffset+1, end, len(m.diffLines))
	return body + "\n" + styleDim.Render(position)
}

func (m *Model) renderConfirm() string {
	changes := m.plan.Changes()
	var lines []string
	lines = append(lines, styleTitle.Render(fmt.Sprintf("Apply %d change(s)?", len(changes))), "")
	shown := changes
	if len(shown) > 12 {
		shown = shown[:12]
	}
	for _, it := range shown {
		lines = append(lines, fmt.Sprintf("%-6s %s: %s  (%s)", it.Op, it.Agent, it.RelPath, it.Artifact.ID))
	}
	if len(changes) > len(shown) {
		lines = append(lines, styleDim.Render(fmt.Sprintf("… and %d more", len(changes)-len(shown))))
	}
	lines = append(lines, "", "y apply · n cancel")
	return lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, styleModal.Render(strings.Join(lines, "\n")))
}

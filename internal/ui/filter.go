package ui

import (
	tea "charm.land/bubbletea/v2"
)

// "/" filtering, scoped independently per active panel: Explorer filters by
// filename among currently visible (expanded) rows, Document filters by
// block heading/content and restricts navigation to matches. Per
// DESIGN.md §9 open question #5, this implements the simpler "visible
// nodes only" scope rather than a recursive auto-expanding search.

func (m Model) startFilter() (tea.Model, tea.Cmd) {
	m.mode = modeFilter
	m.filterInput.Reset()
	m.filterInput.SetValue(m.activeFilter())
	m.filterInput.CursorEnd()
	return m, m.filterInput.Focus()
}

func (m Model) activeFilter() string {
	if m.focus == panelExplorer {
		return m.explorer.filter
	}
	return m.docView.filter
}

func (m Model) handleFilterKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.filterInput.Blur()
		m.applyFilter("")
		return m, nil
	case "enter":
		m.mode = modeBrowse
		m.filterInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.applyFilter(m.filterInput.Value())
	return m, cmd
}

func (m *Model) applyFilter(q string) {
	if m.focus == panelExplorer {
		m.explorer.setFilter(q)
	} else {
		m.docView.setFilter(q)
	}
}

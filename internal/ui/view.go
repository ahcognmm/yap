package ui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const appTitle = "yap"

func (m Model) View() tea.View {
	if m.tooSmall {
		msg := "terminal too small — need at least 60×15"
		return tea.NewView(lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, msg))
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	console := m.docView.renderConsole(m.theme, m.width)

	var body string
	switch {
	case !m.sidebarVisible:
		body = m.docView.render(m.theme, m.focus == panelDocument)
	case m.narrow:
		body = m.docView.render(m.theme, false)
		explorerBox := m.explorer.render(m.theme, true)
		body = renderOverlay(body, explorerBox, m.width, m.height-2)
	default:
		explorerBox := m.explorer.render(m.theme, m.focus == panelExplorer)
		docBox := m.docView.render(m.theme, m.focus == panelDocument)
		body = lipgloss.JoinHorizontal(lipgloss.Top, explorerBox, docBox)
	}

	parts := []string{header, body}
	if console != "" {
		parts = append(parts, console)
	}
	parts = append(parts, footer)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)

	if m.mode == modeHelp {
		helpBox := m.renderHelp()
		content = renderOverlay(content, helpBox, m.width, m.height)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

func (m Model) renderHeader() string {
	title := " " + appTitle + " "
	cwd := ""
	if m.explorer.root != nil {
		cwd = m.explorer.root.Path
	}
	if m.docView.hasDoc() {
		title = " " + appTitle + " ── " + m.docView.path
	} else {
		title = " " + appTitle + " ── " + cwd
	}
	title += "  [ctrl+b] explorer"

	style := lipgloss.NewStyle().
		Foreground(m.theme.AccentPrimary).
		Bold(true).
		Width(m.width)
	return style.Render(title)
}

func (m Model) renderFooter() string {
	if m.mode == modeFilter {
		style := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Width(m.width)
		return style.Render(" /" + m.filterInput.View())
	}

	if m.mode == modeConsole {
		style := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Width(m.width)
		return style.Render(" :" + m.consoleInput.View())
	}

	text := m.flash
	if text == "" {
		h := m.help
		h.ShowAll = false
		if m.focus == panelExplorer {
			text = h.View(m.explorer.keys)
		} else {
			text = h.View(m.docView.keys)
		}
	}
	style := lipgloss.NewStyle().Foreground(m.theme.TextMuted).Width(m.width)
	return style.Render(" " + text)
}

func (m Model) renderHelp() string {
	h := m.help
	h.ShowAll = true
	var body string
	if m.focus == panelExplorer {
		body = h.View(m.explorer.keys)
	} else {
		body = h.View(m.docView.keys)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.AccentPrimary).
		Padding(1, 2).
		Render("Help\n\n" + body)
	return box
}

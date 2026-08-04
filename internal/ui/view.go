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
	console := m.docView.renderConsole(m.theme, m.width, m.height, m.focus == panelConsole)

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

	// MaxHeight(1) matters: layout() budgets exactly one row for the header,
	// so a long path that wraps to two would push the whole frame past the
	// bottom of the terminal and clip the footer.
	style := lipgloss.NewStyle().
		Foreground(m.theme.AccentPrimary).
		Bold(true).
		Width(m.width).
		MaxHeight(1)
	return style.Render(title)
}

func (m Model) renderFooter() string {
	// Every branch clamps to one row: layout() budgets exactly one for the
	// footer, so anything taller pushes the frame past the terminal's last
	// line and the bottom of the UI gets clipped.
	prompt := lipgloss.NewStyle().Foreground(m.theme.TextPrimary).Width(m.width).MaxHeight(1)
	if m.mode == modeFilter {
		return prompt.Render(" /" + m.filterInput.View())
	}
	if m.mode == modeConsole {
		return prompt.Render(" :" + m.consoleInput.View())
	}

	text := m.flash
	if text == "" {
		h := m.help
		h.ShowAll = false
		// Without a width the help view lists every short-help binding on one
		// long line, which then wraps to a second row and overflows layout()'s
		// one-row footer budget. -1 for the leading space below.
		h.SetWidth(m.width - 1)
		switch m.focus {
		case panelExplorer:
			text = h.View(m.explorer.keys)
		case panelConsole:
			text = h.View(m.consoleKeys)
		default:
			text = h.View(m.docView.keys)
		}
	}
	style := lipgloss.NewStyle().Foreground(m.theme.TextMuted).Width(m.width).MaxHeight(1)
	return style.Render(" " + text)
}

func (m Model) renderHelp() string {
	h := m.help
	h.ShowAll = true
	var body string
	switch m.focus {
	case panelExplorer:
		body = h.View(m.explorer.keys)
	case panelConsole:
		body = h.View(m.consoleKeys)
	default:
		body = h.View(m.docView.keys)
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.AccentPrimary).
		Padding(1, 2).
		Render("Help\n\n" + body)
	return box
}

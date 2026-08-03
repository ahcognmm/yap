// Package ui implements the runmd top-level Bubble Tea model: an Explorer
// sidebar and a Document notebook pane, per DESIGN.md.
package ui

import (
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"

	"runmd/internal/markdown"
	"runmd/internal/theme"
)

const flashDuration = 3 * time.Second

// focusedPanel identifies which panel currently owns keyboard input.
type focusedPanel int

const (
	panelExplorer focusedPanel = iota
	panelDocument
)

// mode selects which full-screen overlay, if any, is showing.
type mode int

const (
	modeBrowse mode = iota
	modeHelp
	modeFilter
)

const (
	narrowBreakpoint = 90 // cols below which the Explorer collapses to an overlay
	minWidth         = 60
	minHeight        = 15
)

// Model is the top-level Bubble Tea model.
type Model struct {
	explorer ExplorerModel
	docView  DocumentModel

	theme theme.Theme
	ignore []string

	focus         focusedPanel
	sidebarVisible bool // Explorer shown at all — LazyVim-style global toggle
	mode          mode

	help help.Model

	filterInput textinput.Model

	flash    string
	flashGen int

	width, height int
	tooSmall      bool
	narrow        bool

	openOnStart string // absolute path to a .md file to open once, at startup

	quitting bool
}

// New builds the initial Model rooted at root. If openFile is non-empty, it
// is opened into the Document pane once scanning completes Init's startup
// commands.
func New(root string, openFile string, th theme.Theme, ignore []string) Model {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}

	h := help.New()
	h.Styles = help.DefaultDarkStyles()

	fi := textinput.New()
	fi.Placeholder = "filter"

	return Model{
		explorer:       newExplorerModel(abs, ignore),
		docView:        newDocumentModel(),
		theme:          th,
		ignore:         ignore,
		focus:          panelExplorer,
		sidebarVisible: true,
		help:           h,
		filterInput:    fi,
		openOnStart:    openFile,
	}
}

func (m Model) Init() tea.Cmd {
	if m.openOnStart != "" {
		return tea.Batch(m.explorer.scanRootCmd(), readFileCmd(m.openOnStart))
	}
	return m.explorer.scanRootCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.tooSmall = m.width < minWidth || m.height < minHeight
		m.narrow = m.width < narrowBreakpoint
		m.layout()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)

	case fileLoadedMsg:
		return m.handleFileLoaded(msg)

	case editorFinishedMsg:
		if msg.err != nil {
			return m, flashCmd("editor exited with error: " + msg.err.Error())
		}
		return m, readFileCmd(msg.path)

	case dirScannedMsg:
		m.explorer.applyScan(msg)
		return m, nil

	case runStartedMsg, execLineMsg, execDoneMsg, runTickMsg:
		var cmd tea.Cmd
		m.docView, cmd = m.docView.Update(msg)
		return m, cmd

	case footerFlashMsg:
		m.flashGen++
		m.flash = msg.text
		return m, clearFlashCmd(m.flashGen)

	case footerFlashClearMsg:
		if msg.gen == m.flashGen {
			m.flash = ""
		}
		return m, nil
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Global keys first.
	switch msg.String() {
	case "ctrl+c":
		if m.focus == panelDocument && m.docView.isRunning() {
			return m, m.docView.cancelRunCmd()
		}
		m.quitting = true
		m.docView.closeShell()
		return m, tea.Quit
	case "?":
		if m.mode == modeHelp {
			m.mode = modeBrowse
		} else {
			m.mode = modeHelp
		}
		return m, nil
	case "ctrl+b":
		m.sidebarVisible = !m.sidebarVisible
		if m.sidebarVisible {
			m.focus = panelExplorer
		} else if m.focus == panelExplorer {
			m.focus = panelDocument
		}
		m.layout()
		return m, nil
	}

	if m.mode == modeHelp {
		switch msg.String() {
		case "q", "esc", "?":
			m.mode = modeBrowse
		}
		return m, nil
	}

	if m.mode == modeFilter {
		return m.handleFilterKey(msg)
	}

	if m.narrow && m.sidebarVisible {
		switch msg.String() {
		case "esc":
			m.sidebarVisible = false
			if m.focus == panelExplorer {
				m.focus = panelDocument
			}
			return m, nil
		}
		if m.focus == panelExplorer {
			return m.updateExplorer(msg)
		}
	}

	switch msg.String() {
	case "q":
		m.quitting = true
		m.docView.closeShell()
		return m, tea.Quit
	case "tab":
		if m.docView.hasDoc() {
			if m.focus == panelExplorer {
				m.focus = panelDocument
			} else {
				m.focus = panelExplorer
			}
		}
		return m, nil
	case "/":
		return m.startFilter()
	}

	if m.focus == panelExplorer {
		return m.updateExplorer(msg)
	}
	return m.updateDocument(msg)
}

func (m Model) updateExplorer(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var opened string
	m.explorer, cmd, opened = m.explorer.handleKey(msg)
	if opened != "" {
		return m, tea.Batch(cmd, readFileCmd(opened))
	}
	return m, cmd
}

func (m Model) updateDocument(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.docView, cmd = m.docView.handleKey(msg)
	return m, cmd
}

func (m Model) handleFileLoaded(msg fileLoadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		return m, flashCmd("could not open " + filepath.Base(msg.path) + ": " + msg.err.Error())
	}
	blocks := markdown.Parse(msg.content)
	m.docView = m.docView.loadDocument(msg.path, blocks)
	m.explorer.openPath = msg.path
	m.focus = panelDocument
	if m.narrow {
		m.sidebarVisible = false
	}
	return m, nil
}

// layout recomputes each panel's frame size from the terminal size. Panel
// height is always terminal_height - header - footer, never
// len(explorer.flat) — short trees pad, long trees scroll internally.
func (m *Model) layout() {
	const headerHeight = 1
	const footerHeight = 1
	innerHeight := m.height - headerHeight - footerHeight
	if innerHeight < 0 {
		innerHeight = 0
	}

	if m.narrow {
		m.docView.setSize(m.width, innerHeight)
		m.explorer.setSize(minInt(m.width-4, 40), minInt(innerHeight-4, 20))
		return
	}

	if !m.sidebarVisible {
		m.docView.setSize(m.width, innerHeight)
		return
	}

	explorerWidth := m.width / 4
	if explorerWidth < 24 {
		explorerWidth = 24
	}
	if explorerWidth > 40 {
		explorerWidth = 40
	}
	docWidth := m.width - explorerWidth
	m.explorer.setSize(explorerWidth, innerHeight)
	m.docView.setSize(docWidth, innerHeight)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func readFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		return fileLoadedMsg{path: path, content: content, err: err}
	}
}

func flashCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return footerFlashMsg{text: text}
	}
}

func clearFlashCmd(gen int) tea.Cmd {
	return tea.Tick(flashDuration, func(time.Time) tea.Msg {
		return footerFlashClearMsg{gen: gen}
	})
}

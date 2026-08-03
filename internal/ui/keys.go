package ui

import "charm.land/bubbles/v2/key"

// ExplorerKeyMap is the Explorer panel's key.Map — its footer hints are
// rendered from this, not a shared global map, so the footer never shows
// Document-only actions while the Explorer has focus.
type ExplorerKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	Open       key.Binding
	Tab        key.Binding
	ToggleTree key.Binding
	Filter     key.Binding
	Top        key.Binding
	Bottom     key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func NewExplorerKeyMap() ExplorerKeyMap {
	return ExplorerKeyMap{
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "move up")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "move down")),
		Open:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "toggle/open")),
		Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch panel")),
		ToggleTree: key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "toggle explorer")),
		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

func (k ExplorerKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Open, k.ToggleTree, k.Quit}
}

func (k ExplorerKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.Open},
		{k.Tab, k.ToggleTree, k.Filter, k.Help, k.Quit},
	}
}

// DocumentKeyMap is the Document panel's key.Map.
type DocumentKeyMap struct {
	Up         key.Binding
	Down       key.Binding
	PageUp     key.Binding
	PageDown   key.Binding
	Run         key.Binding
	RunParallel key.Binding
	Cancel      key.Binding
	Copy       key.Binding
	Edit       key.Binding
	Console    key.Binding
	Scratch    key.Binding
	Tab        key.Binding
	ToggleTree key.Binding
	Filter     key.Binding
	Top        key.Binding
	Bottom     key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func NewDocumentKeyMap() DocumentKeyMap {
	return DocumentKeyMap{
		Up:         key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "select block")),
		Down:       key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "select block")),
		PageUp:     key.NewBinding(key.WithKeys("ctrl+u", "pgup"), key.WithHelp("ctrl+u", "scroll up")),
		PageDown:   key.NewBinding(key.WithKeys("ctrl+d", "pgdown"), key.WithHelp("ctrl+d", "scroll down")),
		Run: key.NewBinding(key.WithKeys("enter", "r"), key.WithHelp("enter/r", "run")),
		// shift+enter needs the Kitty keyboard protocol to be distinguishable
		// from plain enter, which not every terminal speaks — "R" is the
		// always-available equivalent.
		RunParallel: key.NewBinding(key.WithKeys("shift+enter", "R"), key.WithHelp("R", "run parallel")),
		Cancel:      key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "cancel")),
		Copy:       key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "copy output")),
		Edit:       key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit")),
		Console:    key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "run in session")),
		Scratch:    key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit+run script")),
		Tab:        key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "switch panel")),
		ToggleTree: key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "toggle explorer")),
		Filter:     key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Top:        key.NewBinding(key.WithKeys("g"), key.WithHelp("g", "top")),
		Bottom:     key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

func (k DocumentKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Run, k.Console, k.Edit, k.Tab, k.ToggleTree, k.Quit}
}

func (k DocumentKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Top, k.Bottom},
		{k.PageUp, k.PageDown},
		{k.Run, k.RunParallel, k.Cancel, k.Copy, k.Edit},
		{k.Console, k.Scratch},
		{k.Tab, k.ToggleTree, k.Filter, k.Help, k.Quit},
	}
}

// GlobalKeyMap holds bindings meaningful regardless of focus.
type GlobalKeyMap struct {
	ToggleTree key.Binding
	Help       key.Binding
	Quit       key.Binding
}

func NewGlobalKeyMap() GlobalKeyMap {
	return GlobalKeyMap{
		ToggleTree: key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "explorer")),
		Help:       key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:       key.NewBinding(key.WithKeys("q", "esc"), key.WithHelp("q", "quit")),
	}
}

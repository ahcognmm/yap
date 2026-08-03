package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"

	"yap/internal/fstree"
)

// ExplorerModel is the file-tree sidebar's state. It is a plain data model
// — flattening happens on expand/collapse so keypress handling never
// re-walks the tree.
type ExplorerModel struct {
	root   *fstree.FileNode
	flat   []fstree.FlatRow
	cursor int
	scroll int // first visible row, for scrolling a fixed-height viewport

	openPath string // path of file currently loaded in Document, for the ● marker
	filter   string // active "/" filter, empty = none

	ignore []string
	keys   ExplorerKeyMap

	width, height int
}

func newExplorerModel(root string, ignore []string) ExplorerModel {
	return ExplorerModel{
		root:   fstree.NewRoot(root),
		ignore: ignore,
		keys:   NewExplorerKeyMap(),
	}
}

func (e *ExplorerModel) setSize(w, h int) {
	e.width, e.height = w, h
}

func (e ExplorerModel) scanRootCmd() tea.Cmd {
	return scanDirCmd(e.root, e.ignore)
}

func scanDirCmd(node *fstree.FileNode, ignore []string) tea.Cmd {
	return func() tea.Msg {
		children, err := fstree.ScanDir(node.Path, ignore, map[string]bool{})
		return dirScannedMsg{node: node, children: children, err: err}
	}
}

// applyScan populates node.Children from a completed lazy scan and
// reflattens the visible rows.
func (e *ExplorerModel) applyScan(msg dirScannedMsg) {
	if msg.err != nil {
		return
	}
	msg.node.Children = msg.children
	if msg.node == e.root {
		msg.node.Expanded = true
	}
	e.reflatten()
}

func (e *ExplorerModel) reflatten() {
	rows := fstree.Flatten(e.root)
	if e.filter != "" {
		rows = filterRows(rows, e.filter)
	}
	e.flat = rows
	if e.cursor >= len(e.flat) {
		e.cursor = len(e.flat) - 1
	}
	if e.cursor < 0 {
		e.cursor = 0
	}
}

// setFilter updates the Explorer's active filename filter, scoped to
// currently visible (expanded) nodes only, and re-derives the flattened row
// list.
func (e *ExplorerModel) setFilter(q string) {
	e.filter = q
	e.reflatten()
}

func filterRows(rows []fstree.FlatRow, q string) []fstree.FlatRow {
	q = strings.ToLower(q)
	out := make([]fstree.FlatRow, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Node.Name), q) {
			out = append(out, r)
		}
	}
	return out
}

// handleKey processes a keypress while the Explorer has focus. It returns
// an updated model, a command, and — when the user opened a Markdown file
// — that file's path (for the caller to dispatch readFileCmd).
func (e ExplorerModel) handleKey(msg tea.KeyPressMsg) (ExplorerModel, tea.Cmd, string) {
	switch {
	case key.Matches(msg, e.keys.Down):
		if e.cursor < len(e.flat)-1 {
			e.cursor++
			e.ensureVisible()
		}
	case key.Matches(msg, e.keys.Up):
		if e.cursor > 0 {
			e.cursor--
			e.ensureVisible()
		}
	case key.Matches(msg, e.keys.Top):
		e.cursor = 0
		e.ensureVisible()
	case key.Matches(msg, e.keys.Bottom):
		e.cursor = len(e.flat) - 1
		e.ensureVisible()
	case key.Matches(msg, e.keys.Open):
		return e.openCursor()
	}
	return e, nil, ""
}

func (e ExplorerModel) cursorNode() *fstree.FileNode {
	if e.cursor < 0 || e.cursor >= len(e.flat) {
		return nil
	}
	return e.flat[e.cursor].Node
}

// openCursor handles enter on the cursor row: for a directory it toggles
// expand/collapse in place; for a file it opens it into the Document pane
// (Markdown only).
func (e ExplorerModel) openCursor() (ExplorerModel, tea.Cmd, string) {
	n := e.cursorNode()
	if n == nil {
		return e, nil, ""
	}
	if n.IsDir {
		if n.Expanded {
			return e.collapseCursor()
		}
		return e.expandCursor()
	}
	if !n.IsMD {
		return e, flashCmd("not a markdown file"), ""
	}
	return e, nil, n.Path
}

func (e ExplorerModel) collapseCursor() (ExplorerModel, tea.Cmd, string) {
	n := e.cursorNode()
	if n != nil && n.IsDir && n.Expanded {
		n.Expanded = false
		e.reflatten()
		return e, nil, ""
	}

	// Cursor isn't sitting on an expanded dir (it's a file, or an
	// already-collapsed dir) — walk up to the nearest ancestor directory and
	// collapse that instead, ranger/yazi-style, so collapse always does
	// something useful while browsing inside an open folder instead of a
	// silent no-op.
	if e.cursor < 0 || e.cursor >= len(e.flat) {
		return e, nil, ""
	}
	depth := e.flat[e.cursor].Depth
	for i := e.cursor - 1; i >= 0; i-- {
		if e.flat[i].Depth < depth {
			parent := e.flat[i].Node
			if parent.IsDir && parent.Expanded {
				parent.Expanded = false
				e.cursor = i
				e.reflatten()
				e.ensureVisible()
			}
			return e, nil, ""
		}
	}
	return e, nil, ""
}

func (e ExplorerModel) expandCursor() (ExplorerModel, tea.Cmd, string) {
	n := e.cursorNode()
	if n == nil || !n.IsDir {
		return e, nil, ""
	}
	if n.Expanded {
		return e, nil, ""
	}
	n.Expanded = true
	if n.Children == nil {
		return e, scanDirCmd(n, e.ignore), ""
	}
	e.reflatten()
	return e, nil, ""
}

// ensureVisible scrolls the fixed-height viewport so the cursor row stays
// on screen.
func (e *ExplorerModel) ensureVisible() {
	if e.height <= 0 {
		return
	}
	if e.cursor < e.scroll {
		e.scroll = e.cursor
	}
	if e.cursor >= e.scroll+e.height {
		e.scroll = e.cursor - e.height + 1
	}
}

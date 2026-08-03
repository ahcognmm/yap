package ui

import "runmd/internal/fstree"

// fileLoadedMsg carries the result of reading a Markdown file selected in
// the Explorer.
type fileLoadedMsg struct {
	path    string
	content []byte
	err     error
}

// dirScannedMsg carries the result of a lazy per-directory scan triggered
// by expanding a folder in the Explorer.
type dirScannedMsg struct {
	node     *fstree.FileNode
	children []*fstree.FileNode
	err      error
}

// execLineMsg is one streamed line of output from a running code block.
type execLineMsg struct {
	blockIdx int
	gen      int // Session generation, guards against stale messages after cancel/rerun
	line     string
}

// execDoneMsg reports a code block's process exit.
type execDoneMsg struct {
	blockIdx int
	gen      int
	exitCode int
	err      error
}

// footerFlashMsg sets a transient footer hint message (e.g. "not a
// markdown file"), cleared by footerFlashClearMsg.
type footerFlashMsg struct {
	text string
}

// footerFlashClearMsg clears a footerFlashMsg after its display duration.
type footerFlashClearMsg struct {
	gen int
}

// editorFinishedMsg reports that the external $EDITOR process launched for
// "e" exited — path is the file it was editing, so the caller can reload it.
type editorFinishedMsg struct {
	path string
	err  error
}

// runTickMsg fires once a second while a block is running, purely to
// redraw its live elapsed-time label — carries no data of its own beyond
// the guard fields.
type runTickMsg struct {
	blockIdx int
	gen      int
}

// sessionLineMsg is one streamed line from an ad-hoc ":" command run
// directly in the document's shared shell.
type sessionLineMsg struct {
	gen  int
	line string
}

// sessionDoneMsg reports an ad-hoc ":" command's exit.
type sessionDoneMsg struct {
	gen      int
	exitCode int
	err      error
}

// scratchEditedMsg reports that the $EDITOR launched for the session scratch
// buffer exited — path is the temp file holding the script to run.
type scratchEditedMsg struct {
	path string
	err  error
}

// runStartedMsg reports that a queued block has actually begun executing
// in the shared shell (as opposed to still waiting behind a prior block).
type runStartedMsg struct {
	blockIdx int
	gen      int
}

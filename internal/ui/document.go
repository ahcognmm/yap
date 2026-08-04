package ui

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"

	"yap/internal/exec"
	"yap/internal/markdown"
	"yap/internal/theme"
)

// runState is a runnable code block's execution status.
type runState int

const (
	stateIdle runState = iota
	stateQueued
	stateRunning
	stateSuccess
	stateFail
)

// blockState is the per-block mutable execution state — kept alongside the
// (read-only) parsed Block since source files are never mutated.
type blockState struct {
	state    runState
	output   []string
	exitCode int
	err      error

	startedAt  time.Time
	finishedAt time.Time

	run      *exec.Run
	detached bool // ran via RunParallel, so it owns its own shell process
	gen      int  // guards stale execLineMsg/execDoneMsg after cancel or rerun
}

// elapsed reports how long the block has been (or was) running, for display
// in the code block's status label — live while running, frozen once done.
func (st blockState) elapsed() time.Duration {
	if st.startedAt.IsZero() {
		return 0
	}
	if st.state == stateRunning {
		return time.Since(st.startedAt).Round(time.Second)
	}
	return st.finishedAt.Sub(st.startedAt).Round(10 * time.Millisecond)
}

// DocumentModel is the single-pane notebook: prose rendered via Glamour,
// shell code blocks selectable and runnable in place, output streamed
// inline below the block.
type DocumentModel struct {
	path   string
	blocks []markdown.Block
	states []blockState

	shell *exec.ShellSession

	// console is the ad-hoc ":" command run in the shared shell — same
	// session as the code blocks, so an `export` here is visible to every
	// block run afterwards.
	console consoleState

	selected int
	filter   string

	// The rendered document lives in the viewport rather than being rebuilt
	// during View(), so scrolling keys can act on real content — and so a
	// keystroke doesn't pay for re-running Glamour over every prose block.
	// offsets is each block's first line within that content (-1 when the
	// block is hidden by the filter).
	vp      viewport.Model
	offsets []int
	theme   theme.Theme
	keys    DocumentKeyMap

	width, height int
}

func newDocumentModel(th theme.Theme) DocumentModel {
	return DocumentModel{
		vp:      viewport.New(),
		console: newConsoleState(),
		theme:   th,
		keys:    NewDocumentKeyMap(),
	}
}

// rebuild re-renders the document into the viewport. Call it after anything
// that changes what the document looks like: the open file, the filter, the
// selection (the selected block's border is thicker), block output, or width.
func (d *DocumentModel) rebuild() {
	if !d.hasDoc() {
		d.vp.SetContent("")
		d.offsets = nil
		return
	}
	content, offsets := d.renderBlocks(d.theme, d.vp.Width())
	d.offsets = offsets
	d.vp.SetContent(content)
}

// scrollToSelected brings the selected block into view, for the block-jump
// keys — plain scrolling deliberately leaves the selection where it is.
func (d *DocumentModel) scrollToSelected() {
	if d.selected >= 0 && d.selected < len(d.offsets) && d.offsets[d.selected] >= 0 {
		d.vp.EnsureVisible(d.offsets[d.selected], 0, 0)
	}
}

// documentFrameSize is the border's frame size (2 cols, 2 rows), fixed for
// every Document panel style — kept here so setSize can size d.vp to the
// real scrollable area without constructing a lipgloss.Style just to ask.
const (
	documentFrameW = 2
	documentFrameH = 2
)

func (d *DocumentModel) setSize(w, h int) {
	d.width, d.height = w, h
	innerW, innerH := w-documentFrameW, h-documentFrameH
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	d.vp.SetWidth(innerW)
	d.vp.SetHeight(innerH)
	d.rebuild() // content is width-wrapped, so a resize invalidates it
}

func (d DocumentModel) hasDoc() bool {
	return d.path != ""
}

func (d DocumentModel) isRunning() bool {
	for i := range d.states {
		if d.states[i].state == stateRunning || d.states[i].state == stateQueued {
			return true
		}
	}
	return false
}

// loadDocument replaces the open document. Per the v1 assumption, opening a
// new file discards the replaced file's execution state entirely.
func (d DocumentModel) loadDocument(path string, blocks []markdown.Block) DocumentModel {
	d.closeShell()
	// New file, new shell — old session output is meaningless. Keep the
	// viewport's dimensions, which layout() already set for this terminal.
	vp := d.console.vp
	vp.SetContent("")
	d.console = consoleState{vp: vp}
	d.path = path
	d.blocks = blocks
	d.states = make([]blockState, len(blocks))
	d.selected = firstRunnableOrZero(blocks)
	d.rebuild()
	d.vp.SetYOffset(0)
	return d
}

// setFilter updates the Document's active block filter (matched against
// each block's heading and raw content) and moves the selection to the
// nearest matching block, if any.
func (d *DocumentModel) setFilter(q string) {
	d.filter = q
	if q != "" && !d.selectable(d.selected) {
		d.selectEdge(1)
	}
	d.rebuild()
	d.scrollToSelected()
}

func (d DocumentModel) matchesFilter(i int) bool {
	if d.filter == "" {
		return true
	}
	if i < 0 || i >= len(d.blocks) {
		return false
	}
	q := strings.ToLower(d.filter)
	b := d.blocks[i]
	return strings.Contains(strings.ToLower(b.Heading), q) ||
		strings.Contains(strings.ToLower(b.Raw), q)
}

func firstRunnableOrZero(blocks []markdown.Block) int {
	for i, b := range blocks {
		if b.Kind == markdown.KindCode {
			return i
		}
	}
	return 0
}

// selectable reports whether block i is a valid cursor target — only code
// blocks are, since prose can't be run, copied, or acted on in any way.
func (d DocumentModel) selectable(i int) bool {
	if i < 0 || i >= len(d.blocks) {
		return false
	}
	return d.blocks[i].Kind == markdown.KindCode && d.matchesFilter(i)
}

// codeBlockPosition returns the selected block's 1-based position among the
// document's code blocks, and how many there are — prose is excluded, so the
// header counter matches what the user can actually select.
func (d DocumentModel) codeBlockPosition() (pos, total int) {
	for i, b := range d.blocks {
		if b.Kind != markdown.KindCode {
			continue
		}
		total++
		if i == d.selected {
			pos = total
		}
	}
	return pos, total
}

// closeShell tears down every process the document owns: its persistent
// shell, plus any detached runs, which have their own processes and would
// otherwise outlive the document (or the app) as orphans. Called when a
// different file is opened (new working directory, so a fresh shell is
// warranted anyway) and at app quit.
func (d *DocumentModel) closeShell() {
	if d.shell != nil {
		d.shell.Close()
		d.shell = nil
	}
	for i := range d.states {
		st := &d.states[i]
		if st.detached && st.run != nil {
			st.run.Kill()
		}
	}
}

func (d DocumentModel) handleKey(msg tea.KeyPressMsg) (DocumentModel, tea.Cmd) {
	switch {
	// j/k scroll the rendered document by a line, like any pager — they do
	// not move the selection. Jumping between code blocks is n/N, so reading
	// prose and choosing what to run stay separate motions.
	case key.Matches(msg, d.keys.Down):
		d.vp.ScrollDown(1)
	case key.Matches(msg, d.keys.Up):
		d.vp.ScrollUp(1)
	case key.Matches(msg, d.keys.NextBlock):
		d.moveSelection(1)
		d.rebuild()
		d.scrollToSelected()
	case key.Matches(msg, d.keys.PrevBlock):
		d.moveSelection(-1)
		d.rebuild()
		d.scrollToSelected()
	case key.Matches(msg, d.keys.Top):
		d.vp.GotoTop()
	case key.Matches(msg, d.keys.Bottom):
		d.vp.GotoBottom()
	case key.Matches(msg, d.keys.PageDown):
		d.vp.HalfPageDown()
	case key.Matches(msg, d.keys.PageUp):
		d.vp.HalfPageUp()
	case key.Matches(msg, d.keys.Run):
		return d.runSelected(false)
	case key.Matches(msg, d.keys.RunParallel):
		return d.runSelected(true)
	case key.Matches(msg, d.keys.Copy):
		return d, d.copySelectedOutputCmd()
	case key.Matches(msg, d.keys.Edit):
		return d, d.editCmd()
	}
	return d, nil
}

// editCmd suspends the TUI and shells out to $EDITOR (falling back to "vi")
// on the currently open file. The program resumes once the editor exits;
// the caller reloads the file from editorFinishedMsg.
func (d DocumentModel) editCmd() tea.Cmd {
	if !d.hasDoc() {
		return nil
	}
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	path := d.path
	c := osexec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{path: path, err: err}
	})
}

// selectEdge jumps to the first (dir 1) or last (dir -1) selectable block.
func (d *DocumentModel) selectEdge(dir int) {
	start := 0
	if dir < 0 {
		start = len(d.blocks) - 1
	}
	for i := start; i >= 0 && i < len(d.blocks); i += dir {
		if d.selectable(i) {
			d.selected = i
			return
		}
	}
}

// moveSelection steps to the next/previous selectable (code) block, skipping
// prose entirely — prose can't be run or copied, so stopping on it would be a
// dead keypress. Wraps around at either end, so n keeps cycling through the
// document's blocks rather than dead-ending on the last one. Stays put if
// there's nothing selectable at all (e.g. a filter that matches nothing).
func (d *DocumentModel) moveSelection(delta int) {
	if len(d.blocks) == 0 {
		return
	}
	n := d.selected
	for i := 0; i < len(d.blocks); i++ {
		n = (n + delta + len(d.blocks)) % len(d.blocks)
		if d.selectable(n) {
			d.selected = n
			return
		}
	}
}

// runSelected runs the selected block. With detached false it goes into the
// document's shared shell, queuing behind anything already running there so
// the block sees earlier blocks' exports and cwd. With detached true it gets
// its own throwaway shell and starts immediately, running alongside whatever
// else is in flight — at the cost of not sharing the session's state.
func (d DocumentModel) runSelected(detached bool) (DocumentModel, tea.Cmd) {
	if d.selected < 0 || d.selected >= len(d.blocks) {
		return d, nil
	}
	block := d.blocks[d.selected]
	if block.Kind != markdown.KindCode || !block.Runnable {
		return d, nil
	}

	// Blocks queued into the shared shell are never implicitly cancelled by
	// running another one — only an explicit ctrl+c (cancelRunCmd) sends
	// SIGINT. Doing it implicitly here was racy: SIGINT could land while the
	// shell was mid completion-marker write, breaking the marker protocol and
	// leaving the block stuck at "running" forever even though it had
	// finished.
	st := &d.states[d.selected]
	st.gen++
	st.state = stateQueued
	st.output = nil
	st.exitCode = 0
	st.err = nil
	// Seeded here so a block that finishes before its runStartedMsg is
	// delivered still reports a real duration; runStartedMsg overwrites this
	// with the true start once the shell actually reaches the block.
	st.startedAt = time.Now()
	st.finishedAt = time.Time{}
	st.detached = detached

	dir := filepath.Dir(d.path)

	var run *exec.Run
	if detached {
		r, err := exec.Detached(dir, block.Raw)
		if err != nil {
			st.state = stateFail
			st.err = err
			d.rebuild()
			return d, nil
		}
		run = r
	} else {
		if d.shell == nil || d.shell.Closed() {
			shell, err := exec.StartShell(dir)
			if err != nil {
				st.state = stateFail
				st.err = err
				d.rebuild()
				return d, nil
			}
			d.shell = shell
		}
		run = d.shell.Run(block.Raw)
	}
	st.run = run

	idx := d.selected
	gen := st.gen
	d.rebuild()
	d.scrollToSelected()
	return d, tea.Batch(
		waitStartedCmd(run, idx, gen),
		readLineCmd(run, idx, gen),
		waitDoneCmd(run, idx, gen),
	)
}

// runTickCmd redraws once a second while a block is running, so its elapsed
// time counts up live instead of only updating on output/exit.
func runTickCmd(blockIdx, gen int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return runTickMsg{blockIdx: blockIdx, gen: gen}
	})
}

// cancelRunCmd cancels every in-flight block: detached runs are signalled
// individually since each owns its own process, and the shared shell is
// interrupted once if anything is running in it.
func (d DocumentModel) cancelRunCmd() tea.Cmd {
	shared := false
	for i := range d.states {
		st := &d.states[i]
		if st.state != stateRunning && st.state != stateQueued {
			continue
		}
		if st.detached {
			st.run.Cancel()
		} else {
			shared = true
		}
	}
	if shared && d.shell != nil {
		d.shell.Cancel()
	}
	return nil
}

func (d DocumentModel) Update(msg tea.Msg) (DocumentModel, tea.Cmd) {
	switch msg := msg.(type) {
	case runStartedMsg:
		if msg.blockIdx < 0 || msg.blockIdx >= len(d.states) {
			return d, nil
		}
		st := &d.states[msg.blockIdx]
		if msg.gen != st.gen || st.state != stateQueued {
			// Stale (cancelled/replaced run), or the block already finished:
			// waitStartedCmd and waitDoneCmd run concurrently in the same
			// tea.Batch, so a block fast enough to finish immediately (echo)
			// can deliver its done message first. Without this guard the late
			// started message would drag the block back to "running" forever.
			return d, nil
		}
		st.state = stateRunning
		st.startedAt = time.Now()
		d.rebuild()
		return d, runTickCmd(msg.blockIdx, msg.gen)

	case execLineMsg:
		if msg.blockIdx < 0 || msg.blockIdx >= len(d.states) {
			return d, nil
		}
		st := &d.states[msg.blockIdx]
		if msg.gen != st.gen {
			return d, nil // stale, from a cancelled/replaced run
		}
		st.output = append(st.output, msg.line)
		d.rebuild()
		return d, readLineCmd(st.run, msg.blockIdx, msg.gen)

	case execDoneMsg:
		if msg.blockIdx < 0 || msg.blockIdx >= len(d.states) {
			return d, nil
		}
		st := &d.states[msg.blockIdx]
		if msg.gen != st.gen {
			return d, nil
		}
		st.exitCode = msg.exitCode
		st.err = msg.err
		st.finishedAt = time.Now()
		if msg.err != nil || msg.exitCode != 0 {
			st.state = stateFail
		} else {
			st.state = stateSuccess
		}
		d.rebuild()
		return d, nil

	case sessionLineMsg:
		if msg.gen != d.console.gen {
			return d, nil
		}
		// Only ever promote queued -> running. consoleLineCmd and
		// consoleDoneCmd run concurrently in the same tea.Batch, so the done
		// message can land before the last line; assigning stateRunning
		// unconditionally here would drag a finished command back to
		// "running…" forever.
		if d.console.state == stateQueued {
			d.console.state = stateRunning
		}
		d.console.output = append(d.console.output, msg.line)
		d.rebuildConsole()
		return d, consoleLineCmd(d.console.run, msg.gen)

	case sessionDoneMsg:
		if msg.gen != d.console.gen {
			return d, nil
		}
		d.console.err = msg.err
		if msg.err != nil || msg.exitCode != 0 {
			d.console.state = stateFail
		} else {
			d.console.state = stateSuccess
		}
		return d, nil

	case runTickMsg:
		if msg.blockIdx < 0 || msg.blockIdx >= len(d.states) {
			return d, nil
		}
		st := &d.states[msg.blockIdx]
		if msg.gen != st.gen || st.state != stateRunning {
			return d, nil // stale tick from a cancelled/finished/replaced run
		}
		d.rebuild() // the elapsed-time label just ticked over
		return d, runTickCmd(msg.blockIdx, msg.gen)
	}
	return d, nil
}

// waitStartedCmd blocks until the shared shell actually begins executing
// this block (as opposed to still sitting behind a prior block in the
// queue) — only then does the elapsed-time clock/tick start.
func waitStartedCmd(r *exec.Run, blockIdx, gen int) tea.Cmd {
	return func() tea.Msg {
		<-r.Started
		return runStartedMsg{blockIdx: blockIdx, gen: gen}
	}
}

func readLineCmd(r *exec.Run, blockIdx, gen int) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-r.Lines
		if !ok {
			return nil
		}
		return execLineMsg{blockIdx: blockIdx, gen: gen, line: line}
	}
}

func waitDoneCmd(r *exec.Run, blockIdx, gen int) tea.Cmd {
	return func() tea.Msg {
		result := <-r.Done
		return execDoneMsg{blockIdx: blockIdx, gen: gen, exitCode: result.ExitCode, err: result.Err}
	}
}

func (d DocumentModel) copySelectedOutputCmd() tea.Cmd {
	if d.selected < 0 || d.selected >= len(d.states) {
		return nil
	}
	out := d.states[d.selected].output
	if len(out) == 0 {
		return nil
	}
	text := ""
	for i, l := range out {
		if i > 0 {
			text += "\n"
		}
		text += l
	}
	return tea.SetClipboard(text)
}

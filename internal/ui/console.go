package ui

import (
	"fmt"
	"image/color"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"

	"yap/internal/exec"
	"yap/internal/theme"
)

// consoleState is the ":" ad-hoc command run against the document's shared
// shell — the same process the code blocks use, so `export FOO=bar` typed
// here is visible to every block run afterwards. That's the point: it lets
// you set up secrets, PATH entries, or a virtualenv without editing the file.
type consoleState struct {
	cmd    string
	output []string
	state  runState
	err    error
	run    *exec.Run
	gen    int
	size   consoleSize

	// vp holds the expanded pane's scrollback. A command's log can be far
	// longer than any pane height, so expanded is a real scrollable box rather
	// than a tail — the collapsed peek doesn't use this.
	vp viewport.Model
}

func newConsoleState() consoleState {
	vp := viewport.New()
	vp.FillHeight = true
	return consoleState{vp: vp}
}

// consoleSize is how much of the console log the pane shows. A command's
// output is usually longer than the few lines that fit under the document, so
// the pane cycles between a peek, the full log, and out of the way entirely.
type consoleSize int

const (
	consoleHidden consoleSize = iota
	consoleCollapsed
	consoleExpanded
)

// next cycles the pane: collapsed → expanded → hidden → collapsed.
func (s consoleSize) next() consoleSize {
	switch s {
	case consoleCollapsed:
		return consoleExpanded
	case consoleExpanded:
		return consoleHidden
	default:
		return consoleCollapsed
	}
}

// hint is the label shown in the pane header for what the toggle key will do
// next, so the cycle is discoverable without opening help.
func (s consoleSize) hint() string {
	switch s {
	case consoleCollapsed:
		return "expand"
	case consoleExpanded:
		return "hide"
	default:
		return "show"
	}
}

// runConsole executes cmd in the document's shared shell, starting the shell
// first if the document doesn't have one yet.
func (d DocumentModel) runConsole(cmd string) (DocumentModel, tea.Cmd) {
	if cmd == "" || !d.hasDoc() {
		return d, nil
	}

	if d.shell == nil || d.shell.Closed() {
		shell, err := exec.StartShell(filepath.Dir(d.path))
		if err != nil {
			d.console.state = stateFail
			d.console.err = err
			d.console.size = consoleCollapsed
			return d, nil
		}
		d.shell = shell
	}

	d.console.gen++
	d.console.cmd = cmd
	d.console.output = nil
	d.console.err = nil
	d.console.state = stateQueued
	// A fresh command reopens the pane, but keeps an already-expanded one
	// expanded — someone reading a long log doesn't want it collapsing under
	// them on every follow-up command.
	if d.console.size == consoleHidden {
		d.console.size = consoleCollapsed
	}
	d.console.vp.GotoTop() // a fresh command starts at its own first line
	d.rebuildConsole()

	run := d.shell.Run(cmd)
	d.console.run = run

	gen := d.console.gen
	return d, tea.Batch(
		consoleLineCmd(run, gen),
		consoleDoneCmd(run, gen),
	)
}

func consoleLineCmd(r *exec.Run, gen int) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-r.Lines
		if !ok {
			return nil
		}
		return sessionLineMsg{gen: gen, line: line}
	}
}

func consoleDoneCmd(r *exec.Run, gen int) tea.Cmd {
	return func() tea.Msg {
		res := <-r.Done
		return sessionDoneMsg{gen: gen, exitCode: res.ExitCode, err: res.Err}
	}
}

// startConsole opens the ":" prompt. It needs an open document, since the
// command runs in that document's shell session.
func (m Model) startConsole() (tea.Model, tea.Cmd) {
	if !m.docView.hasDoc() {
		return m, flashCmd("open a file first — \":\" runs in its shell session")
	}
	m.mode = modeConsole
	m.consoleInput.Reset()
	return m, m.consoleInput.Focus()
}

func (m Model) handleConsoleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		m.consoleInput.Blur()
		return m, nil
	case "enter":
		m.mode = modeBrowse
		m.consoleInput.Blur()
		cmd := strings.TrimSpace(m.consoleInput.Value())
		m.consoleInput.Reset()
		var c tea.Cmd
		m.docView, c = m.docView.runConsole(cmd)
		m.layout() // the console pane just claimed rows from the panels
		return m, c
	}

	var c tea.Cmd
	m.consoleInput, c = m.consoleInput.Update(msg)
	return m, c
}

// scratchCmd suspends the TUI and opens $EDITOR on a temp scratch file
// seeded with the last command. Whatever is saved runs in the document's
// shell session on exit — the multi-line counterpart to the ":" prompt, for
// setup too long or too fiddly to type on one line.
func (d DocumentModel) scratchCmd() tea.Cmd {
	if !d.hasDoc() {
		return flashCmd("open a file first — the scratch buffer runs in its shell session")
	}

	f, err := os.CreateTemp("", "yap-scratch-*.sh")
	if err != nil {
		return flashCmd("could not create scratch file: " + err.Error())
	}
	path := f.Name()

	seed := d.console.cmd
	if seed == "" {
		seed = "# Runs in this document's shell session on save+exit.\n" +
			"# Exports and cd here are visible to every block run afterwards.\n\n"
	} else if !strings.HasSuffix(seed, "\n") {
		seed += "\n"
	}
	_, writeErr := f.WriteString(seed)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		os.Remove(path)
		return flashCmd("could not write scratch file")
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	c := osexec.Command(editor, path)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return scratchEditedMsg{path: path, err: err}
	})
}

// runScratch reads the saved scratch file and runs it in the shell session.
// The temp file is removed either way — its contents live on only as the
// console's recorded command, which reseeds the next scratch edit.
func (d DocumentModel) runScratch(msg scratchEditedMsg) (DocumentModel, tea.Cmd) {
	defer os.Remove(msg.path)

	if msg.err != nil {
		return d, flashCmd("editor exited with error: " + msg.err.Error())
	}
	content, err := os.ReadFile(msg.path)
	if err != nil {
		return d, flashCmd("could not read scratch file: " + err.Error())
	}

	script := strings.TrimSpace(string(content))
	if script == "" || onlyComments(script) {
		return d, flashCmd("scratch buffer empty — nothing run")
	}
	return d.runConsole(script)
}

// onlyComments reports whether a script is nothing but blank lines and
// comments, so quitting the editor without editing the seeded template
// doesn't count as "run this".
func onlyComments(script string) bool {
	for _, line := range strings.Split(script, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			return false
		}
	}
	return true
}

// cancelConsole interrupts an in-flight ":" command via the shared shell's
// process group, leaving the session (and its accumulated env) alive.
func (d *DocumentModel) cancelConsole() {
	if d.shell != nil {
		d.shell.Cancel()
	}
}

// consoleBusy reports whether the ad-hoc command is still in flight, so
// ctrl+c knows to interrupt it rather than quit.
func (d DocumentModel) consoleBusy() bool {
	return d.console.state == stateRunning || d.console.state == stateQueued
}

const (
	// consoleCollapsedLines is the tail-peek height: enough to see a command
	// worked without displacing the document.
	consoleCollapsedLines = 6
	// consoleMinDocRows is the floor the document keeps no matter how much the
	// console asks for.
	consoleMinDocRows = 8
	// consoleExpandedMin is the smallest useful expanded box — two border rows
	// plus a few lines of log. Below this, expanding isn't worth the rows.
	consoleExpandedMin = 6
)

// collapsedLines is how many output lines the tail-peek shows.
func (d DocumentModel) collapsedLines() int {
	if n := len(d.console.output); n < consoleCollapsedLines {
		return n
	}
	return consoleCollapsedLines
}

// consoleHeight is how many terminal rows the console pane occupies, so
// layout can subtract them from the panels rather than letting the pane push
// content past the bottom of the frame.
//
// Expanded claims a fixed share of the terminal rather than shrinking to fit
// its output: a one-line log in a one-line box is indistinguishable from the
// collapsed peek, and the box is scrollable anyway, so a stable size is worth
// more than a snug one.
func (d DocumentModel) consoleHeight(termHeight int) int {
	switch d.console.size {
	case consoleHidden:
		return 0
	case consoleExpanded:
		h := termHeight / 2
		// -2 for the app's own header and footer rows.
		if room := termHeight - 2 - consoleMinDocRows; h > room {
			h = room
		}
		if h < consoleExpandedMin {
			h = consoleExpandedMin
		}
		return h
	default:
		return d.collapsedLines() + 1 // + the ":" command echo line
	}
}

// setConsoleSize sizes the expanded pane's viewport. The +2/-2 are its border.
func (d *DocumentModel) setConsoleSize(w, h int) {
	innerW, innerH := w-2, h-2
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}
	d.console.vp.SetWidth(innerW)
	d.console.vp.SetHeight(innerH)
	d.rebuildConsole()
}

// rebuildConsole refills the expanded pane's viewport with the command and its
// output, and follows the tail while the command is still producing lines —
// but only while the user hasn't scrolled away, so reading back through a long
// log isn't yanked to the bottom by every new line.
func (d *DocumentModel) rebuildConsole() {
	follow := d.console.vp.AtBottom()
	lines := make([]string, 0, len(d.console.output)+2)
	for _, l := range strings.Split(d.console.cmd, "\n") {
		lines = append(lines, "$ "+l)
	}
	lines = append(lines, d.console.output...)
	d.console.vp.SetContent(strings.Join(lines, "\n"))
	if follow {
		d.console.vp.GotoBottom()
	}
}

// toggleConsole cycles the pane's size. It's a no-op before any ":" command
// has run — there'd be nothing to show.
func (d *DocumentModel) toggleConsole() {
	if d.console.cmd == "" {
		return
	}
	d.console.size = d.console.size.next()
}

// consoleExpanded reports whether the pane is the big scrollable box, which is
// the only size that can take focus.
func (d DocumentModel) consoleIsExpanded() bool {
	return d.console.size == consoleExpanded
}

// scrollConsole moves the expanded pane's viewport. Only the expanded box
// scrolls — the collapsed peek is a fixed tail.
func (d *DocumentModel) scrollConsole(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "j", "down":
		d.console.vp.ScrollDown(1)
	case "k", "up":
		d.console.vp.ScrollUp(1)
	case "ctrl+d", "pgdown":
		d.console.vp.HalfPageDown()
	case "ctrl+u", "pgup":
		d.console.vp.HalfPageUp()
	case "g":
		d.console.vp.GotoTop()
	case "G":
		d.console.vp.GotoBottom()
	}
}

// consoleLabel collapses a command to one line for the pane's header — a
// scratch-buffer script can be many lines, but the pane budgets exactly one
// row for the echo.
func consoleLabel(cmd string) string {
	first, _, multi := strings.Cut(cmd, "\n")
	if multi {
		return strings.TrimSpace(first) + " …"
	}
	return cmd
}

// consoleStatus is the console's state as a label and the color to draw it in.
func (d DocumentModel) consoleStatus(th theme.Theme) (string, color.Color) {
	switch d.console.state {
	case stateRunning:
		return "running…", th.StatusRunning
	case stateSuccess:
		return "✓", th.StatusSuccess
	case stateFail:
		if d.console.err != nil {
			return "✗ " + d.console.err.Error(), th.StatusError
		}
		return "✗", th.StatusError
	}
	return "queued…", th.TextMuted
}

// renderConsole draws the session pane beneath the panels: a one-line tail
// peek when collapsed, a bordered scrollable log box when expanded. Empty
// string when hidden.
func (d DocumentModel) renderConsole(th theme.Theme, width, termHeight int, focused bool) string {
	switch d.console.size {
	case consoleHidden:
		return ""
	case consoleExpanded:
		return d.renderConsoleBox(th, width, termHeight, focused)
	}

	status, statusColor := d.consoleStatus(th)
	shown := d.collapsedLines()
	out := d.console.output
	if len(out) > shown {
		out = out[len(out)-shown:]
	}

	prompt := lipgloss.NewStyle().Foreground(th.AccentPrimary).Render(" :" + consoleLabel(d.console.cmd))
	prompt += "  " + lipgloss.NewStyle().Foreground(statusColor).Render(status)
	// Say how much of the log is off-screen, so a truncated tail doesn't read
	// as the whole output, and name the key that reveals the rest.
	meta := fmt.Sprintf("  [ctrl+o %s]", d.console.size.hint())
	switch {
	case len(d.console.output) == 0 && d.console.state != stateQueued:
		// Plenty of useful session commands (export, cd) print nothing. Say so
		// explicitly — otherwise an empty pane reads as a broken one.
		meta = "  no output" + meta
	case len(out) < len(d.console.output):
		meta = fmt.Sprintf("  +%d more%s", len(d.console.output)-len(out), meta)
	}
	prompt += lipgloss.NewStyle().Foreground(th.TextMuted).Render(meta)

	lines := []string{lipgloss.NewStyle().Width(width).Render(prompt)}
	outStyle := lipgloss.NewStyle().Foreground(th.TextMuted).Width(width)
	for _, l := range out {
		lines = append(lines, outStyle.Render("  "+l))
	}
	return strings.Join(lines, "\n")
}

// renderConsoleBox draws the expanded pane as a bordered panel matching the
// Explorer and Document boxes — same border, same focus coloring — so the
// session log reads as a third panel rather than an overlay.
func (d DocumentModel) renderConsoleBox(th theme.Theme, width, termHeight int, focused bool) string {
	border := lipgloss.NormalBorder()
	borderColor := th.BorderPanelIdle
	if focused {
		border = lipgloss.ThickBorder()
		borderColor = th.BorderPanelFocus
	}

	status, statusColor := d.consoleStatus(th)
	title := " Session ── :" + consoleLabel(d.console.cmd) + " "

	style := lipgloss.NewStyle().Border(border).BorderForeground(borderColor)
	innerW := width - 2

	body := d.console.vp.View()
	if len(d.console.output) == 0 && d.console.state != stateQueued {
		body = lipgloss.NewStyle().
			Foreground(th.TextMuted).
			Width(innerW).
			Height(d.console.vp.Height()).
			Render("  no output")
	}

	// The status and the ctrl+o hint ride the top border, the same way a code
	// block's run status does — a separate header row would cost a line the
	// log could use.
	hint := fmt.Sprintf("[ctrl+o %s]", d.console.size.hint())
	top := codeTopBorderLine(border, borderColor, width, title, status, statusColor, hint)
	return top + "\n" + style.BorderTop(false).Render(body)
}

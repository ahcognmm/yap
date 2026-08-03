package ui

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"yap/internal/exec"
	"yap/internal/theme"
)

// consoleState is the ":" ad-hoc command run against the document's shared
// shell — the same process the code blocks use, so `export FOO=bar` typed
// here is visible to every block run afterwards. That's the point: it lets
// you set up secrets, PATH entries, or a virtualenv without editing the file.
type consoleState struct {
	cmd     string
	output  []string
	state   runState
	err     error
	run     *exec.Run
	gen     int
	visible bool // console pane shown below the document
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
			d.console.visible = true
			return d, nil
		}
		d.shell = shell
	}

	d.console.gen++
	d.console.cmd = cmd
	d.console.output = nil
	d.console.err = nil
	d.console.state = stateQueued
	d.console.visible = true

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

// consoleMaxLines caps the console pane so a chatty command can't push the
// document off the screen — older lines scroll out of the tail window.
const consoleMaxLines = 6

// consoleHeight is how many terminal rows the console pane occupies, so
// layout can subtract them from the panels rather than letting the pane push
// content past the bottom of the frame.
func (d DocumentModel) consoleHeight() int {
	if !d.console.visible {
		return 0
	}
	n := len(d.console.output)
	if n > consoleMaxLines {
		n = consoleMaxLines
	}
	return n + 1 // + the ":" command echo line
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

// renderConsole draws the ad-hoc command, its status, and the tail of its
// output beneath the panels. Empty string when no ":" command has been run.
func (d DocumentModel) renderConsole(th theme.Theme, width int) string {
	if !d.console.visible {
		return ""
	}

	statusColor := th.TextMuted
	status := "queued…"
	switch d.console.state {
	case stateRunning:
		statusColor, status = th.StatusRunning, "running…"
	case stateSuccess:
		statusColor, status = th.StatusSuccess, "✓"
	case stateFail:
		statusColor, status = th.StatusError, "✗"
		if d.console.err != nil {
			status = "✗ " + d.console.err.Error()
		}
	}

	prompt := lipgloss.NewStyle().Foreground(th.AccentPrimary).Render(" :" + consoleLabel(d.console.cmd))
	prompt += "  " + lipgloss.NewStyle().Foreground(statusColor).Render(status)

	lines := []string{lipgloss.NewStyle().Width(width).Render(prompt)}
	out := d.console.output
	if len(out) > consoleMaxLines {
		out = out[len(out)-consoleMaxLines:]
	}
	outStyle := lipgloss.NewStyle().Foreground(th.TextMuted).Width(width)
	for _, l := range out {
		lines = append(lines, outStyle.Render("  "+l))
	}
	return strings.Join(lines, "\n")
}

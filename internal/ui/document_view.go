package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/glamour"

	"runmd/internal/markdown"
	"runmd/internal/theme"
)

// render draws the Document panel: prose via Glamour, code blocks in their
// own bordered sub-box with a run indicator and streamed output beneath —
// all clipped and scrolled through d.vp so content taller than the pane
// doesn't overflow the frame.
func (d DocumentModel) render(th theme.Theme, focused bool) string {
	width, height := d.width, d.height

	title := "Document"
	if d.hasDoc() {
		title = pathBase(d.path)
	}
	if len(d.blocks) > 0 {
		title += fmt.Sprintf(" ── %d/%d", d.selected+1, len(d.blocks))
	}

	border := lipgloss.NormalBorder()
	borderColor := th.BorderPanelIdle
	if focused {
		border = lipgloss.ThickBorder()
		borderColor = th.BorderPanelFocus
	}

	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(borderColor)
	fw, fh := style.GetFrameSize()
	innerW, innerH := width-fw, height-fh
	if innerW < 0 {
		innerW = 0
	}
	if innerH < 0 {
		innerH = 0
	}

	var body string
	if !d.hasDoc() {
		body = lipgloss.NewStyle().
			Foreground(th.TextMuted).
			Padding(1, 2).
			Width(innerW).
			Height(innerH).
			Render("Open a file from the Explorer to get started.")
	} else {
		content, offsets := d.renderBlocks(th, innerW)
		vp := d.vp
		vp.SetWidth(innerW)
		vp.SetHeight(innerH)
		vp.SetContent(content)
		if d.selected >= 0 && d.selected < len(offsets) && offsets[d.selected] >= 0 {
			vp.EnsureVisible(offsets[d.selected], 0, 0)
		}
		body = vp.View()
	}

	// body is already sized to exactly innerW x innerH (vp.View() and the
	// placeholder branch both pad internally) — the box style below must
	// NOT also carry Width/Height, since combining Width with Border makes
	// lipgloss re-wrap body at (innerW - border size), which can split an
	// already-full-width line (e.g. a code block's border row) and inject a
	// phantom extra row.
	boxStyle := style.BorderTop(false)
	framed := boxStyle.Render(body)
	top := borderTopLine(border, borderColor, innerW+style.GetHorizontalFrameSize(), " "+title+" ")
	return top + "\n" + framed
}

// renderBlocks joins every block's rendered form and returns, alongside the
// joined content, each block's starting line offset within it (-1 for
// blocks hidden by an active filter) so the caller can scroll a block into
// view.
func (d DocumentModel) renderBlocks(th theme.Theme, width int) (string, []int) {
	var parts []string
	offsets := make([]int, len(d.blocks))
	line := 0
	for i, b := range d.blocks {
		if d.filter != "" && !d.matchesFilter(i) {
			offsets[i] = -1
			continue
		}
		var rendered string
		switch b.Kind {
		case markdown.KindProse:
			rendered = renderProse(b.Raw, width)
		case markdown.KindCode:
			rendered = d.renderCodeBlock(i, b, th, width)
		}
		offsets[i] = line
		parts = append(parts, rendered)
		line += strings.Count(rendered, "\n") + 1
	}
	return strings.Join(parts, "\n"), offsets
}

func renderProse(src string, width int) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return src
	}
	out, err := r.Render(src)
	if err != nil {
		return src
	}
	return strings.TrimRight(out, "\n")
}

func (d DocumentModel) renderCodeBlock(idx int, b markdown.Block, th theme.Theme, width int) string {
	selected := idx == d.selected
	st := d.states[idx]

	border := lipgloss.NormalBorder()
	if selected {
		border = lipgloss.ThickBorder()
	}

	statusColor := th.TextMuted
	statusLabel := ""
	switch st.state {
	case stateQueued:
		statusColor = th.TextMuted
		statusLabel = "queued…"
	case stateRunning:
		statusColor = th.StatusRunning
		statusLabel = fmt.Sprintf("running… %s", formatElapsed(st.elapsed()))
	case stateSuccess:
		statusColor = th.StatusSuccess
		statusLabel = fmt.Sprintf("✓ %s", formatElapsed(st.elapsed()))
	case stateFail:
		statusColor = th.StatusError
		statusLabel = fmt.Sprintf("✗ exit %d (%s)", st.exitCode, formatElapsed(st.elapsed()))
	}

	action := "[ ▶ Run ]"
	if !b.Runnable {
		action = ""
	}

	label := " " + b.Lang + " "

	boxWidth := width
	if boxWidth < 4 {
		boxWidth = 4
	}

	style := lipgloss.NewStyle().
		Border(border).
		BorderForeground(borderColorFor(selected, th))
	fw, _ := style.GetFrameSize()
	contentWidth := boxWidth - fw
	if contentWidth < 0 {
		contentWidth = 0
	}

	// code is pre-sized to exactly contentWidth; bodyStyle must not also
	// carry Width, or combining Width with Border re-wraps code at
	// (contentWidth - border size) and can split an already-full-width line.
	code := lipgloss.NewStyle().Padding(0, 1).Width(contentWidth).Render(b.Raw)
	bodyStyle := style.BorderTop(false)
	box := bodyStyle.Render(code)

	topColor := borderColorFor(selected, th)
	top := codeTopBorderLine(border, topColor, boxWidth, label, statusLabel, statusColor, action)
	box = top + "\n" + box

	if len(st.output) > 0 {
		out := strings.Join(st.output, "\n")
		outBox := lipgloss.NewStyle().
			Foreground(th.TextMuted).
			Padding(0, 2).
			Width(boxWidth).
			Render(out)
		box = box + "\n" + outBox
	}

	return box
}

func borderColorFor(selected bool, th theme.Theme) color.Color {
	if selected {
		return th.AccentPrimary
	}
	return th.TextMuted
}

// codeTopBorderLine draws a code block's top border with the fence label
// woven in near the left corner and the status+run affordance woven in
// near the right corner. Built entirely from independently styled
// fragments — never by slicing an already-rendered ANSI string — so
// escape sequences can never be cut mid-code.
func codeTopBorderLine(b lipgloss.Border, borderColor color.Color, totalWidth int, label, statusLabel string, statusColor color.Color, action string) string {
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)
	labelStyle := lipgloss.NewStyle().Foreground(borderColor)

	var right strings.Builder
	rightW := 0
	if statusLabel != "" {
		right.WriteString(lipgloss.NewStyle().Foreground(statusColor).Render(statusLabel))
		right.WriteString("  ")
		rightW += lipgloss.Width(statusLabel) + 2
	}
	right.WriteString(action)
	rightW += lipgloss.Width(action)

	labelW := lipgloss.Width(label)
	afterCorner := totalWidth - 2
	if afterCorner < 0 {
		afterCorner = 0
	}
	lead := 1
	if lead > afterCorner {
		lead = afterCorner
	}
	trail := 1
	if trail > afterCorner {
		trail = afterCorner
	}
	gap := afterCorner - lead - labelW - trail - rightW
	if gap < 0 {
		gap = 0
	}

	var sb strings.Builder
	sb.WriteString(borderStyle.Render(b.TopLeft))
	sb.WriteString(borderStyle.Render(strings.Repeat(b.Top, lead)))
	sb.WriteString(labelStyle.Render(label))
	sb.WriteString(borderStyle.Render(strings.Repeat(b.Top, gap)))
	sb.WriteString(right.String())
	sb.WriteString(borderStyle.Render(strings.Repeat(b.Top, trail)))
	sb.WriteString(borderStyle.Render(b.TopRight))
	return sb.String()
}

// formatElapsed renders a run duration for the code block status label —
// sub-second precision while short, whole seconds once it's a while.
func formatElapsed(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d < time.Second {
		return d.Round(10 * time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}

func pathBase(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i < 0 {
		return p
	}
	return p[i+1:]
}

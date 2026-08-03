package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"runmd/internal/fstree"
	"runmd/internal/theme"
)

// render draws the Explorer panel, sized per the last setSize call — the
// same box serves both the fixed edge-to-edge panel (§2.1/§2.2) and the
// narrow-mode floating overlay (§2.4); only the frame it's placed into
// differs.
func (e ExplorerModel) render(th theme.Theme, focused bool) string {
	width, height := e.width, e.height
	title := "Explorer"

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

	blankRow := strings.Repeat(" ", innerW)
	rows := make([]string, 0, innerH)
	for i := 0; i < innerH; i++ {
		idx := e.scroll + i
		if idx >= len(e.flat) {
			rows = append(rows, blankRow)
			continue
		}
		rows = append(rows, e.renderRow(e.flat[idx], idx == e.cursor, th, innerW))
	}
	body := strings.Join(rows, "\n")

	// Draw the body with only the sides/bottom border, then prepend a
	// hand-built top border with the title woven in — building the top
	// border from raw pieces (not slicing already-rendered ANSI) avoids
	// cutting escape sequences in half. body is already exactly innerW x
	// innerH (every row rendered/padded to innerW above), so boxStyle must
	// NOT also carry Width/Height — combining Width with Border would
	// re-wrap body at (innerW - border size) and could split a full-width
	// row, injecting a phantom extra line.
	boxStyle := style.BorderTop(false)
	framed := boxStyle.Render(body)
	top := borderTopLine(border, borderColor, innerW+style.GetHorizontalFrameSize(), " "+title+" ")
	return top + "\n" + framed
}

// borderTopLine draws a top border line of the given total width with a
// styled title woven in near the left corner. Built from independently
// styled fragments (never by slicing an already-rendered ANSI string) so
// escape sequences are never cut mid-code.
func borderTopLine(b lipgloss.Border, c color.Color, totalWidth int, title string) string {
	borderStyle := lipgloss.NewStyle().Foreground(c)
	titleStyle := lipgloss.NewStyle().Foreground(c).Bold(true)

	corner := b.TopLeft
	fill := b.Top
	titleW := lipgloss.Width(title)
	afterCorner := totalWidth - 2 // width available after both corners
	if afterCorner < 0 {
		afterCorner = 0
	}
	lead := 1
	if lead > afterCorner {
		lead = afterCorner
	}
	trail := afterCorner - lead - titleW
	if trail < 0 {
		trail = 0
		titleW = afterCorner - lead
		if titleW < 0 {
			titleW = 0
		}
		r := []rune(title)
		if len(r) > titleW {
			r = r[:titleW]
		}
		title = string(r)
	}

	var sb strings.Builder
	sb.WriteString(borderStyle.Render(corner))
	sb.WriteString(borderStyle.Render(strings.Repeat(fill, lead)))
	sb.WriteString(titleStyle.Render(title))
	sb.WriteString(borderStyle.Render(strings.Repeat(fill, trail)))
	sb.WriteString(borderStyle.Render(b.TopRight))
	return sb.String()
}

// renderRow draws one Explorer row: an open-file marker, indent, a chevron
// for directories, and the name — semantic color only, no emoji. The whole
// row is styled once, so the cursor row just adds Reverse on top.
func (e ExplorerModel) renderRow(row fstree.FlatRow, cursor bool, th theme.Theme, width int) string {
	n := row.Node
	indent := strings.Repeat("  ", row.Depth)

	var icon, name string
	fg := th.TreeFile
	bold := false
	switch {
	case n.IsDir:
		arrow := "▸"
		if n.Expanded {
			arrow = "▾"
		}
		icon = arrow + " "
		name = n.Name + "/"
		fg = th.TreeFolder
		bold = true
	case n.IsMD:
		icon = "  "
		name = n.Name
		fg = th.TreeMarkdown
	default:
		icon = "  "
		name = n.Name
	}

	marker := " "
	if n.Path == e.openPath {
		marker = "●"
	}

	style := lipgloss.NewStyle().Foreground(fg).Bold(bold)
	if cursor {
		// Reverse the whole row as one block so cursor + marker read as a
		// single unit, rather than layering a second recolor pass on top.
		style = style.Reverse(true)
	}

	line := marker + " " + indent + icon + name
	return style.MaxWidth(width).Width(width).Render(line)
}

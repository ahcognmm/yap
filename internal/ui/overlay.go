package ui

import "charm.land/lipgloss/v2"

// renderOverlay draws box centered on top of base using Lip Gloss's layer
// compositor — the same helper backs both overlay-class surfaces (the `?`
// help screen and the narrow-mode Explorer overlay) so they stay visually
// consistent, per DESIGN.md §2.4.
func renderOverlay(base, box string, width, height int) string {
	boxW := lipgloss.Width(box)
	boxH := lipgloss.Height(box)

	x := (width - boxW) / 2
	y := (height - boxH) / 2
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}

	baseLayer := lipgloss.NewLayer(base).X(0).Y(0).Z(0)
	boxLayer := lipgloss.NewLayer(box).X(x).Y(y).Z(1)
	return lipgloss.NewCanvas(width, height).Compose(
		lipgloss.NewCompositor(baseLayer, boxLayer),
	).Render()
}

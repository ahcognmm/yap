// Package theme maps semantic tokens to colors so the UI never scatters
// hex codes through view code.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds every semantic color token used by the UI.
type Theme struct {
	Name string

	TextPrimary color.Color
	TextMuted   color.Color

	AccentPrimary color.Color

	StatusSuccess color.Color
	StatusError   color.Color
	StatusRunning color.Color

	TreeFolder     color.Color
	TreeMarkdown   color.Color
	TreeFile       color.Color
	TreeOpenMarker color.Color

	BorderPanelFocus color.Color
	BorderPanelIdle  color.Color
}

// Default is the bundled dark theme.
func Default() Theme {
	return Theme{
		Name: "default-dark",

		TextPrimary: lipgloss.Color("#E6E6E6"),
		TextMuted:   lipgloss.Color("#6C7086"),

		AccentPrimary: lipgloss.Color("#89B4FA"),

		StatusSuccess: lipgloss.Color("#A6E3A1"),
		StatusError:   lipgloss.Color("#F38BA8"),
		StatusRunning: lipgloss.Color("#F9E2AF"),

		TreeFolder:     lipgloss.Color("#E6E6E6"),
		TreeMarkdown:   lipgloss.Color("#74A8F0"),
		TreeFile:       lipgloss.Color("#6C7086"),
		TreeOpenMarker: lipgloss.Color("#94E2D5"),

		BorderPanelFocus: lipgloss.Color("#89B4FA"),
		BorderPanelIdle:  lipgloss.Color("#6C7086"),
	}
}

// ByName resolves a bundled theme by name, falling back to Default.
func ByName(name string) Theme {
	switch name {
	case "catppuccin", "catppuccin-mocha":
		return Catppuccin()
	default:
		return Default()
	}
}

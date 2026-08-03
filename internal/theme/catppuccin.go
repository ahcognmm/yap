package theme

import "charm.land/lipgloss/v2"

// Catppuccin returns the bundled Catppuccin Mocha theme.
func Catppuccin() Theme {
	return Theme{
		Name: "catppuccin-mocha",

		TextPrimary: lipgloss.Color("#CDD6F4"),
		TextMuted:   lipgloss.Color("#7F849C"),

		AccentPrimary: lipgloss.Color("#CBA6F7"),

		StatusSuccess: lipgloss.Color("#A6E3A1"),
		StatusError:   lipgloss.Color("#F38BA8"),
		StatusRunning: lipgloss.Color("#F9E2AF"),

		TreeFolder:     lipgloss.Color("#CDD6F4"),
		TreeMarkdown:   lipgloss.Color("#B4BEFE"),
		TreeFile:       lipgloss.Color("#7F849C"),
		TreeOpenMarker: lipgloss.Color("#94E2D5"),

		BorderPanelFocus: lipgloss.Color("#CBA6F7"),
		BorderPanelIdle:  lipgloss.Color("#585B70"),
	}
}

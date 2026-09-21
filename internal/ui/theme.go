package ui

import "github.com/charmbracelet/lipgloss"

// Jellyfin's brand gradient runs purple → blue; jellytop borrows both ends as
// accents. Every colour is adaptive so the TUI stays legible on light and dark
// terminals alike.
var (
	colAccent  = lipgloss.AdaptiveColor{Light: "#7B3FA0", Dark: "#AA5CC3"}
	colAccent2 = lipgloss.AdaptiveColor{Light: "#00749B", Dark: "#00A4DC"}
	colText    = lipgloss.AdaptiveColor{Light: "#1F2328", Dark: "#E6E6E6"}
	colMuted   = lipgloss.AdaptiveColor{Light: "#6A737D", Dark: "#8B8B8B"}
	colFaint   = lipgloss.AdaptiveColor{Light: "#9AA0A6", Dark: "#5C5C5C"}
	colOK      = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#3FB950"}
	colWarn    = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#D29922"}
	colErr     = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#F85149"}
	colSelBg   = lipgloss.AdaptiveColor{Light: "#E8E0F0", Dark: "#33254A"}
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent)
	styleMuted = lipgloss.NewStyle().Foreground(colMuted)
	styleFaint = lipgloss.NewStyle().Foreground(colFaint)
	styleText  = lipgloss.NewStyle().Foreground(colText)
	styleOK    = lipgloss.NewStyle().Foreground(colOK)
	styleWarn  = lipgloss.NewStyle().Foreground(colWarn)
	styleErr   = lipgloss.NewStyle().Foreground(colErr)
	styleKey   = lipgloss.NewStyle().Bold(true).Foreground(colAccent2)

	styleTabActive = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).
			Background(colAccent).Padding(0, 1)
	styleTabInactive = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)

	styleHeaderRow = lipgloss.NewStyle().Bold(true).Foreground(colMuted)
	styleSelected  = lipgloss.NewStyle().Background(colSelBg).Foreground(colText)

	styleStatusBar = lipgloss.NewStyle().Foreground(colMuted)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colAccent).
			Padding(1, 3)
	styleModalDanger = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colErr).
				Padding(1, 3)
)

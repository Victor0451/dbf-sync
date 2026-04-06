package ui

import "charm.land/lipgloss/v2"

// Colors
var (
	Primary   = lipgloss.Color("#00D4AA")  // cyan-green
	Secondary = lipgloss.Color("#FF6B6B")  // red
	Accent    = lipgloss.Color("#4ECDC4")  // teal
	Muted     = lipgloss.Color("#666666")  // gray
	White     = lipgloss.Color("#FFFFFF")  // white
	DarkBg    = lipgloss.Color("#1a1a2e")  // dark background
	PanelBg   = lipgloss.Color("#16213e")  // panel background
)

// Styles
var (
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary).
			Align(lipgloss.Center)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(Accent).
			Align(lipgloss.Center)

	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Accent).
			PaddingLeft(2)

	NormalStyle = lipgloss.NewStyle().
			Foreground(White).
			PaddingLeft(4)

	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Secondary)

	SuccessStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)

	HintStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2)

	PanelStyle = lipgloss.NewStyle().
			Background(PanelBg).
			Foreground(White).
			Padding(1, 2)

	InputStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Background(DarkBg)

	CursorStyle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true)
)

// List item styles
func GetItemStyle(selected bool) lipgloss.Style {
	if selected {
		return SelectedStyle
	}
	return NormalStyle
}

// Status message styles
var (
	StatusOKStyle     = SuccessStyle
	StatusErrorStyle  = ErrorStyle
	StatusInfoStyle   = lipgloss.NewStyle().Foreground(Accent)
	StatusWarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
)

// Box styles for summary display
var (
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1)

	BoxTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(Primary)
)

// Progress bar style
var ProgressStyle = lipgloss.NewStyle().Foreground(Accent)

// Progress bar specific styles
var (
	ProgressBarFillStyle = lipgloss.NewStyle().Foreground(Primary) // "█" characters
	ProgressBarEmptyStyle = lipgloss.NewStyle().Foreground(Muted)  // "░" characters
	ProgressETAStyle = lipgloss.NewStyle().Foreground(Accent)      // ETA text
	ProgressSpeedStyle = lipgloss.NewStyle().Foreground(Primary)   // Speed text "rec/s"
	ProgressErrorBannerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#FF6B6B")) // Red for error count
)
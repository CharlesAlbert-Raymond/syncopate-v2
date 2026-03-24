package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorPrimary   = lipgloss.Color("#7C3AED") // violet
	colorSecondary = lipgloss.Color("#06B6D4") // cyan
	colorSuccess   = lipgloss.Color("#10B981") // emerald
	colorWarning   = lipgloss.Color("#F59E0B") // amber
	colorDanger    = lipgloss.Color("#EF4444") // red
	colorMuted     = lipgloss.Color("#6B7280") // gray
	colorText      = lipgloss.Color("#E5E7EB") // light gray
	colorBg        = lipgloss.Color("#1F2937") // dark bg
	colorBgAlt     = lipgloss.Color("#374151") // slightly lighter

	// Styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Italic(true)

	selectedRowStyle = lipgloss.NewStyle().
				Background(colorBgAlt).
				Foreground(colorText).
				Bold(true)

	normalRowStyle = lipgloss.NewStyle().
			Foreground(colorText)

	branchStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	mainBranchStyle = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true)

	sessionActiveStyle = lipgloss.NewStyle().
				Foreground(colorSuccess)

	sessionInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	sessionAttachedStyle = lipgloss.NewStyle().
				Foreground(colorWarning)

	pathStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorDanger).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	inputLabelStyle = lipgloss.NewStyle().
			Foreground(colorSecondary).
			Bold(true)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	dialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarning).
			Padding(1, 2).
			Width(50)
)

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
	colorBg         = lipgloss.Color("#1F2937") // dark bg
	colorBgAlt      = lipgloss.Color("#374151") // slightly lighter
	colorWarningDim = lipgloss.Color("#92400E") // dark amber — current + selected

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

	portStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F472B6")) // pink — stands out from other indicators

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

	// Sidebar box styles
	sidebarBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(0, 1)

	sidebarHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary)

	currentMarkerStyle = lipgloss.NewStyle().
				Foreground(colorWarning).
				Bold(true)

	helpBoxStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorText).
			Background(colorPrimary).
			Padding(0, 1)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Background(colorBgAlt).
				Padding(0, 1)

	// ASCII art logos
	logoClassic = "" +
		" ___ _   _ _ __   ___ ___\n" +
		"/ __| | | | '_ \\ / __/ _ \\\n" +
		"\\__ \\ |_| | | | | (_| (_) |\n" +
		"|___/\\__, |_| |_|\\___\\___/\n" +
		"     |___/"

	logoSidebar = "" +
		" ____  _ _ _  __ ___\n" +
		"(_-< || | ' \\/ _/ _ \\\n" +
		"/__/\\_, |_||_\\__\\___/\n" +
		"    |__/"
)

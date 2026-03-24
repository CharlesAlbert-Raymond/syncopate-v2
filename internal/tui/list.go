package tui

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	"github.com/charles-albert-raymond/syncopate/internal/state"
	"github.com/charles-albert-raymond/syncopate/internal/tmux"
)

// tmuxExecCmd wraps exec.Cmd to implement tea.ExecCommand.
type tmuxExecCmd struct {
	cmd *exec.Cmd
}

func (t *tmuxExecCmd) Run() error         { return t.cmd.Run() }
func (t *tmuxExecCmd) SetStdin(r io.Reader)  { t.cmd.Stdin = r }
func (t *tmuxExecCmd) SetStdout(w io.Writer) { t.cmd.Stdout = w }
func (t *tmuxExecCmd) SetStderr(w io.Writer) { t.cmd.Stderr = w }

type listModel struct {
	entries       []state.Entry
	otherPorts    []state.SessionPorts // non-syncopate sessions with ports
	cursor        int
	cursorInitd   bool // true after cursor has been placed on current worktree
	width         int
	height        int
	message       string
	msgStyle      lipgloss.Style
	config        config.Config
}

func newListModel() listModel {
	return listModel{}
}

type entriesMsg struct {
	entries    []state.Entry
	otherPorts []state.SessionPorts
}
type attachMsg struct{ session string }

func fetchEntries(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		result, err := state.Gather(repoRoot)
		if err != nil {
			return errMsg{err}
		}
		return entriesMsg{entries: result.Entries, otherPorts: result.OtherPorts}
	}
}

func (m listModel) Update(msg tea.Msg, repoRoot string) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesMsg:
		m.entries = msg.entries
		m.otherPorts = msg.otherPorts
		if m.cursor >= len(m.entries) {
			m.cursor = max(0, len(m.entries)-1)
		}
		return m, nil

	case attachMsg:
		m.message = fmt.Sprintf("Detached from %s", msg.session)
		m.msgStyle = successStyle
		return m, fetchEntries(repoRoot)

	case tea.KeyMsg:
		switch {
		case msg.String() == "j" || msg.String() == "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case msg.String() == "k" || msg.String() == "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case msg.String() == "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			entry := m.entries[m.cursor]
			sessionName := entry.SessionName

			// Create session if it doesn't exist
			if !entry.HasSession {
				if err := tmux.NewSession(sessionName, entry.Worktree.Path); err != nil {
					m.message = fmt.Sprintf("Error creating session: %v", err)
					m.msgStyle = errorStyle
					return m, nil
				}
			}

			if tmux.IsInsideTmux() {
				if err := tmux.SwitchClient(sessionName); err != nil {
					m.message = fmt.Sprintf("Error switching: %v", err)
					m.msgStyle = errorStyle
					return m, nil
				}
				m.message = fmt.Sprintf("Switched to %s", sessionName)
				m.msgStyle = successStyle
				return m, fetchEntries(repoRoot)
			}

			// Outside tmux: use tea.Exec to attach
			c := &tmuxExecCmd{cmd: exec.Command("tmux", "attach-session", "-t", sessionName)}
			return m, tea.Exec(c, func(err error) tea.Msg {
				if err != nil {
					return errMsg{err}
				}
				return attachMsg{session: sessionName}
			})
		}
	}
	return m, nil
}

// UpdateSidebar handles input in sidebar mode.
// On enter: ensure the worktree session has a sidebar, then switch-client to it.
func (m listModel) UpdateSidebar(msg tea.Msg, repoRoot string) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesMsg:
		m.entries = msg.entries
		m.otherPorts = msg.otherPorts
		if !m.cursorInitd {
			// On first load, place cursor on the current worktree
			for i, e := range m.entries {
				if e.IsCurrent {
					m.cursor = i
					break
				}
			}
			m.cursorInitd = true
		} else if m.cursor >= len(m.entries) {
			m.cursor = max(0, len(m.entries)-1)
		}
		return m, nil

	case attachMsg:
		m.message = fmt.Sprintf("Switched to %s", msg.session)
		m.msgStyle = successStyle
		return m, fetchEntries(repoRoot)

	case tea.KeyMsg:
		switch {
		case msg.String() == "j" || msg.String() == "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case msg.String() == "k" || msg.String() == "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case msg.String() == "enter":
			if len(m.entries) == 0 {
				return m, nil
			}
			entry := m.entries[m.cursor]
			sessionName := entry.SessionName

			// Create the worktree's tmux session if it doesn't exist
			if !entry.HasSession {
				if err := tmux.NewSession(sessionName, entry.Worktree.Path); err != nil {
					m.message = fmt.Sprintf("Error creating session: %v", err)
					m.msgStyle = errorStyle
					return m, nil
				}
			}

			// Ensure the target session has a sidebar pane
			if err := tmux.EnsureSidebar(sessionName, repoRoot); err != nil {
				m.message = fmt.Sprintf("Error adding sidebar: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			// Switch the tmux client to the worktree's session
			if err := tmux.SwitchClient(sessionName); err != nil {
				m.message = fmt.Sprintf("Error switching: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			// Focus the main work pane (not the sidebar) in the target session
			tmux.FocusMainPane(sessionName)

			m.message = fmt.Sprintf("→ %s", entry.BranchShort)
			m.msgStyle = successStyle
			return m, fetchEntries(repoRoot)
		}
	}
	return m, nil
}

// ViewCompact renders a narrow sidebar-friendly view with per-worktree cards.
func (m listModel) ViewCompact(width int) string {
	if width == 0 {
		width = 28
	}
	boxWidth := width - 2 // leave 1 char margin each side

	var parts []string

	// Header
	parts = append(parts, sidebarHeaderStyle.Render(" syncopate"))

	if len(m.entries) == 0 {
		card := sidebarBoxStyle.
			Width(boxWidth).
			BorderForeground(colorMuted).
			Render(subtitleStyle.Render("No worktrees."))
		parts = append(parts, card)
		parts = append(parts, helpBoxStyle.Render(
			lipgloss.NewStyle().Foreground(colorMuted).Render("c create • q quit"),
		))
		return strings.Join(parts, "\n")
	}

	// Inner width inside card: boxWidth - border(2) - padding(2) = boxWidth - 4
	innerWidth := boxWidth - 4

	// Render a card for each worktree
	for i, entry := range m.entries {
		// Resolve display name: alias > branch
		alias := m.config.AliasFor(entry.BranchShort)
		displayName := alias
		if displayName == "" {
			displayName = entry.BranchShort
		}
		if entry.Worktree.IsMain {
			displayName += " ★"
		}

		// Session status dot
		var dot string
		if entry.HasSession {
			if entry.TmuxSession.Attached {
				dot = sessionAttachedStyle.Render("●")
			} else {
				dot = sessionActiveStyle.Render("●")
			}
		} else {
			dot = sessionInactiveStyle.Render("○")
		}

		// Card border color based on state
		borderColor := colorMuted
		isSelected := i == m.cursor
		if entry.IsCurrent && isSelected {
			borderColor = colorWarningDim
		} else if entry.IsCurrent {
			borderColor = colorWarning
		} else if isSelected {
			borderColor = colorSecondary
		}

		// Build card content
		var lines []string

		// Line 1: display name + status dot
		nameStyle := branchStyle
		if entry.Worktree.IsMain {
			nameStyle = mainBranchStyle
		}
		if entry.IsCurrent && isSelected {
			nameStyle = lipgloss.NewStyle().Foreground(colorWarningDim).Bold(true)
		} else if entry.IsCurrent {
			nameStyle = currentMarkerStyle
		}

		name := truncate(displayName, innerWidth-2) // leave room for dot
		nameLine := nameStyle.Render(name)
		// Right-align the dot
		nameLen := lipgloss.Width(nameLine)
		gap := innerWidth - nameLen - 1
		if gap < 1 {
			gap = 1
		}
		nameLine = nameLine + strings.Repeat(" ", gap) + dot
		lines = append(lines, nameLine)

		// Line 2: show branch name when an alias is set (so you can see the real branch)
		if alias != "" {
			branchLine := pathStyle.Render(truncate(entry.BranchShort, innerWidth))
			lines = append(lines, branchLine)
		}

		// Show listening ports
		if len(entry.Ports) > 0 {
			portsStr := formatPorts(entry.Ports)
			lines = append(lines, portStyle.Render(truncate(portsStr, innerWidth)))
		}

		// Current worktree marker
		if entry.IsCurrent {
			lines = append(lines, currentMarkerStyle.Render("▶ current"))
		}

		content := strings.Join(lines, "\n")

		card := sidebarBoxStyle.
			Width(boxWidth).
			BorderForeground(borderColor).
			Render(content)

		parts = append(parts, card)
	}

	// Other tmux sessions with ports
	if len(m.otherPorts) > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(colorMuted).Render(" other sessions"))
		for _, sp := range m.otherPorts {
			name := truncate(sp.Name, innerWidth-2)
			portsStr := formatPorts(sp.Ports)
			otherContent := lipgloss.NewStyle().Foreground(colorMuted).Render(name) + "\n" +
				portStyle.Render(truncate(portsStr, innerWidth))
			otherCard := sidebarBoxStyle.
				Width(boxWidth).
				BorderForeground(colorMuted).
				Render(otherContent)
			parts = append(parts, otherCard)
		}
	}

	// Status message
	if m.message != "" {
		parts = append(parts, helpBoxStyle.Render(
			m.msgStyle.Render(truncate(m.message, innerWidth)),
		))
	}

	// Help footer
	parts = append(parts, helpBoxStyle.Render(
		lipgloss.NewStyle().Foreground(colorMuted).Render("↵ open · c new · d del")+
			"\n"+
			lipgloss.NewStyle().Foreground(colorMuted).Render("? cfg  · q quit"),
	))

	return strings.Join(parts, "\n")
}

func (m listModel) View() string {
	if len(m.entries) == 0 {
		return titleStyle.Render("  syncopate") + "\n" +
			subtitleStyle.Render("  git worktree orchestrator") + "\n\n" +
			subtitleStyle.Render("  No worktrees found. Press 'c' to create one.") + "\n\n" +
			m.renderHelp()
	}

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render("  syncopate"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("  git worktree orchestrator"))
	b.WriteString("\n\n")

	// Column header
	header := fmt.Sprintf("  %-3s %-25s %-14s %-16s %s", "", "BRANCH", "SESSION", "PORTS", "PATH")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render(header))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  " + strings.Repeat("─", 85)))
	b.WriteString("\n")

	// Entries
	for i, entry := range m.entries {
		cursor := "   "
		if i == m.cursor {
			cursor = " ▸ "
		}

		// Branch name
		branch := entry.BranchShort
		bStyle := branchStyle
		if entry.Worktree.IsMain {
			bStyle = mainBranchStyle
			branch += " ★"
		}
		branchStr := bStyle.Render(fmt.Sprintf("%-25s", truncate(branch, 25)))

		// Session status
		var sessStr string
		if entry.HasSession {
			if entry.TmuxSession.Attached {
				sessStr = sessionAttachedStyle.Render("● attached    ")
			} else {
				sessStr = sessionActiveStyle.Render("● active      ")
			}
		} else {
			sessStr = sessionInactiveStyle.Render("○ none        ")
		}

		// Ports
		var portsStr string
		if len(entry.Ports) > 0 {
			portsStr = portStyle.Render(fmt.Sprintf("%-16s", truncate(formatPorts(entry.Ports), 16)))
		} else {
			portsStr = lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("%-16s", "–"))
		}

		// Path (shortened)
		shortPath := shortenPath(entry.Worktree.Path)
		pathStr := pathStyle.Render(shortPath)

		row := fmt.Sprintf("%s%s %s %s %s", cursor, branchStr, sessStr, portsStr, pathStr)

		if i == m.cursor {
			b.WriteString(selectedRowStyle.Render(row))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")
	}

	// Status message
	if m.message != "" {
		b.WriteString("\n")
		b.WriteString("  " + m.msgStyle.Render(m.message))
	}

	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

func (m listModel) renderHelp() string {
	help := "  enter attach • c create • d delete • r refresh • ? config • q quit"
	return helpStyle.Render(help)
}

func shortenPath(p string) string {
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	parent := filepath.Base(dir)
	return filepath.Join("…", parent, base)
}

func formatPorts(ports []int) string {
	parts := make([]string, len(ports))
	for i, p := range ports {
		parts[i] = ":" + strconv.Itoa(p)
	}
	return strings.Join(parts, " ")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

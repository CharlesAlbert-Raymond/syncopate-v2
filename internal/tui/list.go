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
	entries    []state.Entry
	otherPorts []state.SessionPorts // non-syncopate sessions with ports
	cursor     int
	width      int
	height     int
	message    string
	msgStyle   lipgloss.Style
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
		if m.cursor >= len(m.entries) {
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

			m.message = fmt.Sprintf("→ %s", entry.BranchShort)
			m.msgStyle = successStyle
			return m, fetchEntries(repoRoot)
		}
	}
	return m, nil
}

// ViewCompact renders a narrow sidebar-friendly view.
func (m listModel) ViewCompact(width int) string {
	if width == 0 {
		width = 28
	}
	innerWidth := width - 2 // padding

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render(" syncopate"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render(" " + strings.Repeat("─", innerWidth-1)))
	b.WriteString("\n")

	if len(m.entries) == 0 {
		b.WriteString(subtitleStyle.Render(" No worktrees."))
		b.WriteString("\n")
		b.WriteString(helpStyle.Render(" c create • q quit"))
		return b.String()
	}

	// Entries — compact: cursor + status + branch name
	maxBranch := innerWidth - 5 // "▸ ● " = 4 chars + space
	for i, entry := range m.entries {
		cursor := " "
		if i == m.cursor {
			cursor = "▸"
		}

		// Session status indicator
		var indicator string
		if entry.HasSession {
			if entry.TmuxSession.Attached {
				indicator = sessionAttachedStyle.Render("●")
			} else {
				indicator = sessionActiveStyle.Render("●")
			}
		} else {
			indicator = sessionInactiveStyle.Render("○")
		}

		// Branch name
		branch := entry.BranchShort
		if entry.Worktree.IsMain {
			branch += " ★"
		}
		branch = truncate(branch, maxBranch)

		bStyle := branchStyle
		if entry.Worktree.IsMain {
			bStyle = mainBranchStyle
		}

		row := fmt.Sprintf(" %s %s %s", cursor, indicator, bStyle.Render(branch))

		if i == m.cursor {
			b.WriteString(selectedRowStyle.Render(
				lipgloss.NewStyle().Width(width).Render(row),
			))
		} else {
			b.WriteString(row)
		}
		b.WriteString("\n")

		// Show listening ports on a sub-line
		if len(entry.Ports) > 0 {
			portsStr := formatPorts(entry.Ports)
			portsLine := "     " + portStyle.Render(truncate(portsStr, innerWidth-5))
			b.WriteString(portsLine)
			b.WriteString("\n")
		}
	}

	// Other tmux sessions with ports
	if len(m.otherPorts) > 0 {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render(" " + strings.Repeat("─", innerWidth-1)))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render(" other sessions"))
		b.WriteString("\n")
		for _, sp := range m.otherPorts {
			name := truncate(sp.Name, innerWidth-5)
			b.WriteString("   " + lipgloss.NewStyle().Foreground(colorMuted).Render(name))
			b.WriteString("\n")
			portsStr := formatPorts(sp.Ports)
			b.WriteString("     " + portStyle.Render(truncate(portsStr, innerWidth-5)))
			b.WriteString("\n")
		}
	}

	// Status message
	if m.message != "" {
		b.WriteString("\n")
		b.WriteString(" " + m.msgStyle.Render(truncate(m.message, innerWidth)))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" ↵ open • c new • d del"))
	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" ? cfg  • q quit"))

	return b.String()
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

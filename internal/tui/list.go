package tui

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
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
	entries  []state.Entry
	cursor   int
	width    int
	height   int
	message  string
	msgStyle lipgloss.Style
}

func newListModel() listModel {
	return listModel{}
}

type entriesMsg []state.Entry
type attachMsg struct{ session string }

func fetchEntries(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		entries, err := state.Gather(repoRoot)
		if err != nil {
			return errMsg{err}
		}
		return entriesMsg(entries)
	}
}

func (m listModel) Update(msg tea.Msg, repoRoot string) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesMsg:
		m.entries = []state.Entry(msg)
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
	header := fmt.Sprintf("  %-3s %-25s %-14s %s", "", "BRANCH", "SESSION", "PATH")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render(header))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  " + strings.Repeat("─", 70)))
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

		// Path (shortened)
		shortPath := shortenPath(entry.Worktree.Path)
		pathStr := pathStyle.Render(shortPath)

		row := fmt.Sprintf("%s%s %s %s", cursor, branchStr, sessStr, pathStr)

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
	help := "  enter attach • c create • d delete • r refresh • q quit"
	return helpStyle.Render(help)
}

func shortenPath(p string) string {
	dir := filepath.Dir(p)
	base := filepath.Base(p)
	parent := filepath.Base(dir)
	return filepath.Join("…", parent, base)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}

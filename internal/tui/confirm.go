package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	"github.com/charles-albert-raymond/syncopate/internal/state"
	"github.com/charles-albert-raymond/syncopate/internal/tmux"
	"github.com/charles-albert-raymond/syncopate/internal/worktree"
)

type confirmModel struct {
	entry    state.Entry
	repoRoot string
	config   config.Config
	err      string
}

type deleteDoneMsg struct{}

func newConfirmModel(entry state.Entry, repoRoot string, cfg config.Config) confirmModel {
	return confirmModel{
		entry:    entry,
		repoRoot: repoRoot,
		config:   cfg,
	}
}

func (m confirmModel) Update(msg tea.Msg) (confirmModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y":
			// Run on_destroy hook before tearing down
			if err := config.RunHook(
				m.config.OnDestroy,
				m.entry.BranchShort,
				m.entry.Worktree.Path,
			); err != nil {
				m.err = fmt.Sprintf("on_destroy hook failed: %v", err)
				return m, nil
			}

			// Kill tmux session if it exists
			if m.entry.HasSession {
				if err := tmux.KillSession(m.entry.SessionName); err != nil {
					m.err = fmt.Sprintf("Failed to kill session: %v", err)
					return m, nil
				}
			}

			// Remove worktree
			if err := worktree.Remove(m.repoRoot, m.entry.Worktree.Path); err != nil {
				m.err = fmt.Sprintf("Failed to remove worktree: %v", err)
				return m, nil
			}

			return m, func() tea.Msg { return deleteDoneMsg{} }

		case "n", "N", "esc":
			return m, nil
		}
	}
	return m, nil
}

func (m confirmModel) View() string {
	var b strings.Builder

	b.WriteString(errorStyle.Render("Delete Worktree"))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("Branch:  %s\n", branchStyle.Render(m.entry.BranchShort)))
	b.WriteString(fmt.Sprintf("Path:    %s\n", pathStyle.Render(m.entry.Worktree.Path)))

	if m.entry.HasSession {
		b.WriteString(fmt.Sprintf("Session: %s\n", sessionActiveStyle.Render(m.entry.SessionName)))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorWarning).Render("This will also kill the tmux session."))
		b.WriteString("\n")
	}

	if m.config.OnDestroy != "" {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorSecondary).Render(
			fmt.Sprintf("Will run on_destroy: %s", m.config.OnDestroy)))
		b.WriteString("\n")
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("Are you sure? %s / %s",
		lipgloss.NewStyle().Foreground(colorDanger).Bold(true).Render("[y]es"),
		lipgloss.NewStyle().Foreground(colorSuccess).Bold(true).Render("[n]o"),
	))

	return dialogStyle.Render(b.String())
}

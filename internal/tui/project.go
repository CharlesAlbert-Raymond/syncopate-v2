package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
)

// ProjectModel is a read-only TUI that shows worktrees across multiple repos.
type ProjectModel struct {
	projectName string
	repos       []string
	entries     []state.Entry
	cursor      int
	width       int
	height      int
	message     string
	msgStyle    lipgloss.Style
}

func NewProjectModel(projectName string, repos []string) ProjectModel {
	return ProjectModel{
		projectName: projectName,
		repos:       repos,
	}
}

type projectEntriesMsg struct {
	entries []state.Entry
}

func fetchProjectEntries(repos []string) tea.Cmd {
	return func() tea.Msg {
		result, err := state.GatherMulti(repos)
		if err != nil {
			return errMsg{err}
		}
		return projectEntriesMsg{entries: result.Entries}
	}
}

func (m ProjectModel) Init() tea.Cmd {
	return tea.Batch(fetchProjectEntries(m.repos), tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	}))
}

func (m ProjectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Batch(fetchProjectEntries(m.repos), tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}))

	case projectEntriesMsg:
		m.entries = msg.entries
		if m.cursor >= len(m.entries) {
			m.cursor = max(0, len(m.entries)-1)
		}
		return m, nil

	case errMsg:
		m.message = msg.Error()
		m.msgStyle = errorStyle
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.cursor < len(m.entries) {
				entry := m.entries[m.cursor]

				if !entry.HasSession {
					repoRoot := entry.RepoRoot
					if repoRoot == "" {
						repoRoot = entry.Worktree.Path
					}
					cfg, err := config.Load(repoRoot)
					if err != nil {
						m.message = fmt.Sprintf("Error loading config: %v", err)
						m.msgStyle = errorStyle
						return m, nil
					}
					if err := tmux.NewSessionWithLayout(entry.SessionName, entry.Worktree.Path, cfg); err != nil {
						m.message = fmt.Sprintf("Error creating session: %v", err)
						m.msgStyle = errorStyle
						return m, nil
					}
				}

				if tmux.IsInsideTmux() {
					if err := tmux.SwitchClient(entry.SessionName); err != nil {
						m.message = fmt.Sprintf("Error switching: %v", err)
						m.msgStyle = errorStyle
						return m, nil
					}
					m.message = fmt.Sprintf("Switched to %s", entry.SessionName)
					m.msgStyle = successStyle
					return m, fetchProjectEntries(m.repos)
				}
			}
		case "r":
			return m, fetchProjectEntries(m.repos)
		}
	}

	return m, nil
}

func (m ProjectModel) View() string {
	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render(logoClassic))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("  project: %s (%d repos)", m.projectName, len(m.repos))))
	b.WriteString("\n\n")

	if len(m.entries) == 0 {
		b.WriteString(subtitleStyle.Render("  No worktrees found across project repos."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  q quit"))
		return b.String()
	}

	// Column header
	header := fmt.Sprintf("  %-3s %-16s %-25s %-14s %s", "", "REPO", "BRANCH", "SESSION", "PATH")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render(header))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  " + strings.Repeat("─", 90)))
	b.WriteString("\n")

	// Group entries by repo label and render with section headers
	currentRepo := ""
	for i, entry := range m.entries {
		// Repo section header
		if entry.RepoLabel != currentRepo {
			currentRepo = entry.RepoLabel
			if i > 0 {
				b.WriteString("\n")
			}
			repoHeader := fmt.Sprintf("  ╭─ %s", currentRepo)
			b.WriteString(lipgloss.NewStyle().Foreground(colorPrimary).Bold(true).Render(repoHeader))
			b.WriteString("\n")
		}

		cursor := "   "
		if i == m.cursor {
			cursor = " ▸ "
		}

		// Branch display
		branch := entry.BranchShort
		bStyle := branchStyle
		if entry.Worktree.IsMain {
			bStyle = mainBranchStyle
			branch += " ★"
		}
		display := branch
		if entry.Title != "" {
			display = truncate(entry.Title, 14) + " " + lipgloss.NewStyle().Foreground(colorMuted).Render("("+truncate(entry.BranchShort, 9)+")")
		}

		repoStr := lipgloss.NewStyle().Foreground(colorMuted).Render(fmt.Sprintf("%-16s", truncate(entry.RepoLabel, 16)))
		branchStr := bStyle.Render(fmt.Sprintf("%-25s", truncate(display, 25)))

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

		shortPath := shortenPath(entry.Worktree.Path)
		pathStr := pathStyle.Render(shortPath)

		row := fmt.Sprintf("%s%s %s %s %s", cursor, repoStr, branchStr, sessStr, pathStr)
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
	b.WriteString(helpStyle.Render("  enter switch • r refresh • q quit"))

	return lipgloss.NewStyle().
		MaxWidth(m.width).
		MaxHeight(m.height).
		Render(b.String())
}

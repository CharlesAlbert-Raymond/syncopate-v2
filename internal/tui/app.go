package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/syncopate/internal/config"
)

const refreshInterval = 2 * time.Second

type view int

const (
	viewList view = iota
	viewCreate
	viewConfirmDelete
)

type errMsg struct{ error }
type tickMsg time.Time

type Model struct {
	currentView view
	list        listModel
	create      createModel
	confirm     confirmModel
	repoRoot    string
	config      config.Config
	width       int
	height      int
	err         error
}

func NewModel(repoRoot string, cfg config.Config) Model {
	return Model{
		currentView: viewList,
		list:        newListModel(),
		repoRoot:    repoRoot,
		config:      cfg,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(fetchEntries(m.repoRoot), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.list.width = msg.Width
		m.list.height = msg.Height
		return m, nil

	case tea.FocusMsg:
		return m, fetchEntries(m.repoRoot)

	case tickMsg:
		// Periodic refresh only on the list view (don't interrupt forms)
		if m.currentView == viewList {
			return m, tea.Batch(fetchEntries(m.repoRoot), tickCmd())
		}
		return m, tickCmd()

	case errMsg:
		m.err = msg.error
		m.list.message = msg.Error()
		m.list.msgStyle = errorStyle
		m.currentView = viewList
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			if m.currentView == viewList {
				return m, tea.Quit
			}
		}
	}

	switch m.currentView {
	case viewList:
		return m.updateList(msg)
	case viewCreate:
		return m.updateCreate(msg)
	case viewConfirmDelete:
		return m.updateConfirm(msg)
	}

	return m, nil
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "c":
			m.currentView = viewCreate
			m.create = newCreateModel(m.repoRoot, m.config)
			return m, m.create.branchInput.Focus()
		case "d":
			if len(m.list.entries) > 0 {
				entry := m.list.entries[m.list.cursor]
				if entry.Worktree.IsMain {
					m.list.message = "Cannot delete the main worktree"
					m.list.msgStyle = errorStyle
					return m, nil
				}
				m.currentView = viewConfirmDelete
				m.confirm = newConfirmModel(entry, m.repoRoot, m.config)
				return m, nil
			}
		case "r":
			return m, fetchEntries(m.repoRoot)
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg, m.repoRoot)
	return m, cmd
}

func (m Model) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.currentView = viewList
			return m, nil
		}
	case createDoneMsg:
		m.currentView = viewList
		m.list.message = "Worktree created successfully"
		m.list.msgStyle = successStyle
		return m, fetchEntries(m.repoRoot)
	}

	var cmd tea.Cmd
	m.create, cmd = m.create.Update(msg)
	return m, cmd
}

func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" || msg.String() == "n" || msg.String() == "N" {
			m.currentView = viewList
			return m, nil
		}
	case deleteDoneMsg:
		m.currentView = viewList
		m.list.message = "Worktree deleted"
		m.list.msgStyle = successStyle
		return m, fetchEntries(m.repoRoot)
	}

	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	var content string

	switch m.currentView {
	case viewList:
		content = m.list.View()
	case viewCreate:
		content = m.list.View() + "\n\n" + m.create.View()
	case viewConfirmDelete:
		content = m.list.View() + "\n\n" + m.confirm.View()
	}

	return lipgloss.NewStyle().
		MaxWidth(m.width).
		MaxHeight(m.height).
		Render(content)
}

package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	"github.com/charles-albert-raymond/syncopate/internal/state"
)

// PopupCreateModel wraps createModel for standalone popup usage.
type PopupCreateModel struct {
	create createModel
}

// NewPopupCreateModel creates a model for the create worktree popup.
func NewPopupCreateModel(repoRoot string, cfg config.Config) PopupCreateModel {
	return PopupCreateModel{
		create: newCreateModel(repoRoot, cfg),
	}
}

func (m PopupCreateModel) Init() tea.Cmd {
	return m.create.branchInput.Focus()
}

func (m PopupCreateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		}
	case createDoneMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.create, cmd = m.create.Update(msg)
	return m, cmd
}

func (m PopupCreateModel) View() string {
	return m.create.View()
}

// PopupConfirmModel wraps confirmModel for standalone popup usage.
type PopupConfirmModel struct {
	confirm confirmModel
}

// NewPopupConfirmModel creates a model for the delete confirmation popup.
func NewPopupConfirmModel(entry state.Entry, repoRoot string, cfg config.Config) PopupConfirmModel {
	return PopupConfirmModel{
		confirm: newConfirmModel(entry, repoRoot, cfg),
	}
}

func (m PopupConfirmModel) Init() tea.Cmd {
	return nil
}

func (m PopupConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "n", "N":
			return m, tea.Quit
		}
	case deleteDoneMsg:
		return m, tea.Quit
	}

	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m PopupConfirmModel) View() string {
	return m.confirm.View()
}

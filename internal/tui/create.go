package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/orchestrate"
)

type createModel struct {
	branchInput textinput.Model
	baseInput   textinput.Model
	focusIndex  int
	err         string
	repoRoot    string
	config      config.Config
}

type createDoneMsg struct{}

func newCreateModel(repoRoot string, cfg config.Config) createModel {
	bi := textinput.New()
	bi.Placeholder = "feature/my-branch"
	bi.Focus()
	bi.CharLimit = 100
	bi.Width = 40
	bi.PromptStyle = inputLabelStyle
	bi.TextStyle = lipgloss.NewStyle().Foreground(colorText)

	base := textinput.New()
	base.Placeholder = "HEAD (default)"
	base.CharLimit = 100
	base.Width = 40
	base.PromptStyle = inputLabelStyle
	base.TextStyle = lipgloss.NewStyle().Foreground(colorText)

	return createModel{
		branchInput: bi,
		baseInput:   base,
		repoRoot:    repoRoot,
		config:      cfg,
	}
}

func (m createModel) Update(msg tea.Msg) (createModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, nil // handled by parent

		case "tab", "shift+tab":
			if m.focusIndex == 0 {
				m.focusIndex = 1
				m.branchInput.Blur()
				m.baseInput.Focus()
			} else {
				m.focusIndex = 0
				m.baseInput.Blur()
				m.branchInput.Focus()
			}
			return m, nil

		case "enter":
			branch := strings.TrimSpace(m.branchInput.Value())
			if branch == "" {
				m.err = "Branch name is required"
				return m, nil
			}

			base := strings.TrimSpace(m.baseInput.Value())

			if _, _, err := orchestrate.CreateWorktree(m.repoRoot, m.config, branch, base); err != nil {
				m.err = fmt.Sprintf("Failed: %v", err)
				return m, nil
			}

			return m, func() tea.Msg { return createDoneMsg{} }
		}
	}

	// Update the focused input
	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.branchInput, cmd = m.branchInput.Update(msg)
	} else {
		m.baseInput, cmd = m.baseInput.Update(msg)
	}
	return m, cmd
}

func (m createModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Create Worktree"))
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Branch name:"))
	b.WriteString("\n")
	b.WriteString(m.branchInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Base branch (optional):"))
	b.WriteString("\n")
	b.WriteString(m.baseInput.View())
	b.WriteString("\n")

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" enter submit • tab switch field • esc cancel"))

	return borderStyle.Render(b.String())
}

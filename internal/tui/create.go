package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/orchestrate"
	"github.com/charles-albert-raymond/synco/internal/worktree"
)

type createMode int

const (
	modeNewBranch createMode = iota
	modeExistingBranch
)

type createModel struct {
	mode createMode

	// New branch fields
	branchInput textinput.Model
	baseInput   textinput.Model
	focusIndex  int // 0=branch, 1=base (new mode)

	// Existing branch fields
	filterInput textinput.Model
	allBranches []string // combined local + remote
	filtered    []string // branches matching filter
	branchIdx   int      // cursor in filtered list
	fetching    bool     // true while git fetch is running
	fetched     bool     // true once branches have been loaded

	err      string
	repoRoot string
	config   config.Config
}

type createDoneMsg struct{}

// branchesMsg is sent when branch listing completes.
type branchesMsg struct {
	branches []string
	err      error
}

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

	fi := textinput.New()
	fi.Placeholder = "type to filter..."
	fi.CharLimit = 100
	fi.Width = 40
	fi.PromptStyle = inputLabelStyle
	fi.TextStyle = lipgloss.NewStyle().Foreground(colorText)

	return createModel{
		mode:        modeNewBranch,
		branchInput: bi,
		baseInput:   base,
		filterInput: fi,
		repoRoot:    repoRoot,
		config:      cfg,
	}
}

// fetchBranchesCmd fetches from remote then lists all branches.
func fetchBranchesCmd(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		// Best-effort fetch; if it fails, we still show local branches
		_ = worktree.Fetch(repoRoot)

		local, err := worktree.BranchList(repoRoot)
		if err != nil {
			return branchesMsg{err: err}
		}
		remote, err := worktree.RemoteBranchList(repoRoot)
		if err != nil {
			return branchesMsg{err: err}
		}

		// Deduplicate: if a local branch exists, skip the remote variant
		localSet := make(map[string]bool, len(local))
		for _, b := range local {
			localSet[b] = true
		}

		combined := make([]string, 0, len(local)+len(remote))
		combined = append(combined, local...)
		for _, rb := range remote {
			// Strip "origin/" prefix for dedup check
			short := rb
			if idx := strings.Index(rb, "/"); idx != -1 {
				short = rb[idx+1:]
			}
			if !localSet[short] {
				combined = append(combined, rb)
			}
		}

		sort.Strings(combined)
		return branchesMsg{branches: combined}
	}
}

func (m createModel) Update(msg tea.Msg) (createModel, tea.Cmd) {
	switch msg := msg.(type) {
	case branchesMsg:
		m.fetching = false
		m.fetched = true
		if msg.err != nil {
			m.err = fmt.Sprintf("Failed to list branches: %v", msg.err)
			return m, nil
		}
		m.allBranches = msg.branches
		m.applyFilter()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			return m, nil // handled by parent

		case "ctrl+e":
			// Toggle between modes
			if m.mode == modeNewBranch {
				m.mode = modeExistingBranch
				m.err = ""
				m.branchInput.Blur()
				m.baseInput.Blur()
				m.filterInput.Focus()
				if !m.fetched {
					m.fetching = true
					return m, fetchBranchesCmd(m.repoRoot)
				}
				return m, nil
			}
			m.mode = modeNewBranch
			m.err = ""
			m.filterInput.Blur()
			m.branchInput.Focus()
			m.focusIndex = 0
			return m, nil

		case "enter":
			return m.handleSubmit()

		case "tab", "shift+tab":
			if m.mode == modeNewBranch {
				return m.toggleNewBranchFocus()
			}
			return m, nil
		}

		// Mode-specific key handling
		if m.mode == modeExistingBranch {
			return m.updateExisting(msg)
		}
		return m.updateNew(msg)
	}

	// Non-key messages: update the active input
	if m.mode == modeExistingBranch {
		var cmd tea.Cmd
		prev := m.filterInput.Value()
		m.filterInput, cmd = m.filterInput.Update(msg)
		if m.filterInput.Value() != prev {
			m.applyFilter()
		}
		return m, cmd
	}

	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.branchInput, cmd = m.branchInput.Update(msg)
	} else {
		m.baseInput, cmd = m.baseInput.Update(msg)
	}
	return m, cmd
}

func (m createModel) handleSubmit() (createModel, tea.Cmd) {
	if m.mode == modeNewBranch {
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

	// Existing branch mode
	if len(m.filtered) == 0 {
		m.err = "No branch selected"
		return m, nil
	}
	branch := m.filtered[m.branchIdx]
	if _, _, err := orchestrate.CreateWorktreeFromExisting(m.repoRoot, m.config, branch); err != nil {
		m.err = fmt.Sprintf("Failed: %v", err)
		return m, nil
	}
	return m, func() tea.Msg { return createDoneMsg{} }
}

func (m createModel) toggleNewBranchFocus() (createModel, tea.Cmd) {
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
}

func (m createModel) updateNew(msg tea.KeyMsg) (createModel, tea.Cmd) {
	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.branchInput, cmd = m.branchInput.Update(msg)
	} else {
		m.baseInput, cmd = m.baseInput.Update(msg)
	}
	return m, cmd
}

func (m createModel) updateExisting(msg tea.KeyMsg) (createModel, tea.Cmd) {
	switch msg.String() {
	case "down", "ctrl+n":
		if m.branchIdx < len(m.filtered)-1 {
			m.branchIdx++
		}
		return m, nil
	case "up", "ctrl+p":
		if m.branchIdx > 0 {
			m.branchIdx--
		}
		return m, nil
	}

	// Pass to filter input
	var cmd tea.Cmd
	prev := m.filterInput.Value()
	m.filterInput, cmd = m.filterInput.Update(msg)
	if m.filterInput.Value() != prev {
		m.applyFilter()
	}
	return m, cmd
}

func (m *createModel) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	if query == "" {
		m.filtered = m.allBranches
	} else {
		m.filtered = m.filtered[:0]
		for _, b := range m.allBranches {
			if fuzzyMatch(strings.ToLower(b), query) {
				m.filtered = append(m.filtered, b)
			}
		}
	}
	m.branchIdx = 0
}

// fuzzyMatch checks if all characters in pattern appear in s in order.
func fuzzyMatch(s, pattern string) bool {
	pi := 0
	for i := 0; i < len(s) && pi < len(pattern); i++ {
		if s[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func (m createModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Create Worktree"))
	b.WriteString("\n\n")

	// Mode tabs
	newTab := " New Branch "
	existTab := " Existing Branch "
	if m.mode == modeNewBranch {
		newTab = tabActiveStyle.Render(newTab)
		existTab = tabInactiveStyle.Render(existTab)
	} else {
		newTab = tabInactiveStyle.Render(newTab)
		existTab = tabActiveStyle.Render(existTab)
	}
	b.WriteString(newTab + " " + existTab)
	b.WriteString("\n\n")

	if m.mode == modeNewBranch {
		m.viewNewBranch(&b)
	} else {
		m.viewExistingBranch(&b)
	}

	if m.err != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render(m.err))
	}

	b.WriteString("\n")
	if m.mode == modeNewBranch {
		b.WriteString(helpStyle.Render(" enter submit • tab switch field • ctrl+e existing • esc cancel"))
	} else {
		b.WriteString(helpStyle.Render(" enter submit • ↑/↓ select • ctrl+e new branch • esc cancel"))
	}

	return borderStyle.Render(b.String())
}

func (m createModel) viewNewBranch(b *strings.Builder) {
	b.WriteString(inputLabelStyle.Render("Branch name:"))
	b.WriteString("\n")
	b.WriteString(m.branchInput.View())
	b.WriteString("\n\n")

	b.WriteString(inputLabelStyle.Render("Base branch (optional):"))
	b.WriteString("\n")
	b.WriteString(m.baseInput.View())
}

func (m createModel) viewExistingBranch(b *strings.Builder) {
	b.WriteString(inputLabelStyle.Render("Filter:"))
	b.WriteString("\n")
	b.WriteString(m.filterInput.View())
	b.WriteString("\n\n")

	if m.fetching {
		b.WriteString(subtitleStyle.Render("  Fetching branches..."))
		return
	}

	if len(m.filtered) == 0 {
		if m.fetched {
			b.WriteString(subtitleStyle.Render("  No matching branches"))
		}
		return
	}

	// Show up to 8 branches around the cursor
	maxVisible := 8
	start := m.branchIdx - maxVisible/2
	if start < 0 {
		start = 0
	}
	end := start + maxVisible
	if end > len(m.filtered) {
		end = len(m.filtered)
		start = end - maxVisible
		if start < 0 {
			start = 0
		}
	}

	if start > 0 {
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↑ %d more", start)))
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		branch := m.filtered[i]
		if i == m.branchIdx {
			b.WriteString(selectedRowStyle.Render(" ▸ " + branch))
		} else {
			isRemote := strings.Contains(branch, "/")
			if isRemote {
				b.WriteString("   " + subtitleStyle.Render(branch))
			} else {
				b.WriteString("   " + branchStyle.Render(branch))
			}
		}
		b.WriteString("\n")
	}

	remaining := len(m.filtered) - end
	if remaining > 0 {
		b.WriteString(subtitleStyle.Render(fmt.Sprintf("  ↓ %d more", remaining)))
	}

	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("  %d/%d branches", len(m.filtered), len(m.allBranches))))
}

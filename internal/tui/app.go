package tui

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/metadata"
	"github.com/charles-albert-raymond/synco/internal/tmux"
)

const refreshInterval = 2 * time.Second

type view int

const (
	viewList view = iota
	viewCreate
	viewConfirmDelete
	viewConfig
	viewEditTitle
)

type errMsg struct{ error }
type tickMsg time.Time
type popupDoneMsg struct{}
type rebuildDoneMsg struct{ err error }

type Model struct {
	currentView      view
	list             listModel
	create           createModel
	confirm          confirmModel
	configView       configViewModel
	editTitle        editTitleModel
	repoRoot         string
	config           config.Config
	width            int
	height           int
	err              error
	sidebarMode      bool
	sourceDir        string // synco source dir for rebuilding (set via ldflags)
	RestartRequested bool   // signals main() to re-exec after quit
}

func NewModel(repoRoot string, cfg config.Config, sourceDir string) Model {
	lm := newListModel()
	lm.config = cfg
	return Model{
		currentView: viewList,
		list:        lm,
		repoRoot:    repoRoot,
		config:      cfg,
		sourceDir:   sourceDir,
	}
}

func NewSidebarModel(repoRoot string, cfg config.Config, sourceDir string) Model {
	lm := newListModel()
	lm.config = cfg
	return Model{
		currentView: viewList,
		list:        lm,
		repoRoot:    repoRoot,
		config:      cfg,
		sidebarMode: true,
		sourceDir:   sourceDir,
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
		if m.sidebarMode {
			m.list.resetCursorOnNext = true
		}
		return m, fetchEntries(m.repoRoot)

	case tea.BlurMsg:
		if m.sidebarMode {
			// When sidebar loses focus, reset cursor to the current worktree
			m.list.resetCursorToCurrent()
			return m, unfocusSidebar()
		}
		return m, nil

	case popupDoneMsg:
		return m, fetchEntries(m.repoRoot)

	case tickMsg:
		// Periodic refresh only on the list view (don't interrupt forms)
		if m.currentView == viewList {
			return m, tea.Batch(fetchEntries(m.repoRoot), tickCmd())
		}
		return m, tickCmd()

	case rebuildDoneMsg:
		if msg.err != nil {
			m.list.message = fmt.Sprintf("Rebuild failed: %v", msg.err)
			m.list.msgStyle = errorStyle
			m.currentView = viewList
			return m, nil
		}
		m.RestartRequested = true
		return m, tea.Quit

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
		case "ctrl+r":
			if !m.list.filtering {
				return m, m.rebuildCmd()
			}
		case "q":
			if m.currentView == viewList && !m.list.filtering {
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
	case viewConfig:
		return m.updateConfig(msg)
	case viewEditTitle:
		return m.updateEditTitle(msg)
	}

	return m, nil
}

func (m Model) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// When filtering, only handle esc here; let list model handle the rest
		if m.list.filtering {
			if msg.String() == "esc" {
				m.list.exitFilter()
				return m, nil
			}
			// Fall through to list Update/UpdateSidebar which handles filter input
			break
		}

		switch msg.String() {
		case "c":
			if m.sidebarMode {
				return m, launchCreatePopup(m.repoRoot)
			}
			m.currentView = viewCreate
			m.create = newCreateModel(m.repoRoot, m.config)
			return m, m.create.branchInput.Focus()
		case "e":
			entry, ok := m.list.selectedEntry()
			if ok {
				if m.sidebarMode {
					return m, launchEditTitlePopup(m.repoRoot, entry.BranchShort, entry.Title)
				}
				m.currentView = viewEditTitle
				m.editTitle = newEditTitleModel(entry.BranchShort, entry.Title, m.repoRoot, m.config)
				return m, textinput.Blink
			}
		case "d":
			if len(m.list.entries) > 0 {
				entry, ok := m.list.selectedEntry()
				if !ok {
					return m, nil
				}
				if entry.Worktree.IsMain {
					m.list.message = "Cannot delete the main worktree"
					m.list.msgStyle = errorStyle
					return m, nil
				}
				if m.sidebarMode {
					return m, launchDeletePopup(m.repoRoot, entry.BranchShort)
				}
				m.currentView = viewConfirmDelete
				m.confirm = newConfirmModel(entry, m.repoRoot, m.config)
				return m, nil
			}
		case "r":
			return m, fetchEntries(m.repoRoot)
		case "?":
			m.currentView = viewConfig
			m.configView = newConfigViewModel(m.config, m.repoRoot)
			return m, nil
		case "esc":
			if m.sidebarMode {
				return m, unfocusSidebar()
			}
		}
	}

	if m.sidebarMode {
		var cmd tea.Cmd
		m.list, cmd = m.list.UpdateSidebar(msg, m.repoRoot)
		return m, cmd
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
		if msg.title != "" {
			m.saveTitle(msg.branch, msg.title)
		}
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
		m.deleteTitle(m.confirm.entry.BranchShort)
		return m, fetchEntries(m.repoRoot)
	}

	var cmd tea.Cmd
	m.confirm, cmd = m.confirm.Update(msg)
	return m, cmd
}

func (m Model) updateConfig(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		if msg.String() == "?" || msg.String() == "esc" {
			m.currentView = viewList
			return m, nil
		}
	}
	return m, nil
}

func (m Model) updateEditTitle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.currentView = viewList
			return m, nil
		}
	case editTitleDoneMsg:
		m.currentView = viewList
		m.saveTitle(msg.branch, msg.title)
		m.list.message = "Title updated"
		m.list.msgStyle = successStyle
		return m, fetchEntries(m.repoRoot)
	}

	var cmd tea.Cmd
	m.editTitle, cmd = m.editTitle.Update(msg)
	return m, cmd
}

func (m Model) saveTitle(branch, title string) {
	store, err := metadata.Load(m.repoRoot, m.config.WorktreeDir)
	if err != nil {
		return
	}
	if title == "" {
		store.Delete(branch)
	} else {
		store.SetTitle(branch, title)
	}
	_ = store.Save(m.repoRoot, m.config.WorktreeDir)
}

func (m Model) deleteTitle(branch string) {
	store, err := metadata.Load(m.repoRoot, m.config.WorktreeDir)
	if err != nil {
		return
	}
	store.Delete(branch)
	_ = store.Save(m.repoRoot, m.config.WorktreeDir)
}

func (m Model) View() string {
	var content string

	if m.sidebarMode {
		switch m.currentView {
		case viewList:
			content = m.list.ViewCompact(m.width)
		case viewCreate:
			content = m.list.ViewCompact(m.width) + "\n" + m.create.View()
		case viewConfirmDelete:
			content = m.list.ViewCompact(m.width) + "\n" + m.confirm.View()
		case viewEditTitle:
			content = m.list.ViewCompact(m.width) + "\n" + m.editTitle.View()
		case viewConfig:
			content = m.configView.View()
		}
	} else {
		switch m.currentView {
		case viewList:
			content = m.list.View()
		case viewCreate:
			content = m.list.View() + "\n\n" + m.create.View()
		case viewConfirmDelete:
			content = m.list.View() + "\n\n" + m.confirm.View()
		case viewEditTitle:
			content = m.list.View() + "\n\n" + m.editTitle.View()
		case viewConfig:
			content = m.configView.View()
		}
	}

	return lipgloss.NewStyle().
		MaxWidth(m.width).
		MaxHeight(m.height).
		Render(content)
}

func launchCreatePopup(repoRoot string) tea.Cmd {
	return func() tea.Msg {
		_ = tmux.LaunchPopup(
			[]string{"--popup-create", "--root", repoRoot},
			70, 28, "Create Worktree",
		)
		return popupDoneMsg{}
	}
}

func launchEditTitlePopup(repoRoot string, branch, currentTitle string) tea.Cmd {
	return func() tea.Msg {
		args := []string{"--popup-edit-title", "--root", repoRoot, "--branch", branch}
		if currentTitle != "" {
			args = append(args, "--title", currentTitle)
		}
		_ = tmux.LaunchPopup(args, 60, 14, "Edit Title")
		return popupDoneMsg{}
	}
}

func launchDeletePopup(repoRoot string, branch string) tea.Cmd {
	return func() tea.Msg {
		_ = tmux.LaunchPopup(
			[]string{"--popup-delete", "--root", repoRoot, "--branch", branch},
			60, 20, "Delete Worktree",
		)
		return popupDoneMsg{}
	}
}

// rebuildCmd rebuilds the synco binary from source and signals a restart.
// If no source directory is available, it reloads config only and re-execs.
func (m Model) rebuildCmd() tea.Cmd {
	return func() tea.Msg {
		if m.sourceDir != "" {
			exe, err := os.Executable()
			if err != nil {
				return rebuildDoneMsg{err: fmt.Errorf("find executable: %w", err)}
			}
			cmd := exec.Command("go", "build", "-o", exe, ".")
			cmd.Dir = m.sourceDir
			if out, err := cmd.CombinedOutput(); err != nil {
				return rebuildDoneMsg{err: fmt.Errorf("%s: %w", string(out), err)}
			}
		}
		// Even without sourceDir, signal restart to reload config + pick up
		// any externally updated binary.
		return rebuildDoneMsg{}
	}
}

func unfocusSidebar() tea.Cmd {
	return func() tea.Msg {
		session, err := tmux.CurrentSessionName()
		if err == nil {
			tmux.FocusMainPane(session)
		}
		return nil
	}
}

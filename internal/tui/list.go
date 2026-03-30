package tui

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/notify"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
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
	entries            []state.Entry
	otherPorts         []state.SessionPorts // non-synco sessions with ports
	cursor             int
	resetCursorOnNext  bool // reset cursor to current worktree on next entriesMsg
	width              int
	height             int
	message            string
	msgStyle           lipgloss.Style
	config             config.Config
	notified           map[string]bool // sessions that already fired a notification
	silentSessions     map[string]bool // sessions currently in silence (for red dot)
	filtering          bool            // true when fuzzy filter input is active
	filterInput        textinput.Model
	filteredIndices    []int           // indices into entries that match; nil = show all
}

func newListModel() listModel {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 64
	return listModel{
		resetCursorOnNext: true,
		notified:          make(map[string]bool),
		silentSessions:    make(map[string]bool),
		filterInput:       ti,
	}
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
		if m.filtering {
			m.applyFilter()
		} else {
			m.clampCursor()
		}
		return m, m.detectSilence()

	case attachMsg:
		m.message = fmt.Sprintf("Detached from %s", msg.session)
		m.msgStyle = successStyle
		m.exitFilter()
		return m, fetchEntries(repoRoot)

	case tea.KeyMsg:
		// Filter mode: intercept keys
		if m.filtering {
			return m.updateFilter(msg, repoRoot, false)
		}

		switch {
		case msg.String() == "/":
			m.enterFilter()
			return m, nil
		case msg.String() == "j" || msg.String() == "down" ||
			msg.String() == "k" || msg.String() == "up":
			m.moveCursor(msg.String())
		case msg.String() == "enter":
			entry, ok := m.selectedEntry()
			if !ok {
				return m, nil
			}
			m.clearNotification(entry.SessionName)

			if err := m.ensureSession(entry); err != nil {
				m.message = fmt.Sprintf("Error creating session: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			if tmux.IsInsideTmux() {
				if err := tmux.SwitchClient(entry.SessionName); err != nil {
					m.message = fmt.Sprintf("Error switching: %v", err)
					m.msgStyle = errorStyle
					return m, nil
				}
				m.message = fmt.Sprintf("Switched to %s", entry.SessionName)
				m.msgStyle = successStyle
				return m, fetchEntries(repoRoot)
			}

			// Outside tmux: use tea.Exec to attach
			c := &tmuxExecCmd{cmd: exec.Command("tmux", "attach-session", "-t", entry.SessionName)}
			return m, tea.Exec(c, func(err error) tea.Msg {
				if err != nil {
					return errMsg{err}
				}
				return attachMsg{session: entry.SessionName}
			})
		}
	}
	return m, nil
}

// updateFilter handles key events while the filter input is active.
// sidebarMode controls whether enter uses sidebar or classic attach logic.
func (m listModel) updateFilter(msg tea.KeyMsg, repoRoot string, sidebarMode bool) (listModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.exitFilter()
		return m, nil
	case "enter":
		entry, ok := m.selectedEntry()
		if !ok {
			return m, nil
		}
		m.exitFilter()
		m.clearNotification(entry.SessionName)

		if err := m.ensureSession(entry); err != nil {
			m.message = fmt.Sprintf("Error creating session: %v", err)
			m.msgStyle = errorStyle
			return m, nil
		}

		if sidebarMode {
			if err := tmux.EnsureSidebar(entry.SessionName, repoRoot); err != nil {
				m.message = fmt.Sprintf("Error adding sidebar: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}
			if err := tmux.SwitchClient(entry.SessionName); err != nil {
				m.message = fmt.Sprintf("Error switching: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}
			tmux.FocusMainPane(entry.SessionName)
			m.message = fmt.Sprintf("→ %s", entry.BranchShort)
			m.msgStyle = successStyle
			return m, fetchEntries(repoRoot)
		}

		if tmux.IsInsideTmux() {
			if err := tmux.SwitchClient(entry.SessionName); err != nil {
				m.message = fmt.Sprintf("Error switching: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}
			m.message = fmt.Sprintf("Switched to %s", entry.SessionName)
			m.msgStyle = successStyle
			return m, fetchEntries(repoRoot)
		}
		c := &tmuxExecCmd{cmd: exec.Command("tmux", "attach-session", "-t", entry.SessionName)}
		return m, tea.Exec(c, func(err error) tea.Msg {
			if err != nil {
				return errMsg{err}
			}
			return attachMsg{session: entry.SessionName}
		})

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		visible := m.visibleEntries()
		if m.cursor < len(visible)-1 {
			m.cursor++
		}
		return m, nil
	}

	// Forward to text input
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.applyFilter()
	return m, cmd
}

// UpdateSidebar handles input in sidebar mode.
// On enter: ensure the worktree session has a sidebar, then switch-client to it.
func (m listModel) UpdateSidebar(msg tea.Msg, repoRoot string) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case entriesMsg:
		m.entries = msg.entries
		m.otherPorts = msg.otherPorts
		if m.filtering {
			m.applyFilter()
		} else if m.resetCursorOnNext {
			m.resetCursorToCurrent()
			m.resetCursorOnNext = false
		} else {
			m.clampCursor()
		}
		return m, m.detectSilence()

	case attachMsg:
		m.message = fmt.Sprintf("Switched to %s", msg.session)
		m.msgStyle = successStyle
		m.exitFilter()
		return m, fetchEntries(repoRoot)

	case tea.KeyMsg:
		// Filter mode: intercept keys
		if m.filtering {
			return m.updateFilter(msg, repoRoot, true)
		}

		switch {
		case msg.String() == "/":
			m.enterFilter()
			return m, nil
		case msg.String() == "j" || msg.String() == "down" ||
			msg.String() == "k" || msg.String() == "up":
			m.moveCursor(msg.String())
		case msg.String() == "enter":
			entry, ok := m.selectedEntry()
			if !ok {
				return m, nil
			}
			m.clearNotification(entry.SessionName)

			if err := m.ensureSession(entry); err != nil {
				m.message = fmt.Sprintf("Error creating session: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			// Ensure the target session has a sidebar pane
			if err := tmux.EnsureSidebar(entry.SessionName, repoRoot); err != nil {
				m.message = fmt.Sprintf("Error adding sidebar: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			// Switch the tmux client to the worktree's session
			if err := tmux.SwitchClient(entry.SessionName); err != nil {
				m.message = fmt.Sprintf("Error switching: %v", err)
				m.msgStyle = errorStyle
				return m, nil
			}

			// Focus the main work pane (not the sidebar) in the target session
			tmux.FocusMainPane(entry.SessionName)

			m.message = fmt.Sprintf("→ %s", entry.BranchShort)
			m.msgStyle = successStyle
			return m, fetchEntries(repoRoot)
		}
	}
	return m, nil
}

// moveCursor handles j/k/up/down key navigation.
func (m *listModel) moveCursor(key string) {
	count := len(m.visibleEntries())
	switch key {
	case "j", "down":
		if m.cursor < count-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	}
}

// clampCursor ensures the cursor is within bounds of the entries slice.
func (m *listModel) clampCursor() {
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
}

// ensureSession creates a tmux session for the entry if one doesn't exist.
// Returns an error message suitable for display, or empty string on success.
func (m *listModel) ensureSession(entry state.Entry) error {
	if entry.HasSession {
		return nil
	}
	return tmux.NewSessionWithLayout(entry.SessionName, entry.Worktree.Path, m.config)
}

// resetCursorToCurrent moves the cursor to the entry marked IsCurrent.
// If no entry is current, clamps the cursor within bounds.
func (m *listModel) resetCursorToCurrent() {
	for i, e := range m.entries {
		if e.IsCurrent {
			m.cursor = i
			return
		}
	}
	m.clampCursor()
}

// enterFilter activates the fuzzy filter input.
func (m *listModel) enterFilter() {
	m.filtering = true
	m.filterInput.SetValue("")
	m.filterInput.Focus()
	m.filteredIndices = nil
}

// exitFilter deactivates the filter and restores the full list.
func (m *listModel) exitFilter() {
	m.filtering = false
	m.filterInput.Blur()
	m.filterInput.SetValue("")
	m.filteredIndices = nil
	m.clampCursor()
}

// applyFilter recalculates filteredIndices from the current filter text.
func (m *listModel) applyFilter() {
	query := m.filterInput.Value()
	if query == "" {
		m.filteredIndices = nil
		m.clampCursor()
		return
	}

	// Build searchable strings: branch + alias + title
	sources := make([]string, len(m.entries))
	for i, e := range m.entries {
		s := e.BranchShort
		if alias := m.config.AliasFor(e.BranchShort); alias != "" {
			s += " " + alias
		}
		if e.Title != "" {
			s += " " + e.Title
		}
		sources[i] = s
	}

	matches := fuzzy.Find(query, sources)
	m.filteredIndices = make([]int, len(matches))
	for i, match := range matches {
		m.filteredIndices[i] = match.Index
	}

	// Clamp cursor to filtered list
	if m.cursor >= len(m.filteredIndices) {
		m.cursor = max(0, len(m.filteredIndices)-1)
	}
}

// visibleEntries returns the entries to display (filtered or all).
func (m *listModel) visibleEntries() []state.Entry {
	if m.filteredIndices == nil {
		return m.entries
	}
	result := make([]state.Entry, len(m.filteredIndices))
	for i, idx := range m.filteredIndices {
		result[i] = m.entries[idx]
	}
	return result
}

// selectedEntry returns the entry under the cursor, accounting for filtering.
// Returns the entry and true, or zero value and false if the list is empty.
func (m *listModel) selectedEntry() (state.Entry, bool) {
	visible := m.visibleEntries()
	if m.cursor >= len(visible) || len(visible) == 0 {
		return state.Entry{}, false
	}
	return visible[m.cursor], true
}

// ViewCompact renders a narrow sidebar-friendly view with per-worktree cards.
func (m listModel) ViewCompact(width int) string {
	if width == 0 {
		width = 28
	}
	boxWidth := width - 2 // leave 1 char margin each side

	var parts []string

	// Header
	parts = append(parts, sidebarHeaderStyle.Render(logoSidebar))

	// Filter input
	if m.filtering {
		filterLine := filterPromptStyle.Render("/") + filterInputStyle.Render(m.filterInput.View())
		parts = append(parts, helpBoxStyle.Render(filterLine))
	}

	visible := m.visibleEntries()

	if len(visible) == 0 {
		msg := "No worktrees."
		if m.filtering && len(m.entries) > 0 {
			msg = "No matches."
		}
		card := sidebarBoxStyle.
			Width(boxWidth).
			BorderForeground(colorMuted).
			Render(subtitleStyle.Render(msg))
		parts = append(parts, card)
		if !m.filtering {
			parts = append(parts, helpBoxStyle.Render(
				lipgloss.NewStyle().Foreground(colorMuted).Render("c create • q quit"),
			))
		}
		return strings.Join(parts, "\n")
	}

	// Inner width inside card: boxWidth - border(2) - padding(2) = boxWidth - 4
	innerWidth := boxWidth - 4

	// Render a card for each worktree
	for i, entry := range visible {
		// Resolve display name: title > alias > branch
		displayName := entry.Title
		if displayName == "" {
			displayName = m.config.AliasFor(entry.BranchShort)
		}
		showBranchBelow := displayName != ""
		if displayName == "" {
			displayName = entry.BranchShort
		}
		if entry.Worktree.IsMain {
			displayName += " ★"
		}

		// Session status dot
		var dot string
		if m.silentSessions[entry.SessionName] {
			dot = notificationDotStyle.Render("●")
		} else if entry.HasSession {
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

		// Line 2: show branch name when a title or alias provides the primary label
		if showBranchBelow {
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
		lipgloss.NewStyle().Foreground(colorMuted).Render("↵ open · c new · e title")+
			"\n"+
			lipgloss.NewStyle().Foreground(colorMuted).Render("d del · / filter · ^r build"),
	))

	return strings.Join(parts, "\n")
}

func (m listModel) View() string {
	visible := m.visibleEntries()

	if len(m.entries) == 0 {
		return titleStyle.Render(logoClassic) + "\n" +
			subtitleStyle.Render("  git worktree orchestrator") + "\n\n" +
			subtitleStyle.Render("  No worktrees found. Press 'c' to create one.") + "\n\n" +
			m.renderHelp()
	}

	var b strings.Builder

	// Header
	b.WriteString(titleStyle.Render(logoClassic))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("  git worktree orchestrator"))
	b.WriteString("\n\n")

	// Filter input
	if m.filtering {
		filterLine := "  " + filterPromptStyle.Render("/ ") + filterInputStyle.Render(m.filterInput.View())
		b.WriteString(filterLine)
		b.WriteString("\n\n")
	}

	// Column header
	header := fmt.Sprintf("  %-3s %-25s %-14s %-16s %s", "", "BRANCH", "SESSION", "PORTS", "PATH")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Bold(true).Render(header))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(colorMuted).Render("  " + strings.Repeat("─", 85)))
	b.WriteString("\n")

	if len(visible) == 0 && m.filtering {
		b.WriteString("\n")
		b.WriteString("  " + subtitleStyle.Render("No matches."))
		b.WriteString("\n")
		b.WriteString(m.renderHelp())
		return b.String()
	}

	// Entries
	for i, entry := range visible {
		cursor := "   "
		if i == m.cursor {
			cursor = " ▸ "
		}

		// Branch / title display
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
		branchStr := bStyle.Render(fmt.Sprintf("%-25s", truncate(display, 25)))

		// Session status
		var sessStr string
		if m.silentSessions[entry.SessionName] {
			sessStr = notificationDotStyle.Render("● idle        ")
		} else if entry.HasSession {
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
	help := "  enter attach • / filter • c create • e title • d delete • r refresh • ^r rebuild • ? config • q quit"
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
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-1]) + "…"
}

// detectSilence checks for signal files written by `synco notify` (triggered
// by Claude Code's Stop hook) and returns notification commands.
func (m *listModel) detectSilence() tea.Cmd {
	if !m.config.NotificationsEnabled() {
		return nil
	}

	var cmds []tea.Cmd

	for _, entry := range m.entries {
		sess := entry.SessionName
		if !entry.HasSession {
			delete(m.silentSessions, sess)
			delete(m.notified, sess)
			continue
		}

		if notify.HasSignal(sess) {
			m.silentSessions[sess] = true
			if !m.notified[sess] {
				m.notified[sess] = true
				if m.config.BellEnabled() {
					cmds = append(cmds, sendBellCmd())
				}
				if m.config.SystemNotificationEnabled() {
					cmds = append(cmds, sendSystemNotificationCmd(entry.BranchShort, m.config.NotificationSound()))
				}
				if m.config.Notifications != nil && m.config.Notifications.OnSilence != "" {
					cmds = append(cmds, runOnSilenceHookCmd(m.config.Notifications.OnSilence, entry.BranchShort, sess))
				}
			}
		} else {
			delete(m.silentSessions, sess)
			delete(m.notified, sess)
		}
	}

	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

// clearNotification removes the notification state and signal file for a session.
func (m *listModel) clearNotification(session string) {
	delete(m.silentSessions, session)
	delete(m.notified, session)
	notify.ClearSignal(session)
}

func sendBellCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Print("\a")
		return nil
	}
}

func sendSystemNotificationCmd(branch, sound string) tea.Cmd {
	return func() tea.Msg {
		script := `display notification "Claude finished working on this branch." with title "🎵 synco" subtitle (system attribute "SYNCO_BRANCH") sound name (system attribute "SYNCO_SOUND")`
		cmd := exec.Command("osascript", "-e", script)
		cmd.Env = append(os.Environ(),
			"SYNCO_BRANCH="+branch,
			"SYNCO_SOUND="+sound,
		)
		_ = cmd.Run()
		return nil
	}
}

func runOnSilenceHookCmd(hook, branch, session string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("sh", "-c", hook)
		cmd.Env = append(os.Environ(),
			"SYNCO_BRANCH="+branch,
			"SYNCO_SESSION="+session,
		)
		_ = cmd.Run()
		return nil
	}
}

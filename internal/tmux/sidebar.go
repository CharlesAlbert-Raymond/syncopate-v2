package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/charles-albert-raymond/synco/internal/config"
)

const defaultSidebarWidth = "28"

// LaunchState describes the current tmux environment when synco is invoked.
type LaunchState int

const (
	// OutsideNoSession — not in tmux, no synco sessions exist.
	OutsideNoSession LaunchState = iota
	// OutsideHasSession — not in tmux, but synco sessions exist.
	OutsideHasSession
	// InsideNoSidebar — inside a tmux session, but no sidebar pane.
	InsideNoSidebar
	// InsideHasSidebar — inside a tmux session, sidebar already running.
	InsideHasSidebar
)

// DetectState figures out which of the 4 launch states we're in.
func DetectState(project string) LaunchState {
	if !IsInsideTmux() {
		sessions, _ := ListSessions(project)
		if len(sessions) > 0 {
			return OutsideHasSession
		}
		return OutsideNoSession
	}

	if hasSidebarPane() {
		return InsideHasSidebar
	}
	return InsideNoSidebar
}

// termSize returns the current terminal dimensions (columns, rows).
// Falls back to 0, 0 if the terminal size cannot be determined (tmux will use its default).
func termSize() (cols, rows uint16) {
	var ws struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(os.Stdout.Fd()),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0
	}
	return ws.Col, ws.Row
}

// CreateSessionAndAttach creates a new tmux session at repoRoot with sidebar, then attaches.
func CreateSessionAndAttach(repoRoot string, sidebarWidth string, cfg config.Config) error {
	project := ProjectName(repoRoot)
	sessName := SessionNameFor(project, RootSessionKey)

	args := []string{"new-session", "-d", "-s", sessName, "-c", repoRoot}
	// Set the detached session size to match the current terminal so that
	// layout panes and sidebar are created at the correct dimensions.
	if cols, rows := termSize(); cols > 0 && rows > 0 {
		args = append(args, "-x", fmt.Sprintf("%d", cols), "-y", fmt.Sprintf("%d", rows))
	}
	cmd := exec.Command("tmux", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %s: %w", string(out), err)
	}

	// Apply layout before sidebar so pane indices are predictable
	if layout := cfg.DefaultLayout(); layout != nil {
		if err := ApplyLayout(sessName, layout); err != nil {
			return fmt.Errorf("apply layout: %w", err)
		}
	}

	// Apply theme (border colors, labels)
	if err := ApplyTheme(sessName, cfg.Theme); err != nil {
		return fmt.Errorf("apply theme: %w", err)
	}

	if err := addSidebar(sessName, repoRoot, sidebarWidth); err != nil {
		return err
	}

	// Replace the current process with tmux attach for clean terminal handoff.
	// Using syscall.Exec (instead of cmd.Run) ensures proper terminal sizing
	// so the layout created in the detached session resizes correctly on attach.
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessName}, os.Environ())
}

// AttachFirstSession attaches to the first available synco session for the given project.
func AttachFirstSession(project string) error {
	sessions, err := ListSessions(project)
	if err != nil || len(sessions) == 0 {
		return fmt.Errorf("no synco sessions found")
	}

	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", sessions[0].Name}, os.Environ())
}

// AddSidebarToCurrent splits the current tmux session and adds a sidebar pane.
func AddSidebarToCurrent(repoRoot string, sidebarWidth string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own binary: %w", err)
	}

	// Remember the active pane so we can restore focus after the split.
	session, _ := CurrentSessionName()
	activePaneBefore := ""
	if session != "" {
		activePaneBefore = activePane(session)
	}

	cmd := exec.Command("tmux", "split-window", "-fhb",
		"-l", sidebarWidth,
		binary, "--sidebar", "--root", repoRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %w", string(out), err)
	}

	// Restore focus to the work pane.
	if activePaneBefore != "" {
		_ = exec.Command("tmux", "select-pane", "-t", activePaneBefore).Run()
	}

	return nil
}

// EnsureSidebar makes sure the given session has a synco sidebar pane.
// If it already has one, this is a no-op.
func EnsureSidebar(session, repoRoot string) error {
	if hasSidebarPaneInSession(session) {
		return nil
	}
	return addSidebar(session, repoRoot, defaultSidebarWidth)
}

// addSidebar splits a sidebar into the left side of the given session.
func addSidebar(session, repoRoot string, width string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own binary: %w", err)
	}

	// Remember the active work pane before splitting so we can restore focus.
	activePaneBefore := activePane(session)

	cmd := exec.Command("tmux", "split-window", "-fhb",
		"-l", width,
		"-t", session,
		binary, "--sidebar", "--root", repoRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %w", string(out), err)
	}

	// Restore focus to the work pane that was active before the sidebar split.
	if activePaneBefore != "" {
		_ = exec.Command("tmux", "select-pane", "-t", activePaneBefore).Run()
	}

	return nil
}

// hasSidebarPane checks if the current session has a pane running synco --sidebar.
func hasSidebarPane() bool {
	cmd := exec.Command("tmux", "list-panes", "-F", "#{pane_current_command} #{pane_start_command}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "--sidebar") {
			return true
		}
	}
	return false
}

// hasSidebarPaneInSession checks if a specific session has a synco sidebar.
func hasSidebarPaneInSession(session string) bool {
	cmd := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_start_command}")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "--sidebar") {
			return true
		}
	}
	return false
}

// FocusMainPane selects the main (non-sidebar) pane in the given session.
func FocusMainPane(session string) {
	mainPane := findMainPane(session)
	if mainPane == "" {
		return
	}
	_ = exec.Command("tmux", "select-pane", "-t", mainPane).Run()
}

// activePane returns the pane_id of the currently active pane in a session.
func activePane(session string) string {
	cmd := exec.Command("tmux", "display-message", "-t", session, "-p", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// findMainPane returns the pane_id of the first non-sidebar pane in a session.
func findMainPane(session string) string {
	cmd := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_id}\t#{pane_start_command}")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		if !strings.Contains(parts[1], "--sidebar") {
			return parts[0]
		}
	}
	return ""
}

func listPanes(session string) ([]string, error) {
	cmd := exec.Command("tmux", "list-panes", "-t", session, "-F", "#{pane_id}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	return strings.Fields(strings.TrimSpace(string(out))), nil
}

// LaunchPopup opens a tmux popup overlay centered on the main work pane.
// Blocks until the popup command exits.
func LaunchPopup(args []string, width, height int, title string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own binary: %w", err)
	}

	cmdArgs := []string{"display-popup", "-EE",
		"-w", fmt.Sprintf("%d", width),
		"-h", fmt.Sprintf("%d", height),
	}

	// Target the main (non-sidebar) pane so the popup centers on it
	// instead of the narrow sidebar pane.
	if session, err := CurrentSessionName(); err == nil {
		if panes, err := listPanes(session); err == nil && len(panes) >= 2 {
			cmdArgs = append(cmdArgs, "-t", panes[len(panes)-1])
		}
	}

	if title != "" {
		cmdArgs = append(cmdArgs, "-T", fmt.Sprintf(" %s ", title))
	}
	cmdArgs = append(cmdArgs, "--", binary)
	cmdArgs = append(cmdArgs, args...)

	cmd := exec.Command("tmux", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux display-popup: %s: %w", string(out), err)
	}
	return nil
}

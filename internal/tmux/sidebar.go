package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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
func DetectState() LaunchState {
	if !IsInsideTmux() {
		sessions, _ := ListSessions()
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

// CreateSessionAndAttach creates a new tmux session at repoRoot with sidebar, then attaches.
func CreateSessionAndAttach(repoRoot string, sidebarWidth string, cfg config.Config) error {
	// Use the repo directory name as the session base name
	sessName := SessionNameFor("main")

	cmd := exec.Command("tmux", "new-session", "-d", "-s", sessName, "-c", repoRoot)
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

	cmd = exec.Command("tmux", "attach-session", "-t", sessName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// AttachFirstSession attaches to the first available synco session.
func AttachFirstSession() error {
	sessions, err := ListSessions()
	if err != nil || len(sessions) == 0 {
		return fmt.Errorf("no synco sessions found")
	}

	cmd := exec.Command("tmux", "attach-session", "-t", sessions[0].Name)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// AddSidebarToCurrent splits the current tmux session and adds a sidebar pane.
func AddSidebarToCurrent(repoRoot string, sidebarWidth string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find own binary: %w", err)
	}

	cmd := exec.Command("tmux", "split-window", "-fhb",
		"-l", sidebarWidth,
		binary, "--sidebar", "--root", repoRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %w", string(out), err)
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

	cmd := exec.Command("tmux", "split-window", "-fhb",
		"-l", width,
		"-t", session,
		binary, "--sidebar", "--root", repoRoot,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux split-window: %s: %w", string(out), err)
	}

	// Focus the right pane (the work area, not the sidebar)
	panes, err := listPanes(session)
	if err == nil && len(panes) >= 2 {
		_ = exec.Command("tmux", "select-pane", "-t", panes[1]).Run()
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
// The sidebar is always pane 0 (left), so the main pane is the last one.
func FocusMainPane(session string) {
	panes, err := listPanes(session)
	if err != nil || len(panes) < 2 {
		return
	}
	_ = exec.Command("tmux", "select-pane", "-t", panes[len(panes)-1]).Run()
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

package tmux

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
)

// Session represents a tmux session managed by synco.
type Session struct {
	Name     string
	Attached bool
}

var unsafeChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// RootSessionKey is the stable identifier used for the root worktree's tmux session.
// Using a constant instead of the branch name ensures navigation keeps working
// when the user switches branches on the root worktree.
const RootSessionKey = "root"

// ProjectName derives a sanitized project identifier from a repo root path.
// It resolves to the main working tree so that worktrees share the same
// project name as the root repo.
func ProjectName(repoRoot string) string {
	name := filepath.Base(mainWorktreeRoot(repoRoot))
	safe := unsafeChars.ReplaceAllString(name, "-")
	for strings.Contains(safe, "--") {
		safe = strings.ReplaceAll(safe, "--", "-")
	}
	return strings.Trim(safe, "-")
}

// mainWorktreeRoot returns the path of the main working tree for a repo.
// If repoRoot is already the main worktree (or detection fails), it returns repoRoot unchanged.
func mainWorktreeRoot(repoRoot string) string {
	cmd := exec.Command("git", "-C", repoRoot, "rev-parse", "--path-format=absolute", "--git-common-dir")
	out, err := cmd.Output()
	if err != nil {
		return repoRoot
	}
	gitCommonDir := strings.TrimSpace(string(out))
	// git-common-dir points to the .git directory of the main worktree.
	// The main worktree root is its parent.
	root := filepath.Dir(gitCommonDir)
	if root == "" || root == "." {
		return repoRoot
	}
	return root
}

// sanitize cleans a string for use in tmux session names.
func sanitize(s string) string {
	safe := unsafeChars.ReplaceAllString(s, "-")
	for strings.Contains(safe, "--") {
		safe = strings.ReplaceAll(safe, "--", "-")
	}
	return strings.Trim(safe, "-")
}

// SessionNameFor derives a tmux session name from a project name and branch.
func SessionNameFor(project, branch string) string {
	return project + "-" + sanitize(branch)
}

// ListSessions returns tmux sessions prefixed with the project name.
func ListSessions(project string) ([]Session, error) {
	prefix := project + "-"
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_attached}")
	out, err := cmd.Output()
	if err != nil {
		// tmux returns error when no server is running — that's fine
		if strings.Contains(err.Error(), "exit status") {
			return nil, nil
		}
		return nil, fmt.Errorf("tmux list-sessions: %w", err)
	}

	var sessions []Session
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		sessions = append(sessions, Session{
			Name:     name,
			Attached: parts[1] != "0",
		})
	}
	return sessions, nil
}

// NewSession creates a detached tmux session with the given name and working directory.
func NewSession(name, workdir string) error {
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workdir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux new-session: %s: %w", string(out), err)
	}
	return nil
}

// SendKeys sends a command string to a tmux session.
func SendKeys(session, keys string) error {
	cmd := exec.Command("tmux", "send-keys", "-t", session, keys, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux send-keys: %s: %w", string(out), err)
	}
	return nil
}

// KillSession kills a tmux session.
func KillSession(name string) error {
	cmd := exec.Command("tmux", "kill-session", "-t", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux kill-session: %s: %w", string(out), err)
	}
	return nil
}

// AttachSession replaces the current process with tmux attach.
func AttachSession(name string) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found: %w", err)
	}
	return syscall.Exec(tmuxPath, []string{"tmux", "attach-session", "-t", name}, os.Environ())
}

// IsInsideTmux returns true if we are currently inside a tmux session.
func IsInsideTmux() bool {
	return os.Getenv("TMUX") != ""
}

// CurrentSessionName returns the name of the tmux session we're currently in.
func CurrentSessionName() (string, error) {
	cmd := exec.Command("tmux", "display-message", "-p", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux display-message: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// SwitchClient switches the current tmux client to the given session.
// This is used when already inside tmux instead of attach.
func SwitchClient(name string) error {
	cmd := exec.Command("tmux", "switch-client", "-t", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tmux switch-client: %s: %w", string(out), err)
	}
	return nil
}

// SessionExists returns true if a tmux session with the given name exists.
func SessionExists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// CapturePaneOutput captures the last N lines of terminal output from a session's pane.
func CapturePaneOutput(session string, lines int) (string, error) {
	if lines <= 0 {
		lines = 50
	}
	cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p", "-S", fmt.Sprintf("-%d", lines))
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane: %w", err)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

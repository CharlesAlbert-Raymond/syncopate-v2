package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/tmux"
)

// hookInput is the JSON payload Claude Code sends to hook commands via stdin.
type hookInput struct {
	CWD string `json:"cwd"`
}

// RunNotify reads the Claude Code Stop hook JSON from stdin, maps the CWD
// to a synco session name, and writes a signal file. Exits silently if the
// CWD is not part of a synco-managed project.
func RunNotify(stdin io.Reader) error {
	var input hookInput
	if err := json.NewDecoder(stdin).Decode(&input); err != nil {
		return nil // not valid JSON — exit silently
	}
	if input.CWD == "" {
		return nil
	}

	sessionName, err := resolveSession(input.CWD)
	if err != nil {
		return nil // not a synco-managed repo — exit silently
	}

	if !tmux.SessionExists(sessionName) {
		return nil // no tmux session for this worktree
	}

	return WriteSignal(sessionName)
}

// resolveSession maps a working directory to a synco tmux session name.
func resolveSession(cwd string) (string, error) {
	// Resolve to absolute path
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}

	// Resolve project name from config if available, fall back to dir name
	mainRoot := tmux.MainWorktreeRoot(cwd)
	cfg, _ := config.Load(mainRoot)
	project := tmux.ResolveProjectName(cwd, cfg.ProjectName)
	if project == "" {
		return "", fmt.Errorf("not a git repo")
	}

	// Determine if this is the main worktree
	mainRoot, _ = filepath.Abs(mainRoot)

	var sessionKey string
	if filepath.Clean(cwd) == filepath.Clean(mainRoot) {
		sessionKey = tmux.RootSessionKey
	} else {
		branch, err := gitBranch(cwd)
		if err != nil {
			return "", err
		}
		sessionKey = branch
	}

	return tmux.SessionNameFor(project, sessionKey), nil
}

// gitBranch returns the current branch name for the given directory.
func gitBranch(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

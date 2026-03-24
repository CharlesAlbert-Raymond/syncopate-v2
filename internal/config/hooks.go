package config

import (
	"fmt"
	"os"
	"os/exec"
)

// RunHook executes a lifecycle script with worktree context as env vars.
// Returns nil if the hook is empty (not configured).
func RunHook(script, branch, worktreePath string) error {
	if script == "" {
		return nil
	}

	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = worktreePath
	cmd.Env = append(os.Environ(),
		"SYNCOPATE_BRANCH="+branch,
		"SYNCOPATE_WORKTREE_PATH="+worktreePath,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("hook failed: %w", err)
	}
	return nil
}

// RunHookInTmux sends the hook script to a tmux session instead of running inline.
// This is preferred so the user can see output in their session.
func RunHookInTmux(sessionName, script, branch, worktreePath string) error {
	if script == "" {
		return nil
	}

	// Wrap the script with env vars so it has context
	wrapped := fmt.Sprintf(
		"SYNCOPATE_BRANCH=%q SYNCOPATE_WORKTREE_PATH=%q %s",
		branch, worktreePath, script,
	)

	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, wrapped, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send hook to tmux: %s: %w", string(out), err)
	}
	return nil
}

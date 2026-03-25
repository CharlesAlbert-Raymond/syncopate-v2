package orchestrate

import (
	"fmt"
	"strings"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
	"github.com/charles-albert-raymond/synco/internal/worktree"
)

// CreateWorktree creates a git worktree, a tmux session, and runs the on_create hook.
func CreateWorktree(repoRoot string, cfg config.Config, branch, base string) (wtPath, sessName string, err error) {
	wtPath = cfg.WorktreePath(repoRoot, branch)

	if err := worktree.Add(repoRoot, wtPath, branch, true, base); err != nil {
		return "", "", fmt.Errorf("failed to create worktree: %w", err)
	}

	project := tmux.ProjectName(repoRoot)
	sessName = tmux.SessionNameFor(project, branch)
	if err := tmux.NewSession(sessName, wtPath); err != nil {
		return wtPath, "", fmt.Errorf("worktree created at %s but tmux session failed: %w", wtPath, err)
	}

	if err := config.RunHookInTmux(sessName, cfg.OnCreate, branch, wtPath); err != nil {
		return wtPath, sessName, fmt.Errorf("worktree and session created but on_create hook failed: %w", err)
	}

	return wtPath, sessName, nil
}

// CreateWorktreeFromExisting creates a worktree from an existing branch (local or remote).
// For remote branches like "origin/feature-x", it creates a local tracking branch "feature-x".
func CreateWorktreeFromExisting(repoRoot string, cfg config.Config, branch string) (wtPath, sessName string, err error) {
	// Strip remote prefix for the local branch name and worktree path
	localBranch := branch
	if idx := strings.Index(branch, "/"); idx != -1 && !strings.HasPrefix(branch, "refs/") {
		localBranch = branch[idx+1:]
	}

	wtPath = cfg.WorktreePath(repoRoot, localBranch)

	if err := worktree.Add(repoRoot, wtPath, branch, false, ""); err != nil {
		return "", "", fmt.Errorf("failed to create worktree: %w", err)
	}

	project := tmux.ProjectName(repoRoot)
	sessName = tmux.SessionNameFor(project, localBranch)
	if err := tmux.NewSession(sessName, wtPath); err != nil {
		return wtPath, "", fmt.Errorf("worktree created at %s but tmux session failed: %w", wtPath, err)
	}

	if err := config.RunHookInTmux(sessName, cfg.OnCreate, localBranch, wtPath); err != nil {
		return wtPath, sessName, fmt.Errorf("worktree and session created but on_create hook failed: %w", err)
	}

	return wtPath, sessName, nil
}

// DeleteWorktreeOpts controls delete behavior.
type DeleteWorktreeOpts struct {
	DeleteBranch bool
}

// DeleteWorktree removes a worktree, optionally deletes the branch, and kills the tmux session.
// It does NOT handle "deleting self" tmux switching — TUI callers handle that separately.
func DeleteWorktree(repoRoot string, cfg config.Config, entry state.Entry, opts DeleteWorktreeOpts) error {
	// Run on_destroy hook
	if err := config.RunHook(cfg.OnDestroy, entry.BranchShort, entry.Worktree.Path); err != nil {
		return fmt.Errorf("on_destroy hook failed: %w", err)
	}

	// Kill tmux session first for instant UI feedback
	if entry.HasSession {
		_ = tmux.KillSession(entry.SessionName)
	}

	// Fast-remove worktree: rename to trash dir, prune git metadata,
	// then delete files in the background
	if err := worktree.RemoveFast(repoRoot, entry.Worktree.Path); err != nil {
		return fmt.Errorf("failed to remove worktree: %w", err)
	}

	// Optionally delete branch
	if opts.DeleteBranch {
		if err := worktree.DeleteBranch(repoRoot, entry.BranchShort); err != nil {
			return fmt.Errorf("worktree removed but branch delete failed: %w", err)
		}
	}

	return nil
}

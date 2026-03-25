package worktree

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Worktree represents a single git worktree.
type Worktree struct {
	Path   string
	HEAD   string
	Branch string // short name, e.g. "feature-x"
	IsMain bool
}

// List returns all worktrees for the repo at repoRoot.
func List(repoRoot string) ([]Worktree, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git worktree list: %w", err)
	}
	return parsePorcelain(out, repoRoot), nil
}

// Add creates a new worktree at the given path for the given branch.
// If newBranch is true, it creates a new branch from startPoint (or HEAD).
func Add(repoRoot, path, branch string, newBranch bool, startPoint string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	args := []string{"worktree", "add"}
	if newBranch {
		args = append(args, "-b", branch, absPath)
		if startPoint != "" {
			args = append(args, startPoint)
		}
	} else {
		args = append(args, absPath, branch)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", string(out), err)
	}
	return nil
}

// Remove removes a worktree at the given path (blocking).
func Remove(repoRoot, path string) error {
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", string(out), err)
	}
	return nil
}

// RemoveFast removes a worktree without blocking on file deletion.
// It renames the directory to a trash path (instant on the same filesystem),
// prunes git worktree metadata, then deletes the trashed files in the background.
func RemoveFast(repoRoot, path string) error {
	trashPath := path + fmt.Sprintf(".synco-trash-%d", time.Now().UnixNano())

	if err := os.Rename(path, trashPath); err != nil {
		// Fall back to the blocking path if rename fails (e.g. cross-device)
		return Remove(repoRoot, path)
	}

	// Tell git the worktree is gone
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try to undo the rename so state isn't broken
		_ = os.Rename(trashPath, path)
		return fmt.Errorf("git worktree prune: %s: %w", string(out), err)
	}

	// Delete trashed files in the background
	go func() {
		_ = os.RemoveAll(trashPath)
	}()

	return nil
}

// DeleteBranch deletes a local git branch.
func DeleteBranch(repoRoot, branch string) error {
	cmd := exec.Command("git", "branch", "-D", branch)
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch -D: %s: %w", string(out), err)
	}
	return nil
}

// BranchList returns local branch names for the repo.
func BranchList(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "branch", "--format=%(refname:short)")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}
	var branches []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		b := strings.TrimSpace(scanner.Text())
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// RemoteBranchList returns remote branch names (e.g. "origin/feature-x") for the repo.
func RemoteBranchList(repoRoot string) ([]string, error) {
	cmd := exec.Command("git", "branch", "-r", "--format=%(refname:short)")
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch -r: %w", err)
	}
	var branches []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		b := strings.TrimSpace(scanner.Text())
		if b == "" || strings.Contains(b, "HEAD") {
			continue
		}
		branches = append(branches, b)
	}
	return branches, nil
}

// Fetch runs git fetch --prune to update remote refs.
func Fetch(repoRoot string) error {
	cmd := exec.Command("git", "fetch", "--prune")
	cmd.Dir = repoRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch: %s: %w", string(out), err)
	}
	return nil
}

func parsePorcelain(data []byte, repoRoot string) []Worktree {
	var worktrees []Worktree
	var current Worktree
	isMain := true // first entry is always the main worktree

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			if current.Path != "" {
				current.IsMain = isMain
				worktrees = append(worktrees, current)
				current = Worktree{}
				isMain = false
			}
			continue
		}

		if strings.HasPrefix(line, "worktree ") {
			current.Path = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "HEAD ") {
			current.HEAD = strings.TrimPrefix(line, "HEAD ")
		} else if strings.HasPrefix(line, "branch ") {
			ref := strings.TrimPrefix(line, "branch ")
			// Convert refs/heads/feature-x -> feature-x
			current.Branch = strings.TrimPrefix(ref, "refs/heads/")
		} else if line == "detached" {
			current.Branch = "(detached)"
		}
	}

	// Handle last entry if no trailing newline
	if current.Path != "" {
		current.IsMain = isMain
		worktrees = append(worktrees, current)
	}

	return worktrees
}

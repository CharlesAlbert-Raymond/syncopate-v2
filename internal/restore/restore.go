package restore

import (
	"fmt"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
)

// Result holds what happened during a restore operation.
type Result struct {
	Restored []string // session names that were created
	Skipped  []string // session names that already existed
	Errors   []error  // errors encountered (non-fatal, per-session)
}

// OrphanedWorktrees returns entries that have a worktree on disk but no tmux session.
func OrphanedWorktrees(entries []state.Entry) []state.Entry {
	var orphans []state.Entry
	for _, e := range entries {
		if !e.HasSession {
			orphans = append(orphans, e)
		}
	}
	return orphans
}

// Run restores tmux sessions for worktrees that are missing them.
// If runHooks is true, the on_create hook from config is executed in each restored session.
func Run(repoRoot string, cfg config.Config, runHooks bool) (*Result, error) {
	gathered, err := state.Gather(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("gather state: %w", err)
	}

	return RestoreEntries(gathered.Entries, cfg, runHooks)
}

// RestoreEntries restores sessions for the given entries.
// Separated from Run so it can be tested with controlled input.
func RestoreEntries(entries []state.Entry, cfg config.Config, runHooks bool) (*Result, error) {
	res := &Result{}

	for _, entry := range entries {
		if entry.HasSession {
			res.Skipped = append(res.Skipped, entry.SessionName)
			continue
		}

		if err := tmux.NewSession(entry.SessionName, entry.Worktree.Path); err != nil {
			res.Errors = append(res.Errors, fmt.Errorf("restore %s: %w", entry.SessionName, err))
			continue
		}

		if runHooks && cfg.OnCreate != "" {
			if err := config.RunHookInTmux(entry.SessionName, cfg.OnCreate, entry.BranchShort, entry.Worktree.Path); err != nil {
				res.Errors = append(res.Errors, fmt.Errorf("hook for %s: %w", entry.SessionName, err))
			}
		}

		res.Restored = append(res.Restored, entry.SessionName)
	}

	return res, nil
}

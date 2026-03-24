package state

import (
	"github.com/charles-albert-raymond/syncopate/internal/tmux"
	"github.com/charles-albert-raymond/syncopate/internal/worktree"
)

// Entry is the reconciled view of a worktree and its optional tmux session.
type Entry struct {
	Worktree    worktree.Worktree
	BranchShort string
	SessionName string
	TmuxSession *tmux.Session
	HasSession  bool
	IsCurrent   bool // true if this is the worktree whose tmux session we're in
}

// Gather produces the full list of entries by joining worktrees with tmux sessions.
func Gather(repoRoot string) ([]Entry, error) {
	wts, err := worktree.List(repoRoot)
	if err != nil {
		return nil, err
	}

	sessions, err := tmux.ListSyncopateSessions()
	if err != nil {
		return nil, err
	}

	sessionMap := make(map[string]*tmux.Session, len(sessions))
	for i := range sessions {
		sessionMap[sessions[i].Name] = &sessions[i]
	}

	// Detect which tmux session we're currently in
	currentSession, _ := tmux.CurrentSessionName()

	entries := make([]Entry, 0, len(wts))
	for _, wt := range wts {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}

		sessName := tmux.SessionNameFor(branch)
		sess := sessionMap[sessName]

		entries = append(entries, Entry{
			Worktree:    wt,
			BranchShort: branch,
			SessionName: sessName,
			TmuxSession: sess,
			HasSession:  sess != nil,
			IsCurrent:   sessName == currentSession,
		})
	}

	return entries, nil
}

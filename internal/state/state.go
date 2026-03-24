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
	Ports       []int // TCP ports being listened on in this session
}

// SessionPorts holds port info for a non-syncopate tmux session.
type SessionPorts struct {
	Name  string
	Ports []int
}

// GatherResult contains entries and port info for other tmux sessions.
type GatherResult struct {
	Entries       []Entry
	OtherPorts    []SessionPorts // non-syncopate sessions with listening ports
}

// Gather produces the full list of entries by joining worktrees with tmux sessions,
// plus port info for all tmux sessions.
func Gather(repoRoot string) (*GatherResult, error) {
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

	// Batch-fetch listening ports for all tmux sessions at once
	portsBySession := tmux.PortsBySession()

	// Track which sessions are syncopate-managed
	syncopateSessions := make(map[string]bool)

	entries := make([]Entry, 0, len(wts))
	for _, wt := range wts {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}

		sessName := tmux.SessionNameFor(branch)
		sess := sessionMap[sessName]
		syncopateSessions[sessName] = true

		entries = append(entries, Entry{
			Worktree:    wt,
			BranchShort: branch,
			SessionName: sessName,
			TmuxSession: sess,
			HasSession:  sess != nil,
			Ports:       portsBySession[sessName],
		})
	}

	// Collect ports from non-syncopate sessions
	var otherPorts []SessionPorts
	for sess, ports := range portsBySession {
		if !syncopateSessions[sess] && len(ports) > 0 {
			otherPorts = append(otherPorts, SessionPorts{Name: sess, Ports: ports})
		}
	}

	return &GatherResult{
		Entries:    entries,
		OtherPorts: otherPorts,
	}, nil
}

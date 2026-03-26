package state

import (
	"github.com/charles-albert-raymond/synco/internal/tmux"
	"github.com/charles-albert-raymond/synco/internal/worktree"
)

// Entry is the reconciled view of a worktree and its optional tmux session.
type Entry struct {
	Worktree    worktree.Worktree
	BranchShort string
	SessionName string
	TmuxSession *tmux.Session
	HasSession  bool
	Ports       []int // TCP ports being listened on in this session
	IsCurrent   bool  // true if this is the worktree whose tmux session we're in
	IdleSeconds int   // seconds since last pane output, -1 if no session
}

// SessionPorts holds port info for a non-synco tmux session.
type SessionPorts struct {
	Name  string
	Ports []int
}

// GatherResult contains entries and port info for other tmux sessions.
type GatherResult struct {
	Entries       []Entry
	OtherPorts    []SessionPorts // non-synco sessions with listening ports
}

// Gather produces the full list of entries by joining worktrees with tmux sessions,
// plus port info for all tmux sessions.
func Gather(repoRoot string) (*GatherResult, error) {
	project := tmux.ProjectName(repoRoot)

	wts, err := worktree.List(repoRoot)
	if err != nil {
		return nil, err
	}

	sessions, err := tmux.ListSessions(project)
	if err != nil {
		return nil, err
	}

	sessionMap := make(map[string]*tmux.Session, len(sessions))
	for i := range sessions {
		sessionMap[sessions[i].Name] = &sessions[i]
	}

	// Batch-fetch listening ports and activity for all tmux sessions at once
	portsBySession := tmux.PortsBySession()
	activityBySession := tmux.ActivityBySession(project)

	// Track which sessions are synco-managed
	syncoSessions := make(map[string]bool)

	// Detect which tmux session we're currently in
	currentSession, _ := tmux.CurrentSessionName()

	entries := make([]Entry, 0, len(wts))
	for _, wt := range wts {
		branch := wt.Branch
		if branch == "" {
			branch = "(detached)"
		}

		// Use a stable key for the root worktree so its session name
		// doesn't change when the user switches branches on the root.
		sessionKey := branch
		if wt.IsMain {
			sessionKey = tmux.RootSessionKey
		}
		sessName := tmux.SessionNameFor(project, sessionKey)
		sess := sessionMap[sessName]
		syncoSessions[sessName] = true

		idle := -1
		if sess != nil {
			if v, ok := activityBySession[sessName]; ok {
				idle = v
			}
		}

		entries = append(entries, Entry{
			Worktree:    wt,
			BranchShort: branch,
			SessionName: sessName,
			TmuxSession: sess,
			HasSession:  sess != nil,
			Ports:       portsBySession[sessName],
			IsCurrent:   sessName == currentSession,
			IdleSeconds: idle,
		})
	}

	// Collect ports from non-synco sessions
	var otherPorts []SessionPorts
	for sess, ports := range portsBySession {
		if !syncoSessions[sess] && len(ports) > 0 {
			otherPorts = append(otherPorts, SessionPorts{Name: sess, Ports: ports})
		}
	}

	return &GatherResult{
		Entries:    entries,
		OtherPorts: otherPorts,
	}, nil
}

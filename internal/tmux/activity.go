package tmux

import (
	"bufio"
	"bytes"
	"hash/fnv"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// activityTracker detects pane-level silence by comparing captured pane content
// between polling intervals. This avoids relying on #{window_activity} which is
// window-scoped and includes sidebar pane output, making it useless for
// detecting idle agents.
type activityTracker struct {
	mu         sync.Mutex
	hashes     map[string]uint64    // pane_id -> content hash
	lastChange map[string]time.Time // pane_id -> last content change time
}

var tracker = &activityTracker{
	hashes:     make(map[string]uint64),
	lastChange: make(map[string]time.Time),
}

// ActivityBySession returns a map of session name -> seconds since last pane output
// for synco-managed sessions (those matching the given project prefix).
// Panes whose start command contains "--sidebar" are excluded so the sidebar
// pane doesn't skew results. For sessions with multiple panes, the minimum
// idle time (most recent activity) is returned.
func ActivityBySession(project string) map[string]int {
	return tracker.idleBySession(project)
}

func (t *activityTracker) idleBySession(project string) map[string]int {
	prefix := project + "-"
	cmd := exec.Command("tmux", "list-panes", "-a", "-F",
		"#{session_name}\t#{pane_id}\t#{pane_start_command}")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()

	alive := make(map[string]bool)
	minIdle := make(map[string]int)
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 3)
		if len(parts) < 3 {
			continue
		}
		session := parts[0]
		paneID := parts[1]

		if !strings.HasPrefix(session, prefix) {
			continue
		}
		// Skip sidebar panes
		if strings.Contains(parts[2], "--sidebar") {
			continue
		}

		alive[paneID] = true

		// Capture current visible content and hash it
		content := capturePane(paneID)
		h := hashBytes(content)

		prev, exists := t.hashes[paneID]
		if !exists || prev != h {
			t.hashes[paneID] = h
			t.lastChange[paneID] = now
		}

		idle := int(now.Sub(t.lastChange[paneID]).Seconds())
		if idle < 0 {
			idle = 0
		}

		if !seen[session] || idle < minIdle[session] {
			minIdle[session] = idle
			seen[session] = true
		}
	}

	// Clean up stale panes
	for id := range t.hashes {
		if !alive[id] {
			delete(t.hashes, id)
			delete(t.lastChange, id)
		}
	}

	return minIdle
}

func capturePane(paneID string) []byte {
	cmd := exec.Command("tmux", "capture-pane", "-t", paneID, "-p")
	out, _ := cmd.Output()
	return out
}

func hashBytes(b []byte) uint64 {
	h := fnv.New64a()
	h.Write(b)
	return h.Sum64()
}

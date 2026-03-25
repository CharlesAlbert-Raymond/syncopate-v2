package restore

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
	wt "github.com/charles-albert-raymond/synco/internal/worktree"
)

// ============================================================
// Unit tests — no tmux or git required
// ============================================================

func TestOrphanedWorktrees_FiltersCorrectly(t *testing.T) {
	entries := []state.Entry{
		{BranchShort: "main", SessionName: "synco-main", HasSession: true},
		{BranchShort: "feat-a", SessionName: "synco-feat-a", HasSession: false},
		{BranchShort: "feat-b", SessionName: "synco-feat-b", HasSession: true},
		{BranchShort: "feat-c", SessionName: "synco-feat-c", HasSession: false},
	}

	orphans := OrphanedWorktrees(entries)
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(orphans))
	}
	if orphans[0].BranchShort != "feat-a" {
		t.Errorf("expected first orphan to be feat-a, got %s", orphans[0].BranchShort)
	}
	if orphans[1].BranchShort != "feat-c" {
		t.Errorf("expected second orphan to be feat-c, got %s", orphans[1].BranchShort)
	}
}

func TestOrphanedWorktrees_AllHaveSessions(t *testing.T) {
	entries := []state.Entry{
		{BranchShort: "main", HasSession: true},
		{BranchShort: "feat-a", HasSession: true},
	}

	orphans := OrphanedWorktrees(entries)
	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(orphans))
	}
}

func TestOrphanedWorktrees_NoneHaveSessions(t *testing.T) {
	entries := []state.Entry{
		{BranchShort: "feat-a", HasSession: false},
		{BranchShort: "feat-b", HasSession: false},
	}

	orphans := OrphanedWorktrees(entries)
	if len(orphans) != 2 {
		t.Fatalf("expected 2 orphans, got %d", len(orphans))
	}
}

func TestOrphanedWorktrees_Empty(t *testing.T) {
	orphans := OrphanedWorktrees(nil)
	if len(orphans) != 0 {
		t.Fatalf("expected 0 orphans, got %d", len(orphans))
	}
}

func TestRestoreEntries_SkipsExistingSessions(t *testing.T) {
	// This test does NOT create real tmux sessions — it just passes entries
	// marked as HasSession:true and verifies they end up in Skipped.
	entries := []state.Entry{
		{BranchShort: "main", SessionName: "synco-main", HasSession: true},
		{BranchShort: "feat-a", SessionName: "synco-feat-a", HasSession: true},
	}

	res, err := RestoreEntries(entries, config.Config{}, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Restored) != 0 {
		t.Errorf("expected 0 restored, got %d", len(res.Restored))
	}
	if len(res.Skipped) != 2 {
		t.Errorf("expected 2 skipped, got %d", len(res.Skipped))
	}
}

// ============================================================
// Integration tests — require a running tmux server and
// run against the real git repo you're working in.
//
// Run with:  go test -v -run Integration ./internal/restore/
//
// These tests create temporary worktrees + tmux sessions and
// clean them up afterwards. They will NOT touch your existing
// sessions or worktrees.
// ============================================================

const testBranchPrefix = "synco-test-restore-"

func skipIfNoTmux(t *testing.T) {
	t.Helper()
	out, err := exec.Command("tmux", "list-sessions").CombinedOutput()
	// tmux returns exit 1 when no server — that's fine, it means tmux exists
	if err != nil && !strings.Contains(string(out), "no server") && !strings.Contains(err.Error(), "exit status") {
		t.Skip("tmux not available, skipping integration test")
	}
}

func gitRepoRoot(t *testing.T) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("not in a git repo: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func testBranchName(suffix string) string {
	return testBranchPrefix + suffix
}

func createTestWorktree(t *testing.T, repoRoot, suffix string) (branch, worktreePath string) {
	t.Helper()
	branch = testBranchName(suffix)
	worktreePath = filepath.Join(repoRoot, ".worktrees", branch)

	err := wt.Add(repoRoot, worktreePath, branch, true, "HEAD")
	if err != nil {
		t.Fatalf("failed to create test worktree %s: %v", branch, err)
	}
	return branch, worktreePath
}

func cleanupTestWorktree(t *testing.T, repoRoot, branch, worktreePath string) {
	t.Helper()
	_ = wt.Remove(repoRoot, worktreePath)
	_ = wt.DeleteBranch(repoRoot, branch)
}

func cleanupTestSession(t *testing.T, sessionName string) {
	t.Helper()
	_ = tmux.KillSession(sessionName)
}

func sessionExists(sessionName string) bool {
	sessions, err := tmux.ListSessions()
	if err != nil {
		return false
	}
	for _, s := range sessions {
		if s.Name == sessionName {
			return true
		}
	}
	return false
}

// TestIntegration_RestoreCreatesSessionForOrphanedWorktree verifies that
// a worktree without a tmux session gets a new session created by restore.
func TestIntegration_RestoreCreatesSessionForOrphanedWorktree(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)

	branch, wtPath := createTestWorktree(t, repoRoot, "orphan1")
	sessName := tmux.SessionNameFor(branch)
	defer cleanupTestWorktree(t, repoRoot, branch, wtPath)
	defer cleanupTestSession(t, sessName)

	// Sanity: no session should exist yet for this worktree
	if sessionExists(sessName) {
		t.Fatalf("session %s should not exist before restore", sessName)
	}

	// Build a fake entry mimicking what state.Gather would produce
	entry := state.Entry{
		Worktree:    wt.Worktree{Path: wtPath, Branch: branch},
		BranchShort: branch,
		SessionName: sessName,
		HasSession:  false,
	}

	res, err := RestoreEntries([]state.Entry{entry}, config.Config{}, false)
	if err != nil {
		t.Fatalf("RestoreEntries failed: %v", err)
	}

	if len(res.Restored) != 1 {
		t.Fatalf("expected 1 restored, got %d", len(res.Restored))
	}
	if res.Restored[0] != sessName {
		t.Errorf("expected restored session %s, got %s", sessName, res.Restored[0])
	}

	// Verify the tmux session actually exists now
	if !sessionExists(sessName) {
		t.Errorf("session %s should exist after restore", sessName)
	}
}

// TestIntegration_RestoreSkipsExistingSession verifies that restore does NOT
// create a duplicate session when one already exists.
func TestIntegration_RestoreSkipsExistingSession(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)

	branch, wtPath := createTestWorktree(t, repoRoot, "existing1")
	sessName := tmux.SessionNameFor(branch)
	defer cleanupTestWorktree(t, repoRoot, branch, wtPath)
	defer cleanupTestSession(t, sessName)

	// Manually create the session first
	if err := tmux.NewSession(sessName, wtPath); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	entry := state.Entry{
		Worktree:    wt.Worktree{Path: wtPath, Branch: branch},
		BranchShort: branch,
		SessionName: sessName,
		HasSession:  true, // already has a session
	}

	res, err := RestoreEntries([]state.Entry{entry}, config.Config{}, false)
	if err != nil {
		t.Fatalf("RestoreEntries failed: %v", err)
	}

	if len(res.Restored) != 0 {
		t.Errorf("expected 0 restored, got %d", len(res.Restored))
	}
	if len(res.Skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(res.Skipped))
	}
}

// TestIntegration_RestoreRunsOnCreateHook verifies that the on_create hook
// is executed inside the restored tmux session.
func TestIntegration_RestoreRunsOnCreateHook(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)

	branch, wtPath := createTestWorktree(t, repoRoot, "hook1")
	sessName := tmux.SessionNameFor(branch)
	defer cleanupTestWorktree(t, repoRoot, branch, wtPath)
	defer cleanupTestSession(t, sessName)

	// Create a marker file path the hook will write to
	markerFile := filepath.Join(os.TempDir(), "synco-test-hook-"+branch)
	defer os.Remove(markerFile)

	cfg := config.Config{
		OnCreate: "touch " + markerFile,
	}

	entry := state.Entry{
		Worktree:    wt.Worktree{Path: wtPath, Branch: branch},
		BranchShort: branch,
		SessionName: sessName,
		HasSession:  false,
	}

	res, err := RestoreEntries([]state.Entry{entry}, cfg, true)
	if err != nil {
		t.Fatalf("RestoreEntries failed: %v", err)
	}
	if len(res.Restored) != 1 {
		t.Fatalf("expected 1 restored, got %d", len(res.Restored))
	}

	// The hook runs inside tmux via send-keys. The shell in the new session
	// needs time to initialize before it processes the command, so poll
	// for the marker file with a generous timeout.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(markerFile); err == nil {
			return // success
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Errorf("on_create hook did not run: marker file %s not found after 5s", markerFile)
}

// TestIntegration_RestoreNoHooksWhenDisabled verifies that hooks are NOT
// run when runHooks=false.
func TestIntegration_RestoreNoHooksWhenDisabled(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)

	branch, wtPath := createTestWorktree(t, repoRoot, "nohook1")
	sessName := tmux.SessionNameFor(branch)
	defer cleanupTestWorktree(t, repoRoot, branch, wtPath)
	defer cleanupTestSession(t, sessName)

	markerFile := filepath.Join(os.TempDir(), "synco-test-nohook-"+branch)
	defer os.Remove(markerFile)

	cfg := config.Config{
		OnCreate: "touch " + markerFile,
	}

	entry := state.Entry{
		Worktree:    wt.Worktree{Path: wtPath, Branch: branch},
		BranchShort: branch,
		SessionName: sessName,
		HasSession:  false,
	}

	_, err := RestoreEntries([]state.Entry{entry}, cfg, false)
	if err != nil {
		t.Fatalf("RestoreEntries failed: %v", err)
	}

	time.Sleep(500 * time.Millisecond)

	if _, err := os.Stat(markerFile); err == nil {
		t.Errorf("hook should NOT have run, but marker file %s exists", markerFile)
	}
}

// TestIntegration_RunFullRestore exercises the full Run() path using
// the real repo state. It creates a worktree, ensures there's no session,
// then calls Run() and verifies the session gets created.
func TestIntegration_RunFullRestore(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)
	cfg, _ := config.Load(repoRoot)

	branch, wtPath := createTestWorktree(t, repoRoot, "full1")
	sessName := tmux.SessionNameFor(branch)
	defer cleanupTestWorktree(t, repoRoot, branch, wtPath)
	defer cleanupTestSession(t, sessName)

	// Ensure no session exists
	_ = tmux.KillSession(sessName)

	// Run the full restore — this calls state.Gather internally
	res, err := Run(repoRoot, cfg, false)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Our test worktree should appear in the restored list
	found := false
	for _, name := range res.Restored {
		if name == sessName {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected %s in restored list, got: %v", sessName, res.Restored)
	}

	if !sessionExists(sessName) {
		t.Errorf("session %s should exist after full restore", sessName)
	}
}

// TestIntegration_RestoreMultipleOrphans verifies that multiple orphaned
// worktrees are all restored in a single call.
func TestIntegration_RestoreMultipleOrphans(t *testing.T) {
	skipIfNoTmux(t)
	repoRoot := gitRepoRoot(t)

	type testWT struct {
		branch, path, session string
	}

	var created []testWT
	for _, suffix := range []string{"multi1", "multi2", "multi3"} {
		branch, wtPath := createTestWorktree(t, repoRoot, suffix)
		sessName := tmux.SessionNameFor(branch)
		created = append(created, testWT{branch, wtPath, sessName})
	}

	defer func() {
		for _, c := range created {
			cleanupTestSession(t, c.session)
			cleanupTestWorktree(t, repoRoot, c.branch, c.path)
		}
	}()

	var entries []state.Entry
	for _, c := range created {
		entries = append(entries, state.Entry{
			Worktree:    wt.Worktree{Path: c.path, Branch: c.branch},
			BranchShort: c.branch,
			SessionName: c.session,
			HasSession:  false,
		})
	}

	res, err := RestoreEntries(entries, config.Config{}, false)
	if err != nil {
		t.Fatalf("RestoreEntries failed: %v", err)
	}

	if len(res.Restored) != 3 {
		t.Fatalf("expected 3 restored, got %d: %v", len(res.Restored), res.Restored)
	}

	for _, c := range created {
		if !sessionExists(c.session) {
			t.Errorf("session %s should exist after restore", c.session)
		}
	}
}

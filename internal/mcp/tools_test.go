package mcp

import (
	"testing"

	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/worktree"
)

func TestFindEntry(t *testing.T) {
	entries := []state.Entry{
		{BranchShort: "main", Worktree: worktree.Worktree{IsMain: true}},
		{BranchShort: "feature/auth", Worktree: worktree.Worktree{Path: "/wt/auth"}},
		{BranchShort: "fix/bug-123", Worktree: worktree.Worktree{Path: "/wt/bug"}},
	}

	// Found
	e, ok := findEntry(entries, "feature/auth")
	if !ok {
		t.Fatal("expected to find feature/auth")
	}
	if e.Worktree.Path != "/wt/auth" {
		t.Errorf("Path = %q, want /wt/auth", e.Worktree.Path)
	}

	// Not found
	_, ok = findEntry(entries, "nonexistent")
	if ok {
		t.Error("expected not to find nonexistent branch")
	}

	// Empty slice
	_, ok = findEntry(nil, "main")
	if ok {
		t.Error("expected not to find in nil slice")
	}
}

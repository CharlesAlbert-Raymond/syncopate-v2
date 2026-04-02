package state

import (
	"testing"

	"github.com/charles-albert-raymond/synco/internal/worktree"
)

func TestFindEntry(t *testing.T) {
	entries := []Entry{
		{
			Worktree:    worktree.Worktree{Path: "/repo", Branch: "fix/bar", IsMain: true},
			BranchShort: "fix/bar",
			SessionName: "myproject", // root session = just the project name
		},
		{
			Worktree:    worktree.Worktree{Path: "/repo/.worktrees/feat-x", Branch: "feat/x", IsMain: false},
			BranchShort: "feat/x",
			SessionName: "myproject/feat-x",
		},
	}

	tests := []struct {
		name    string
		branch  string
		wantOK  bool
		wantSes string
	}{
		{"root keyword returns main worktree", "root", true, "myproject"},
		{"current root branch matches", "fix/bar", true, "myproject"},
		{"linked worktree branch matches", "feat/x", true, "myproject/feat-x"},
		{"unknown branch returns not found", "nonexistent", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := FindEntry(entries, tt.branch)
			if ok != tt.wantOK {
				t.Fatalf("FindEntry(%q) ok = %v, want %v", tt.branch, ok, tt.wantOK)
			}
			if ok && entry.SessionName != tt.wantSes {
				t.Errorf("FindEntry(%q).SessionName = %q, want %q", tt.branch, entry.SessionName, tt.wantSes)
			}
		})
	}
}

func TestFindEntryRootStableAfterBranchChange(t *testing.T) {
	// Simulate the root worktree changing branches: "root" still finds it.
	entries := []Entry{
		{
			Worktree:    worktree.Worktree{Path: "/repo", Branch: "main", IsMain: true},
			BranchShort: "main",
			SessionName: "proj", // root = just project name
		},
	}

	// Initially on "main"
	e, ok := FindEntry(entries, "root")
	if !ok || e.BranchShort != "main" {
		t.Fatalf("expected root to find main, got ok=%v branch=%q", ok, e.BranchShort)
	}

	// Simulate branch change
	entries[0].Worktree.Branch = "fix/something"
	entries[0].BranchShort = "fix/something"

	e, ok = FindEntry(entries, "root")
	if !ok || e.BranchShort != "fix/something" {
		t.Fatalf("after branch change: expected root to find fix/something, got ok=%v branch=%q", ok, e.BranchShort)
	}

	// Old branch name no longer matches
	_, ok = FindEntry(entries, "main")
	if ok {
		t.Fatal("expected 'main' to no longer match after branch change")
	}
}

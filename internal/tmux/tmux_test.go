package tmux

import "testing"

func TestSessionNameFor(t *testing.T) {
	tests := []struct {
		branch string
		want   string
	}{
		{"main", "synco-main"},
		{"feature-x", "synco-feature-x"},
		{"feature/auth-refactor", "synco-feature-auth-refactor"},
		{"feat/add-mcp-for-syncopate", "synco-feat-add-mcp-for-syncopate"},
		{"my.branch.name", "synco-my-branch-name"},
		{"a//b", "synco-a-b"},
		{"---dashes---", "synco-dashes"},
		{"", "synco-"},
		{"simple", "synco-simple"},
		{"UPPER_case", "synco-UPPER_case"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := SessionNameFor(tt.branch)
			if got != tt.want {
				t.Errorf("SessionNameFor(%q) = %q, want %q", tt.branch, got, tt.want)
			}
		})
	}
}

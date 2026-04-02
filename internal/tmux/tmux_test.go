package tmux

import "testing"

func TestSessionNameFor(t *testing.T) {
	project := "myproject"
	tests := []struct {
		branch string
		want   string
	}{
		// Root session is just the project name (sorts first in choose-tree)
		{RootSessionKey, "myproject"},
		// Branch sessions use project/branch format
		{"main", "myproject/main"},
		{"feature-x", "myproject/feature-x"},
		{"feature/auth-refactor", "myproject/feature-auth-refactor"},
		{"feat/add-mcp-for-syncopate", "myproject/feat-add-mcp-for-syncopate"},
		{"my.branch.name", "myproject/my-branch-name"},
		{"a//b", "myproject/a-b"},
		{"---dashes---", "myproject/dashes"},
		{"", "myproject/"},
		{"simple", "myproject/simple"},
		{"UPPER_case", "myproject/UPPER_case"},
	}

	for _, tt := range tests {
		t.Run(tt.branch, func(t *testing.T) {
			got := SessionNameFor(project, tt.branch)
			if got != tt.want {
				t.Errorf("SessionNameFor(%q, %q) = %q, want %q", project, tt.branch, got, tt.want)
			}
		})
	}
}

func TestIsProjectSession(t *testing.T) {
	tests := []struct {
		name    string
		session string
		project string
		want    bool
	}{
		{"root session", "synco", "synco", true},
		{"branch session", "synco/feat-auth", "synco", true},
		{"unrelated session", "other-app", "synco", false},
		{"partial prefix match is not enough", "synco-old-format", "synco", false},
		{"different project with same prefix", "syncopate/root", "synco", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsProjectSession(tt.session, tt.project)
			if got != tt.want {
				t.Errorf("IsProjectSession(%q, %q) = %v, want %v", tt.session, tt.project, got, tt.want)
			}
		})
	}
}

func TestProjectName(t *testing.T) {
	tests := []struct {
		repoRoot string
		want     string
	}{
		{"/home/user/projects/my-app", "my-app"},
		{"/home/user/projects/My.Project", "My-Project"},
		{"/home/user/projects/simple", "simple"},
	}

	for _, tt := range tests {
		t.Run(tt.repoRoot, func(t *testing.T) {
			got := ProjectName(tt.repoRoot)
			if got != tt.want {
				t.Errorf("ProjectName(%q) = %q, want %q", tt.repoRoot, got, tt.want)
			}
		})
	}
}

func TestResolveProjectName(t *testing.T) {
	tests := []struct {
		name        string
		repoRoot    string
		configLabel string
		want        string
	}{
		{"uses config label when set", "/home/user/my-app", "my-custom-name", "my-custom-name"},
		{"falls back to dir name", "/home/user/my-app", "", "my-app"},
		{"sanitizes config label", "/home/user/my-app", "My.Project", "My-Project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveProjectName(tt.repoRoot, tt.configLabel)
			if got != tt.want {
				t.Errorf("ResolveProjectName(%q, %q) = %q, want %q", tt.repoRoot, tt.configLabel, got, tt.want)
			}
		})
	}
}

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	"github.com/charles-albert-raymond/syncopate/internal/tui"
)

func main() {
	repoRoot, err := findGitRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "syncopate must be run from within a git repository.\n")
		os.Exit(1)
	}

	cfg, err := config.Load(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load .syncopate.yaml: %v\n", err)
		cfg = config.Config{WorktreeDir: ".."}
	}

	m := tui.NewModel(repoRoot, cfg)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

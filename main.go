package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	syncmcp "github.com/charles-albert-raymond/syncopate/internal/mcp"
	"github.com/charles-albert-raymond/syncopate/internal/tmux"
	"github.com/charles-albert-raymond/syncopate/internal/tui"
)

func main() {
	// Handle "mcp" subcommand before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		runMCP()
		return
	}

	sidebarFlag := flag.Bool("sidebar", false, "run in compact sidebar mode (used internally)")
	classicFlag := flag.Bool("classic", false, "run the original full-screen TUI")
	rootFlag := flag.String("root", "", "repo root path (used internally by sidebar)")
	flag.Parse()

	repoRoot := resolveRepoRoot(*rootFlag)
	cfg := loadConfig(repoRoot)

	switch {
	case *sidebarFlag:
		// Internal: run the compact sidebar TUI
		m := tui.NewSidebarModel(repoRoot, cfg)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case *classicFlag:
		// Original full-screen TUI
		m := tui.NewModel(repoRoot, cfg)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	default:
		launch(repoRoot)
	}
}

func launch(repoRoot string) {
	state := tmux.DetectState()

	switch state {
	case tmux.OutsideNoSession:
		// Not in tmux, nothing exists — create session with sidebar, attach
		if err := tmux.CreateSessionAndAttach(repoRoot, 28); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.OutsideHasSession:
		// Not in tmux, sessions exist — reconnect to the first one
		fmt.Println("Reconnecting to existing syncopate session...")
		if err := tmux.AttachFirstSession(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.InsideNoSidebar:
		// In tmux but no sidebar — add it to the current session
		if err := tmux.AddSidebarToCurrent(repoRoot, 28); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.InsideHasSidebar:
		// Sidebar already running in this session
		fmt.Println("syncopate is already running in this session.")
		fmt.Println()
		fmt.Println("  Tip: focus the sidebar pane with Ctrl-b + ←")
		fmt.Println("  Or run 'syncopate --classic' for full-screen mode.")
	}
}

func runMCP() {
	rootFlag := flag.String("root", "", "repo root path")
	flag.Parse()

	repoRoot := resolveRepoRoot(*rootFlag)
	cfg := loadConfig(repoRoot)

	if err := syncmcp.Serve(repoRoot, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func resolveRepoRoot(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	root, err := findGitRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "syncopate must be run from within a git repository.\n")
		os.Exit(1)
	}
	return root
}

func loadConfig(repoRoot string) config.Config {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load .syncopate.yaml: %v\n", err)
		cfg = config.Config{WorktreeDir: ".."}
	}
	return cfg
}

func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	return strings.TrimSpace(string(out)), nil
}

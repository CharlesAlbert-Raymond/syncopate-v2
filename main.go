package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charles-albert-raymond/synco/internal/config"
	syncmcp "github.com/charles-albert-raymond/synco/internal/mcp"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
	"github.com/charles-albert-raymond/synco/internal/tui"
)

func main() {
	// Handle "mcp" subcommand before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		runMCP(os.Args[2:])
		return
	}

	sidebarFlag := flag.Bool("sidebar", false, "run in compact sidebar mode (used internally)")
	classicFlag := flag.Bool("classic", false, "run the original full-screen TUI")
	rootFlag := flag.String("root", "", "repo root path (used internally by sidebar)")
	popupCreateFlag := flag.Bool("popup-create", false, "run create form as popup (internal)")
	popupDeleteFlag := flag.Bool("popup-delete", false, "run delete confirm as popup (internal)")
	branchFlag := flag.String("branch", "", "branch name for popup-delete (internal)")
	flag.Parse()

	repoRoot := resolveRepoRoot(*rootFlag)
	cfg := loadConfig(repoRoot)

	switch {
	case *popupCreateFlag:
		m := tui.NewPopupCreateModel(repoRoot, cfg)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case *popupDeleteFlag:
		branch := *branchFlag
		if branch == "" {
			fmt.Fprintln(os.Stderr, "Error: --branch is required for --popup-delete")
			os.Exit(1)
		}
		result, err := state.Gather(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		var found *state.Entry
		for i, e := range result.Entries {
			if e.BranchShort == branch {
				found = &result.Entries[i]
				break
			}
		}
		if found == nil {
			fmt.Fprintf(os.Stderr, "Error: worktree for branch %q not found\n", branch)
			os.Exit(1)
		}
		m := tui.NewPopupConfirmModel(*found, repoRoot, cfg)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

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
		launch(repoRoot, cfg)
	}
}

func launch(repoRoot string, cfg config.Config) {
	sidebarWidth := cfg.SidebarWidth
	if sidebarWidth == "" {
		sidebarWidth = "28"
	}

	state := tmux.DetectState()

	switch state {
	case tmux.OutsideNoSession:
		// Not in tmux, nothing exists — create session with sidebar, attach
		if err := tmux.CreateSessionAndAttach(repoRoot, sidebarWidth, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.OutsideHasSession:
		// Not in tmux, sessions exist — reconnect to the first one
		// Apply theme to all existing sessions in case config changed
		tmux.ApplyThemeToAllSessions(cfg.Theme)
		fmt.Println("Reconnecting to existing synco session...")
		if err := tmux.AttachFirstSession(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.InsideNoSidebar:
		// In tmux but no sidebar — apply theme and add sidebar
		if sess, err := tmux.CurrentSessionName(); err == nil {
			_ = tmux.ApplyTheme(sess, cfg.Theme)
		}
		if err := tmux.AddSidebarToCurrent(repoRoot, sidebarWidth); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case tmux.InsideHasSidebar:
		// Sidebar already running in this session
		fmt.Println("synco is already running in this session.")
		fmt.Println()
		fmt.Println("  Tip: focus the sidebar pane with Ctrl-b + ←")
		fmt.Println("  Or run 'synco --classic' for full-screen mode.")
	}
}

func runMCP(args []string) {
	mcpFlags := flag.NewFlagSet("mcp", flag.ExitOnError)
	rootFlag := mcpFlags.String("root", "", "repo root path")
	mcpFlags.Parse(args)

	repoRoot := resolveRepoRoot(*rootFlag)

	if err := syncmcp.Serve(repoRoot); err != nil {
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
		fmt.Fprintf(os.Stderr, "synco must be run from within a git repository.\n")
		os.Exit(1)
	}
	return root
}

func loadConfig(repoRoot string) config.Config {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load .synco.yaml: %v\n", err)
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

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charles-albert-raymond/synco/internal/config"
	syncmcp "github.com/charles-albert-raymond/synco/internal/mcp"
	"github.com/charles-albert-raymond/synco/internal/notify"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"
	"github.com/charles-albert-raymond/synco/internal/tui"
)

// sourceDir is set at build time via -ldflags to the synco source directory.
// When set, ctrl+r in the TUI can rebuild the binary from source.
var sourceDir string

// buildChannel is set at build time via -ldflags ("stable" or "canary").
var buildChannel string

func main() {
	// Handle subcommands before flag parsing
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp":
			runMCP(os.Args[2:])
			return
		case "notify":
			if err := notify.RunNotify(os.Stdin); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		case "setup-hooks":
			if err := notify.SetupHooks(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
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
		entry, ok := state.FindEntry(result.Entries, branch)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: worktree for branch %q not found\n", branch)
			os.Exit(1)
		}
		found := &entry
		m := tui.NewPopupConfirmModel(*found, repoRoot, cfg)
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case *sidebarFlag:
		// Internal: run the compact sidebar TUI
		m := tui.NewSidebarModel(repoRoot, cfg, sourceDir)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
		final, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if fm, ok := final.(tui.Model); ok && fm.RestartRequested {
			reexec()
		}

	case *classicFlag:
		// Original full-screen TUI
		m := tui.NewModel(repoRoot, cfg, sourceDir)
		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithReportFocus())
		final, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if fm, ok := final.(tui.Model); ok && fm.RestartRequested {
			reexec()
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

	project := tmux.ProjectName(repoRoot)
	state := tmux.DetectState(project)

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
		tmux.ApplyThemeToAllSessions(project, cfg.Theme)
		fmt.Println("Reconnecting to existing synco session...")
		if err := tmux.AttachFirstSession(project); err != nil {
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
	// Always resolve to the main worktree root so that synco launched from a
	// linked worktree uses the same repo root as the main worktree.
	return tmux.MainWorktreeRoot(root)
}

func loadConfig(repoRoot string) config.Config {
	cfg, err := config.Load(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load .synco.yaml: %v\n", err)
		cfg = config.Config{WorktreeDir: ".."}
	}
	return cfg
}

// reexec replaces the current process with a fresh instance of the same binary.
func reexec() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding executable: %v\n", err)
		os.Exit(1)
	}
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "Error re-executing: %v\n", err)
		os.Exit(1)
	}
}

func findGitRoot() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository")
	}
	toplevel := strings.TrimSpace(string(out))
	// Resolve to the main worktree root in case CWD is inside a linked worktree
	return tmux.MainWorktreeRoot(toplevel), nil
}

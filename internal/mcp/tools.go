package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/orchestrate"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Tool definitions ---

var listWorktreesTool = mcp.NewTool("synco_list_worktrees",
	mcp.WithDescription("List all git worktrees with their tmux session status, listening ports, and git state."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var createWorktreeTool = mcp.NewTool("synco_create_worktree",
	mcp.WithDescription("Create a new git worktree with a tmux session. Optionally runs the on_create hook."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name to create (e.g. 'feature/auth-refactor').")),
	mcp.WithString("base", mcp.Description("Base branch or commit to branch from. Defaults to HEAD.")),
)

var deleteWorktreeTool = mcp.NewTool("synco_delete_worktree",
	mcp.WithDescription("Delete a git worktree and its tmux session. Cannot delete the main worktree."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to delete.")),
	mcp.WithBoolean("delete_branch", mcp.Description("Also delete the git branch. Defaults to the auto_delete_branch config value.")),
	mcp.WithDestructiveHintAnnotation(true),
)

var switchSessionTool = mcp.NewTool("synco_switch_session",
	mcp.WithDescription("Switch the tmux client to a worktree's session. Only works when invoked from inside tmux."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to switch to.")),
	mcp.WithDestructiveHintAnnotation(false),
)

var sendKeysTool = mcp.NewTool("synco_send_keys",
	mcp.WithDescription("Send a command to a worktree's tmux session (like typing it in the terminal and pressing Enter)."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree whose session to send keys to.")),
	mcp.WithString("keys", mcp.Required(), mcp.Description("The command or keystrokes to send.")),
)

var getConfigTool = mcp.NewTool("synco_get_config",
	mcp.WithDescription("Read the current synco configuration (merged global + local)."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var sessionOutputTool = mcp.NewTool("synco_session_output",
	mcp.WithDescription("Capture recent terminal output from a worktree's tmux session pane."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree whose session output to capture.")),
	mcp.WithNumber("lines", mcp.Description("Number of lines to capture. Defaults to 50.")),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

// --- Helpers ---

func textResult(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return mcp.NewToolResultText(string(data)), nil
}

func errResult(format string, args ...any) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...)), nil
}

func stringArg(req mcp.CallToolRequest, name string) string {
	args := req.GetArguments()
	v, _ := args[name].(string)
	return v
}

func boolArg(req mcp.CallToolRequest, name string) (val bool, present bool) {
	args := req.GetArguments()
	v, ok := args[name].(bool)
	return v, ok
}

func numberArg(req mcp.CallToolRequest, name string) (val float64, present bool) {
	args := req.GetArguments()
	v, ok := args[name].(float64)
	return v, ok
}

// findEntry looks up a state.Entry by branch name.
func findEntry(entries []state.Entry, branch string) (state.Entry, bool) {
	for _, e := range entries {
		if e.BranchShort == branch {
			return e, true
		}
	}
	return state.Entry{}, false
}

// --- Handlers ---

type worktreeInfo struct {
	Branch      string `json:"branch"`
	Alias       string `json:"alias,omitempty"`
	Path        string `json:"path"`
	HEAD        string `json:"head"`
	IsMain      bool   `json:"is_main"`
	SessionName string `json:"session_name"`
	HasSession  bool   `json:"has_session"`
	IsAttached  bool   `json:"is_attached"`
	Ports       []int  `json:"ports,omitempty"`
	IsCurrent   bool   `json:"is_current"`
}

func (tc *toolContext) handleListWorktrees(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	result, err := state.Gather(tc.repoRoot)
	if err != nil {
		return errResult("failed to gather worktree state: %v", err)
	}

	cfg, _ := config.Load(tc.repoRoot)

	infos := make([]worktreeInfo, len(result.Entries))
	for i, e := range result.Entries {
		infos[i] = worktreeInfo{
			Branch:      e.BranchShort,
			Alias:       cfg.AliasFor(e.BranchShort),
			Path:        e.Worktree.Path,
			HEAD:        e.Worktree.HEAD,
			IsMain:      e.Worktree.IsMain,
			SessionName: e.SessionName,
			HasSession:  e.HasSession,
			IsAttached:  e.HasSession && e.TmuxSession.Attached,
			Ports:       e.Ports,
			IsCurrent:   e.IsCurrent,
		}
	}

	return textResult(infos)
}

func (tc *toolContext) handleCreateWorktree(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}
	base := stringArg(req, "base")

	cfg, _ := config.Load(tc.repoRoot)
	wtPath, sessName, err := orchestrate.CreateWorktree(tc.repoRoot, cfg, branch, base)
	if err != nil {
		return errResult("%v", err)
	}

	return textResult(map[string]string{
		"status":       "created",
		"branch":       branch,
		"path":         wtPath,
		"session_name": sessName,
	})
}

func (tc *toolContext) handleDeleteWorktree(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}

	result, err := state.Gather(tc.repoRoot)
	if err != nil {
		return errResult("failed to gather state: %v", err)
	}

	entry, found := findEntry(result.Entries, branch)
	if !found {
		return errResult("no worktree found for branch %q", branch)
	}
	if entry.Worktree.IsMain {
		return errResult("cannot delete the main worktree")
	}

	cfg, _ := config.Load(tc.repoRoot)

	deleteBranch := cfg.ShouldDeleteBranch()
	if v, ok := boolArg(req, "delete_branch"); ok {
		deleteBranch = v
	}

	opts := orchestrate.DeleteWorktreeOpts{DeleteBranch: deleteBranch}
	if err := orchestrate.DeleteWorktree(tc.repoRoot, cfg, entry, opts); err != nil {
		return errResult("%v", err)
	}

	return textResult(map[string]any{
		"status":         "deleted",
		"branch":         branch,
		"branch_deleted": deleteBranch,
	})
}

func (tc *toolContext) handleSwitchSession(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}

	if !tmux.IsInsideTmux() {
		return errResult("cannot switch session: not inside tmux")
	}

	sessName := tmux.SessionNameFor(branch)

	// Auto-create session if worktree exists but session doesn't (matches TUI behavior)
	if !tmux.SessionExists(sessName) {
		result, err := state.Gather(tc.repoRoot)
		if err != nil {
			return errResult("failed to gather state: %v", err)
		}
		entry, found := findEntry(result.Entries, branch)
		if !found {
			return errResult("no worktree found for branch %q", branch)
		}
		if err := tmux.NewSession(sessName, entry.Worktree.Path); err != nil {
			return errResult("failed to create session: %v", err)
		}
	}

	if err := tmux.SwitchClient(sessName); err != nil {
		return errResult("failed to switch session: %v", err)
	}

	return textResult(map[string]string{
		"status":  "switched",
		"session": sessName,
	})
}

func (tc *toolContext) handleSendKeys(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}
	keys := stringArg(req, "keys")
	if keys == "" {
		return errResult("keys is required")
	}

	sessName := tmux.SessionNameFor(branch)
	if !tmux.SessionExists(sessName) {
		return errResult("no tmux session for branch %q; create the worktree first or start a session", branch)
	}

	if err := tmux.SendKeys(sessName, keys); err != nil {
		return errResult("failed to send keys: %v", err)
	}

	return textResult(map[string]string{
		"status":  "sent",
		"session": sessName,
		"keys":    keys,
	})
}

func (tc *toolContext) handleGetConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load(tc.repoRoot)
	if err != nil {
		return errResult("failed to load config: %v", err)
	}

	return textResult(map[string]any{
		"worktree_dir":       cfg.WorktreeDir,
		"on_create":          cfg.OnCreate,
		"on_destroy":         cfg.OnDestroy,
		"auto_delete_branch": cfg.ShouldDeleteBranch(),
		"aliases":            cfg.Aliases,
		"global_config_path": config.GlobalConfigPath(),
		"local_config_path":  tc.repoRoot + "/.synco.yaml",
	})
}

func (tc *toolContext) handleSessionOutput(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}

	lines := 50
	if v, ok := numberArg(req, "lines"); ok && v > 0 {
		lines = int(v)
	}

	sessName := tmux.SessionNameFor(branch)
	if !tmux.SessionExists(sessName) {
		return errResult("no tmux session for branch %q; create the worktree first or start a session", branch)
	}

	output, err := tmux.CapturePaneOutput(sessName, lines)
	if err != nil {
		return errResult("failed to capture output: %v", err)
	}

	return textResult(map[string]string{
		"session": sessName,
		"output":  output,
	})
}

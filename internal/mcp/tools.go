package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charles-albert-raymond/synco/internal/config"
	"github.com/charles-albert-raymond/synco/internal/metadata"
	"github.com/charles-albert-raymond/synco/internal/notify"
	"github.com/charles-albert-raymond/synco/internal/orchestrate"
	"github.com/charles-albert-raymond/synco/internal/state"
	"github.com/charles-albert-raymond/synco/internal/tmux"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- Tool definitions ---

var listWorktreesTool = mcp.NewTool("synco_list_worktrees",
	mcp.WithDescription("List all git worktrees with their tmux session status, listening ports, and git state. The main worktree (is_main=true) can be addressed as \"root\" in other tools regardless of its current branch."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var createWorktreeTool = mcp.NewTool("synco_create_worktree",
	mcp.WithDescription("Create a new git worktree with a tmux session. Optionally runs the on_create hook."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name to create (e.g. 'feature/auth-refactor').")),
	mcp.WithString("base", mcp.Description("Base branch or commit to branch from. Defaults to HEAD. Ignored when existing_branch is true.")),
	mcp.WithBoolean("existing_branch", mcp.Description("If true, use an existing local or remote branch instead of creating a new one.")),
	mcp.WithString("title", mcp.Description("Optional human-readable title for the worktree card.")),
)

var deleteWorktreeTool = mcp.NewTool("synco_delete_worktree",
	mcp.WithDescription("Delete a git worktree and its tmux session. Cannot delete the main worktree."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to delete. Use \"root\" for the main worktree (will be rejected — cannot delete main).")),
	mcp.WithBoolean("delete_branch", mcp.Description("Also delete the git branch. Defaults to the auto_delete_branch config value.")),
	mcp.WithDestructiveHintAnnotation(true),
)

var switchSessionTool = mcp.NewTool("synco_switch_session",
	mcp.WithDescription("Switch the tmux client to a worktree's session. Only works when invoked from inside tmux."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to switch to. Use \"root\" to always target the main worktree regardless of its current branch.")),
	mcp.WithDestructiveHintAnnotation(false),
)

var sendKeysTool = mcp.NewTool("synco_send_keys",
	mcp.WithDescription("Send a command to a worktree's tmux session (like typing it in the terminal and pressing Enter)."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree whose session to send keys to. Use \"root\" for the main worktree.")),
	mcp.WithString("keys", mcp.Required(), mcp.Description("The command or keystrokes to send.")),
)

var getConfigTool = mcp.NewTool("synco_get_config",
	mcp.WithDescription("Read the current synco configuration (merged global + local)."),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var sessionOutputTool = mcp.NewTool("synco_session_output",
	mcp.WithDescription("Capture recent terminal output from a worktree's tmux session pane."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree whose session output to capture. Use \"root\" for the main worktree.")),
	mcp.WithNumber("lines", mcp.Description("Number of lines to capture. Defaults to 50.")),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var inspectTaskTool = mcp.NewTool("synco_inspect_task",
	mcp.WithDescription("Read the TICKET.md file from a worktree's directory to understand the task or requirements for that worktree."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree to inspect. Use \"root\" for the main worktree.")),
	mcp.WithString("filename", mcp.Description("File to read. Defaults to TICKET.md.")),
	mcp.WithReadOnlyHintAnnotation(true),
	mcp.WithDestructiveHintAnnotation(false),
)

var setWorktreeTitleTool = mcp.NewTool("synco_set_worktree_title",
	mcp.WithDescription("Set or clear the human-readable title for a worktree card."),
	mcp.WithString("branch", mcp.Required(), mcp.Description("Branch name of the worktree.")),
	mcp.WithString("title", mcp.Required(), mcp.Description("Title to set. Pass empty string to clear.")),
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
// Delegates to state.FindEntry which supports "root" as a stable identifier
// for the main worktree.
func findEntry(entries []state.Entry, branch string) (state.Entry, bool) {
	return state.FindEntry(entries, branch)
}

// resolveSessionName returns the tmux session name for a branch, using state.Gather
// to correctly handle the root worktree (whose session key is stable, not branch-based).
func (tc *toolContext) resolveSessionName(branch string) (string, error) {
	result, err := tc.gatherState()
	if err != nil {
		return "", fmt.Errorf("failed to gather state: %v", err)
	}
	entry, found := findEntry(result.Entries, branch)
	if !found {
		return "", fmt.Errorf("no worktree found for branch %q", branch)
	}
	return entry.SessionName, nil
}

// gatherState gathers worktree state using the config's project name if set.
func (tc *toolContext) gatherState() (*state.GatherResult, error) {
	cfg, _ := config.Load(tc.repoRoot)
	return state.GatherWithOpts(tc.repoRoot, state.GatherOpts{
		ProjectName: cfg.ProjectName,
		WorktreeDir: cfg.WorktreeDir,
	})
}

// --- Handlers ---

type worktreeInfo struct {
	Branch      string `json:"branch"`
	Title       string `json:"title,omitempty"`
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
	result, err := tc.gatherState()
	if err != nil {
		return errResult("failed to gather worktree state: %v", err)
	}

	cfg, err := config.Load(tc.repoRoot)
	if err != nil {
		return errResult("failed to load config: %v", err)
	}

	infos := make([]worktreeInfo, len(result.Entries))
	for i, e := range result.Entries {
		infos[i] = worktreeInfo{
			Branch:      e.BranchShort,
			Title:       e.Title,
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

	cfg, err := config.Load(tc.repoRoot)
	if err != nil {
		return errResult("failed to load config: %v", err)
	}

	var wtPath, sessName string
	if existing, _ := boolArg(req, "existing_branch"); existing {
		wtPath, sessName, err = orchestrate.CreateWorktreeFromExisting(tc.repoRoot, cfg, branch)
	} else {
		base := stringArg(req, "base")
		wtPath, sessName, err = orchestrate.CreateWorktree(tc.repoRoot, cfg, branch, base)
	}
	if err != nil {
		return errResult("%v", err)
	}

	// Save optional title
	title := stringArg(req, "title")
	if title != "" {
		store, err := metadata.Load(tc.repoRoot, cfg.WorktreeDir)
		if err == nil {
			// For existing remote branches, use the local branch name
			metaKey := branch
			if idx := strings.Index(branch, "/"); idx != -1 && !strings.HasPrefix(branch, "refs/") {
				metaKey = branch[idx+1:]
			}
			store.SetTitle(metaKey, title)
			_ = store.Save(tc.repoRoot, cfg.WorktreeDir)
		}
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

	result, err := tc.gatherState()
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

	cfg, err := config.Load(tc.repoRoot)
	if err != nil {
		return errResult("failed to load config: %v", err)
	}

	deleteBranch := cfg.ShouldDeleteBranch()
	if v, ok := boolArg(req, "delete_branch"); ok {
		deleteBranch = v
	}

	opts := orchestrate.DeleteWorktreeOpts{DeleteBranch: deleteBranch}
	if err := orchestrate.DeleteWorktree(tc.repoRoot, cfg, entry, opts); err != nil {
		return errResult("%v", err)
	}

	// Clean up metadata
	if store, err := metadata.Load(tc.repoRoot, cfg.WorktreeDir); err == nil {
		store.Delete(branch)
		_ = store.Save(tc.repoRoot, cfg.WorktreeDir)
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

	result, err := tc.gatherState()
	if err != nil {
		return errResult("failed to gather state: %v", err)
	}
	entry, found := findEntry(result.Entries, branch)
	if !found {
		return errResult("no worktree found for branch %q", branch)
	}
	sessName := entry.SessionName

	// Auto-create session if worktree exists but session doesn't (matches TUI behavior)
	if !entry.HasSession {
		cfg, err := config.Load(tc.repoRoot)
		if err != nil {
			return errResult("failed to load config: %v", err)
		}
		if err := tmux.NewSessionWithLayout(sessName, entry.Worktree.Path, cfg); err != nil {
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

	sessName, err := tc.resolveSessionName(branch)
	if err != nil {
		return errResult("%v", err)
	}
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

	notifInfo := map[string]any{
		"enabled":             cfg.NotificationsEnabled(),
		"bell":                cfg.BellEnabled(),
		"system_notification": cfg.SystemNotificationEnabled(),
		"sound":               cfg.NotificationSound(),
		"silence_seconds":     cfg.SilenceThreshold(),
		"hook_configured":     notify.IsHookConfigured(),
	}

	return textResult(map[string]any{
		"worktree_dir":       cfg.WorktreeDir,
		"on_create":          cfg.OnCreate,
		"on_destroy":         cfg.OnDestroy,
		"auto_delete_branch": cfg.ShouldDeleteBranch(),
		"aliases":            cfg.Aliases,
		"notifications":      notifInfo,
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

	sessName, err := tc.resolveSessionName(branch)
	if err != nil {
		return errResult("%v", err)
	}
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

func (tc *toolContext) handleSetWorktreeTitle(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}
	title := stringArg(req, "title")

	cfg, err := config.Load(tc.repoRoot)
	if err != nil {
		return errResult("failed to load config: %v", err)
	}

	store, err := metadata.Load(tc.repoRoot, cfg.WorktreeDir)
	if err != nil {
		return errResult("failed to load metadata: %v", err)
	}

	if title == "" {
		store.Delete(branch)
	} else {
		store.SetTitle(branch, title)
	}

	if err := store.Save(tc.repoRoot, cfg.WorktreeDir); err != nil {
		return errResult("failed to save metadata: %v", err)
	}

	action := "set"
	if title == "" {
		action = "cleared"
	}
	return textResult(map[string]string{
		"status": action,
		"branch": branch,
		"title":  title,
	})
}

func (tc *toolContext) handleInspectTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	branch := stringArg(req, "branch")
	if branch == "" {
		return errResult("branch is required")
	}

	result, err := tc.gatherState()
	if err != nil {
		return errResult("failed to gather state: %v", err)
	}

	entry, found := findEntry(result.Entries, branch)
	if !found {
		return errResult("no worktree found for branch %q", branch)
	}

	filename := stringArg(req, "filename")
	if filename == "" {
		filename = "TICKET.md"
	}

	taskPath := filepath.Join(entry.Worktree.Path, filename)
	data, err := os.ReadFile(taskPath)
	if err != nil {
		if os.IsNotExist(err) {
			return errResult("no %s found in worktree %q at %s", filename, branch, taskPath)
		}
		return errResult("failed to read %s: %v", filename, err)
	}

	return textResult(map[string]string{
		"branch":   branch,
		"path":     taskPath,
		"filename": filename,
		"content":  string(data),
	})
}

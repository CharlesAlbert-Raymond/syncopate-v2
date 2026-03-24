package mcp

import (
	"context"
	"os"

	"github.com/charles-albert-raymond/syncopate/internal/config"
	"github.com/mark3labs/mcp-go/server"
)

// toolContext holds shared state for all MCP tool handlers.
type toolContext struct {
	repoRoot string
	cfg      config.Config
}

// Serve starts the MCP server over stdio, blocking until the connection closes.
func Serve(repoRoot string, cfg config.Config) error {
	s := server.NewMCPServer("syncopate", "1.0.0",
		server.WithToolCapabilities(true),
	)

	ctx := &toolContext{repoRoot: repoRoot, cfg: cfg}

	s.AddTool(listWorktreesTool, ctx.handleListWorktrees)
	s.AddTool(createWorktreeTool, ctx.handleCreateWorktree)
	s.AddTool(deleteWorktreeTool, ctx.handleDeleteWorktree)
	s.AddTool(switchSessionTool, ctx.handleSwitchSession)
	s.AddTool(sendKeysTool, ctx.handleSendKeys)
	s.AddTool(getConfigTool, ctx.handleGetConfig)
	s.AddTool(sessionOutputTool, ctx.handleSessionOutput)

	stdio := server.NewStdioServer(s)
	return stdio.Listen(context.Background(), os.Stdin, os.Stdout)
}

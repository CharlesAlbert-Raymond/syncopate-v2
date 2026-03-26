package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Pane defines a single pane in a layout.
type Pane struct {
	Command string `yaml:"command"`
	Split   string `yaml:"split,omitempty"`   // "horizontal" or "vertical"
	Size    string `yaml:"size,omitempty"`     // e.g. "30%"
}

// Layout defines a named window layout with multiple panes.
type Layout struct {
	Panes []Pane `yaml:"panes"`
}

// Theme holds tmux border color configuration.
type Theme struct {
	PaneBorder       string `yaml:"pane_border,omitempty"`
	PaneBorderActive string `yaml:"pane_border_active,omitempty"`
	PaneBorderLabels bool   `yaml:"pane_border_labels,omitempty"`
}

// Notifications holds notification preferences.
type Notifications struct {
	Enabled            *bool  `yaml:"enabled,omitempty"`
	SilenceSeconds     int    `yaml:"silence_seconds,omitempty"`
	Bell               *bool  `yaml:"bell,omitempty"`
	SystemNotification *bool  `yaml:"system_notification,omitempty"`
	Sound              string `yaml:"sound,omitempty"`
	OnSilence          string `yaml:"on_silence,omitempty"`
}

// Config holds the merged synco configuration.
type Config struct {
	WorktreeDir      string            `yaml:"worktree_dir"`
	SidebarWidth     string            `yaml:"sidebar_width,omitempty"`
	OnCreate         string            `yaml:"on_create"`
	OnDestroy        string            `yaml:"on_destroy"`
	AutoDeleteBranch *bool             `yaml:"auto_delete_branch,omitempty"`
	Aliases          map[string]string `yaml:"aliases,omitempty"`
	Theme            *Theme            `yaml:"theme,omitempty"`
	Layouts          map[string]Layout `yaml:"layouts,omitempty"`
	Notifications    *Notifications    `yaml:"notifications,omitempty"`
}

// DefaultLayout returns the "default" layout, or nil if none is configured.
func (c Config) DefaultLayout() *Layout {
	if c.Layouts == nil {
		return nil
	}
	l, ok := c.Layouts["default"]
	if !ok {
		return nil
	}
	return &l
}

// NotificationsEnabled returns true when notifications are configured and not explicitly disabled.
func (c Config) NotificationsEnabled() bool {
	if c.Notifications == nil {
		return false
	}
	if c.Notifications.Enabled != nil {
		return *c.Notifications.Enabled
	}
	return true // enabled by default when section exists
}

// SilenceThreshold returns the configured silence seconds, or 10 as default.
func (c Config) SilenceThreshold() int {
	if c.Notifications != nil && c.Notifications.SilenceSeconds > 0 {
		return c.Notifications.SilenceSeconds
	}
	return 10
}

// BellEnabled returns whether the terminal bell should fire on silence.
func (c Config) BellEnabled() bool {
	if c.Notifications != nil && c.Notifications.Bell != nil {
		return *c.Notifications.Bell
	}
	return true
}

// SystemNotificationEnabled returns whether macOS system notifications should fire.
func (c Config) SystemNotificationEnabled() bool {
	if c.Notifications != nil && c.Notifications.SystemNotification != nil {
		return *c.Notifications.SystemNotification
	}
	return true
}

// NotificationSound returns the macOS notification sound name, or "Glass" as default.
func (c Config) NotificationSound() string {
	if c.Notifications != nil && c.Notifications.Sound != "" {
		return c.Notifications.Sound
	}
	return "Glass"
}

// AliasFor returns the alias for a branch, or empty string if none.
func (c Config) AliasFor(branch string) string {
	if c.Aliases == nil {
		return ""
	}
	return c.Aliases[branch]
}

// ShouldDeleteBranch returns the resolved value of auto_delete_branch (default: false).
func (c Config) ShouldDeleteBranch() bool {
	if c.AutoDeleteBranch != nil {
		return *c.AutoDeleteBranch
	}
	return false
}

// Load reads the global config then the local config, merging them.
// Local fields override global when set.
func Load(repoRoot string) (Config, error) {
	global, _ := loadFile(globalConfigPath())
	local, _ := loadFile(filepath.Join(repoRoot, ".synco.yaml"))

	merged := merge(global, local)

	if merged.WorktreeDir == "" {
		merged.WorktreeDir = ".worktrees"
	}

	return merged, nil
}

// GlobalConfigPath returns the path to the global config file.
func GlobalConfigPath() string {
	return globalConfigPath()
}

func globalConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "synco", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "synco", "config.yaml")
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// merge returns a Config where local fields override global when non-empty.
func merge(global, local Config) Config {
	out := global

	if local.WorktreeDir != "" {
		out.WorktreeDir = local.WorktreeDir
	}
	if local.SidebarWidth != "" {
		out.SidebarWidth = local.SidebarWidth
	}
	if local.OnCreate != "" {
		out.OnCreate = local.OnCreate
	}
	if local.OnDestroy != "" {
		out.OnDestroy = local.OnDestroy
	}
	if local.AutoDeleteBranch != nil {
		out.AutoDeleteBranch = local.AutoDeleteBranch
	}

	// Merge aliases: local overrides global per-key
	if len(local.Aliases) > 0 {
		if out.Aliases == nil {
			out.Aliases = make(map[string]string)
		}
		for k, v := range local.Aliases {
			out.Aliases[k] = v
		}
	}

	// Theme: local overrides global entirely if set
	if local.Theme != nil {
		out.Theme = local.Theme
	}

	// Notifications: local overrides global entirely if set
	if local.Notifications != nil {
		out.Notifications = local.Notifications
	}

	// Layouts: local overrides global per-key
	if len(local.Layouts) > 0 {
		if out.Layouts == nil {
			out.Layouts = make(map[string]Layout)
		}
		for k, v := range local.Layouts {
			out.Layouts[k] = v
		}
	}

	return out
}

// WorktreePath computes the absolute worktree path for a branch.
func (c Config) WorktreePath(repoRoot, branch string) string {
	safeName := sanitizeBranchForPath(branch)
	return filepath.Join(repoRoot, c.WorktreeDir, safeName)
}

func sanitizeBranchForPath(branch string) string {
	// Replace path separators so feature/foo becomes feature-foo
	result := make([]byte, 0, len(branch))
	for i := 0; i < len(branch); i++ {
		ch := branch[i]
		if ch == '/' || ch == '\\' {
			result = append(result, '-')
		} else {
			result = append(result, ch)
		}
	}
	return string(result)
}

package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds the merged syncopate configuration.
type Config struct {
	WorktreeDir string `yaml:"worktree_dir"`
	OnCreate    string `yaml:"on_create"`
	OnDestroy   string `yaml:"on_destroy"`
}

// Load reads the global config then the local config, merging them.
// Local fields override global when set.
func Load(repoRoot string) (Config, error) {
	global, _ := loadFile(globalConfigPath())
	local, _ := loadFile(filepath.Join(repoRoot, ".syncopate.yaml"))

	merged := merge(global, local)

	if merged.WorktreeDir == "" {
		merged.WorktreeDir = ".worktrees"
	}

	return merged, nil
}

func globalConfigPath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "syncopate", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "syncopate", "config.yaml")
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
	if local.OnCreate != "" {
		out.OnCreate = local.OnCreate
	}
	if local.OnDestroy != "" {
		out.OnDestroy = local.OnDestroy
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

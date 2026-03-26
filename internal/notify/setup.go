package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const hookCommand = "synco notify"

// IsHookConfigured returns true if the Claude Code Stop hook for synco is set up.
func IsHookConfigured() bool {
	settings, err := readJSONFile(claudeSettingsPath())
	if err != nil {
		return false
	}
	return hookExists(settings)
}

// SetupHooks adds a Claude Code Stop hook for `synco notify` to the user's
// ~/.claude/settings.json. It is idempotent and preserves existing hooks.
func SetupHooks() error {
	settingsPath := claudeSettingsPath()

	// Read existing settings
	settings, err := readJSONFile(settingsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", settingsPath, err)
	}
	if settings == nil {
		settings = make(map[string]any)
	}

	// Check if our hook already exists
	if hookExists(settings) {
		fmt.Println("Claude Code Stop hook for synco is already configured.")
		return nil
	}

	// Back up the file if it exists
	if _, err := os.Stat(settingsPath); err == nil {
		backupPath := settingsPath + ".backup"
		data, _ := os.ReadFile(settingsPath)
		if err := os.WriteFile(backupPath, data, 0644); err != nil {
			return fmt.Errorf("backing up settings: %w", err)
		}
		fmt.Printf("Backed up existing settings to %s\n", backupPath)
	}

	// Add our hook
	addHook(settings)

	// Write back
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(settingsPath, append(data, '\n'), 0644); err != nil {
		return err
	}

	fmt.Printf("Added Stop hook to %s\n", settingsPath)
	fmt.Println("Claude Code will now notify synco when an agent finishes.")
	return nil
}

func claudeSettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func readJSONFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// hookExists checks if a Stop hook containing "synco notify" is already configured.
func hookExists(settings map[string]any) bool {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		return false
	}
	stopHooks, ok := hooks["Stop"].([]any)
	if !ok {
		return false
	}
	for _, entry := range stopHooks {
		entryMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		innerHooks, ok := entryMap["hooks"].([]any)
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hMap["command"].(string)
			if strings.Contains(cmd, "synco notify") {
				return true
			}
		}
	}
	return false
}

// addHook appends a Stop hook entry for `synco notify`.
func addHook(settings map[string]any) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = make(map[string]any)
		settings["hooks"] = hooks
	}

	newEntry := map[string]any{
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": hookCommand,
			},
		},
	}

	stopHooks, ok := hooks["Stop"].([]any)
	if !ok {
		stopHooks = []any{}
	}
	hooks["Stop"] = append(stopHooks, newEntry)
}

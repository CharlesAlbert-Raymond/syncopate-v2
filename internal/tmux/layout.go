package tmux

import (
	"fmt"
	"os/exec"

	"github.com/charles-albert-raymond/syncopate/internal/config"
)

// ApplyTheme sets pane border styles on the given tmux session.
func ApplyTheme(session string, theme *config.Theme) error {
	if theme == nil {
		return nil
	}

	opts := make(map[string]string)

	if theme.PaneBorder != "" {
		opts["pane-border-style"] = fmt.Sprintf("fg=%s", theme.PaneBorder)
	}
	if theme.PaneBorderActive != "" {
		opts["pane-active-border-style"] = fmt.Sprintf("fg=%s", theme.PaneBorderActive)
	}
	if theme.PaneBorderLabels {
		opts["pane-border-status"] = "top"
		opts["pane-border-format"] = " #{pane_index}: #{pane_current_command} "
	}

	for k, v := range opts {
		cmd := exec.Command("tmux", "set-option", "-t", session, k, v)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("tmux set-option %s: %s: %w", k, string(out), err)
		}
	}

	return nil
}

// ApplyLayout splits the session's first window into the configured panes
// and runs the specified commands. The first pane definition uses the existing
// pane; subsequent panes are created via split-window.
func ApplyLayout(session string, layout *config.Layout) error {
	if layout == nil || len(layout.Panes) == 0 {
		return nil
	}

	// The first pane already exists. Send its command if set.
	firstPane := layout.Panes[0]
	if firstPane.Command != "" {
		if err := SendKeys(session, firstPane.Command); err != nil {
			return fmt.Errorf("layout pane 0: %w", err)
		}
	}

	// Create additional panes
	for i := 1; i < len(layout.Panes); i++ {
		pane := layout.Panes[i]

		args := []string{"split-window"}

		// Split direction: horizontal = top/bottom (-v), vertical = left/right (-h)
		// tmux convention: -v splits vertically (creates horizontal layout),
		// -h splits horizontally (creates vertical layout).
		// We follow the user's mental model: "horizontal" = stack top/bottom, "vertical" = side by side.
		switch pane.Split {
		case "vertical":
			args = append(args, "-h")
		default: // horizontal is the default
			args = append(args, "-v")
		}

		if pane.Size != "" {
			args = append(args, "-l", pane.Size)
		}

		args = append(args, "-t", session)

		cmd := exec.Command("tmux", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("layout pane %d split: %s: %w", i, string(out), err)
		}

		// Send the command to the newly created pane (it becomes the active pane)
		if pane.Command != "" {
			if err := SendKeys(session, pane.Command); err != nil {
				return fmt.Errorf("layout pane %d command: %w", i, err)
			}
		}
	}

	return nil
}

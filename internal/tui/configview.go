package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/charles-albert-raymond/syncopate/internal/config"
)

type configViewModel struct {
	config   config.Config
	repoRoot string
}

func newConfigViewModel(cfg config.Config, repoRoot string) configViewModel {
	return configViewModel{config: cfg, repoRoot: repoRoot}
}

func (m configViewModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Configuration"))
	b.WriteString("\n\n")

	// Sources
	headerStyle := lipgloss.NewStyle().Foreground(colorSecondary).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(colorMuted).Width(16)
	valueStyle := lipgloss.NewStyle().Foreground(colorText)
	emptyStyle := lipgloss.NewStyle().Foreground(colorMuted).Italic(true)

	b.WriteString(headerStyle.Render("Sources"))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Global:"))
	b.WriteString(valueStyle.Render(config.GlobalConfigPath()))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Local:"))
	b.WriteString(valueStyle.Render(filepath.Join(m.repoRoot, ".syncopate.yaml")))
	b.WriteString("\n\n")

	// Resolved config
	b.WriteString(headerStyle.Render("Resolved Config"))
	b.WriteString("\n")

	renderField := func(label, value string) {
		b.WriteString(labelStyle.Render("  " + label + ":"))
		if value == "" {
			b.WriteString(emptyStyle.Render("(not set)"))
		} else {
			b.WriteString(valueStyle.Render(value))
		}
		b.WriteString("\n")
	}

	renderField("worktree_dir", m.config.WorktreeDir)
	absDir := m.config.WorktreeDir
	if !filepath.IsAbs(absDir) {
		absDir = filepath.Join(m.repoRoot, absDir)
	}
	b.WriteString(labelStyle.Render(""))
	b.WriteString(pathStyle.Render(fmt.Sprintf("→ %s", absDir)))
	b.WriteString("\n")

	renderField("on_create", m.config.OnCreate)
	renderField("on_destroy", m.config.OnDestroy)

	deleteBranchVal := "false"
	if m.config.ShouldDeleteBranch() {
		deleteBranchVal = "true"
	}
	renderField("auto_delete_branch", deleteBranchVal)

	if len(m.config.Aliases) > 0 {
		b.WriteString("\n")
		b.WriteString(headerStyle.Render("Aliases"))
		b.WriteString("\n")
		for branch, alias := range m.config.Aliases {
			b.WriteString(labelStyle.Render("  " + branch + ":"))
			b.WriteString(valueStyle.Render(alias))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render(" ? close • esc close"))

	return borderStyle.Render(b.String())
}

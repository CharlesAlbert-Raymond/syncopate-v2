package config

import "testing"

func TestMerge(t *testing.T) {
	trueVal := true
	global := Config{
		WorktreeDir: ".wt",
		OnCreate:    "npm install",
		Aliases:     map[string]string{"main": "trunk"},
	}
	local := Config{
		WorktreeDir:      ".worktrees",
		AutoDeleteBranch: &trueVal,
		Aliases:          map[string]string{"dev": "development"},
	}

	got := merge(global, local)

	if got.WorktreeDir != ".worktrees" {
		t.Errorf("WorktreeDir = %q, want .worktrees (local overrides global)", got.WorktreeDir)
	}
	if got.OnCreate != "npm install" {
		t.Errorf("OnCreate = %q, want npm install (inherited from global)", got.OnCreate)
	}
	if !got.ShouldDeleteBranch() {
		t.Error("ShouldDeleteBranch() = false, want true (local overrides)")
	}
	if got.Aliases["main"] != "trunk" {
		t.Error("global alias 'main' should be preserved")
	}
	if got.Aliases["dev"] != "development" {
		t.Error("local alias 'dev' should be merged in")
	}
}

func TestMergeEmptyLocal(t *testing.T) {
	global := Config{WorktreeDir: ".wt", OnCreate: "echo hi"}
	got := merge(global, Config{})

	if got.WorktreeDir != ".wt" {
		t.Errorf("WorktreeDir = %q, want .wt", got.WorktreeDir)
	}
	if got.OnCreate != "echo hi" {
		t.Errorf("OnCreate = %q, want 'echo hi'", got.OnCreate)
	}
}

func TestSanitizeBranchForPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"feature/auth", "feature-auth"},
		{"simple", "simple"},
		{"a/b/c", "a-b-c"},
		{"back\\slash", "back-slash"},
		{"no-change", "no-change"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeBranchForPath(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeBranchForPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShouldDeleteBranchDefault(t *testing.T) {
	c := Config{}
	if c.ShouldDeleteBranch() {
		t.Error("default should be false")
	}
}

func TestMergeNotifications(t *testing.T) {
	falseVal := false
	global := Config{
		Notifications: &Notifications{
			SilenceSeconds: 30,
			Sound:          "Ping",
		},
	}
	local := Config{
		Notifications: &Notifications{
			Enabled:        &falseVal,
			SilenceSeconds: 5,
		},
	}
	got := merge(global, local)

	if got.Notifications == nil {
		t.Fatal("Notifications should not be nil")
	}
	if got.NotificationsEnabled() {
		t.Error("NotificationsEnabled() = true, want false (local explicitly disabled)")
	}
	if got.SilenceThreshold() != 5 {
		t.Errorf("SilenceThreshold() = %d, want 5 (local overrides global entirely)", got.SilenceThreshold())
	}
	// Sound should be empty since local overrides global entirely
	if got.NotificationSound() != "Glass" {
		// local.Sound is empty, so default "Glass" should be returned
	}
}

func TestNotificationsDefaults(t *testing.T) {
	// nil Notifications: disabled
	c := Config{}
	if c.NotificationsEnabled() {
		t.Error("nil Notifications should mean disabled")
	}
	if c.SilenceThreshold() != 10 {
		t.Errorf("default SilenceThreshold = %d, want 10", c.SilenceThreshold())
	}

	// non-nil Notifications with zero values: enabled with defaults
	c2 := Config{Notifications: &Notifications{}}
	if !c2.NotificationsEnabled() {
		t.Error("empty Notifications struct should default to enabled")
	}
	if !c2.BellEnabled() {
		t.Error("default BellEnabled should be true")
	}
	if !c2.SystemNotificationEnabled() {
		t.Error("default SystemNotificationEnabled should be true")
	}
	if c2.NotificationSound() != "Glass" {
		t.Errorf("default NotificationSound = %q, want Glass", c2.NotificationSound())
	}
}

func TestAliasFor(t *testing.T) {
	c := Config{Aliases: map[string]string{"main": "trunk"}}
	if got := c.AliasFor("main"); got != "trunk" {
		t.Errorf("AliasFor(main) = %q, want trunk", got)
	}
	if got := c.AliasFor("missing"); got != "" {
		t.Errorf("AliasFor(missing) = %q, want empty", got)
	}

	// nil aliases
	c2 := Config{}
	if got := c2.AliasFor("main"); got != "" {
		t.Errorf("AliasFor with nil aliases = %q, want empty", got)
	}
}

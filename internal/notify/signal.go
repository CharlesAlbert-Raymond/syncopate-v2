package notify

import (
	"os"
	"path/filepath"
	"time"
)

// SignalDir returns the directory where notification signal files are stored.
func SignalDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "synco", "notify")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "synco", "notify")
}

// WriteSignal creates a signal file for the given session name.
func WriteSignal(sessionName string) error {
	dir := SignalDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, sessionName)
	return os.WriteFile(path, []byte(time.Now().UTC().Format(time.RFC3339)), 0644)
}

// HasSignal returns true if a signal file exists for the given session.
func HasSignal(sessionName string) bool {
	path := filepath.Join(SignalDir(), sessionName)
	_, err := os.Stat(path)
	return err == nil
}

// ClearSignal removes the signal file for the given session.
func ClearSignal(sessionName string) {
	path := filepath.Join(SignalDir(), sessionName)
	os.Remove(path)
}

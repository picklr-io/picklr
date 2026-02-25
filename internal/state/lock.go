package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Lock acquires a file lock on the state to prevent concurrent modifications.
func (m *Manager) Lock() error {
	lockPath := m.lockPath()
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Check if lock already exists
	if info, err := os.Stat(lockPath); err == nil {
		// If lock is older than 10 minutes, consider it stale
		if time.Since(info.ModTime()) > 10*time.Minute {
			os.Remove(lockPath)
		} else {
			return fmt.Errorf("state is locked by another process (lock file: %s). "+
				"If this is an error, remove the lock file manually", lockPath)
		}
	}

	// Create lock file with current PID and timestamp
	content := fmt.Sprintf("pid=%d\ntime=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(lockPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	return nil
}

// Unlock releases the state lock.
func (m *Manager) Unlock() error {
	lockPath := m.lockPath()
	if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}
	return nil
}

func (m *Manager) lockPath() string {
	return m.path + ".lock"
}

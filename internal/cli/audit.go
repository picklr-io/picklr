package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	Timestamp string         `json:"timestamp"`
	Operation string         `json:"operation"` // "apply", "destroy", "import", "state.rm", "state.mv"
	User      string         `json:"user"`
	Workspace string         `json:"workspace"`
	Changes   []AuditChange  `json:"changes,omitempty"`
	Summary   map[string]int `json:"summary,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// AuditChange records a single resource change.
type AuditChange struct {
	Address string `json:"address"`
	Action  string `json:"action"`
}

// auditLogPath returns the path to the audit log file.
func auditLogPath() string {
	return filepath.Join(picklrDir(), "audit.log")
}

// writeAuditLog appends an audit entry to the audit log file.
func writeAuditLog(entry AuditEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.User == "" {
		entry.User = currentUser()
	}
	if entry.Workspace == "" {
		entry.Workspace = currentWorkspace()
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal audit entry: %w", err)
	}

	f, err := os.OpenFile(auditLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		// Audit logging failure should not block operations
		return nil
	}
	defer f.Close()

	_, err = f.WriteString(string(data) + "\n")
	return err
}

func currentUser() string {
	if user := os.Getenv("USER"); user != "" {
		return user
	}
	if user := os.Getenv("USERNAME"); user != "" {
		return user
	}
	return "unknown"
}

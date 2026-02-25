package state

import (
	"context"
	"fmt"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
)

// Backend defines the interface for state storage backends.
type Backend interface {
	// Read loads the state from the backend.
	Read(ctx context.Context) (*ir.State, error)

	// Write saves the state to the backend.
	Write(ctx context.Context, state *ir.State) error

	// Lock acquires an exclusive lock on the state.
	Lock() error

	// Unlock releases the lock on the state.
	Unlock() error
}

// BackendConfig holds configuration for a state backend.
type BackendConfig struct {
	Type   string            `json:"type"` // "local", "s3", "gcs", "http"
	Config map[string]string `json:"config"`
}

// S3BackendConfig holds configuration for S3 state backend.
type S3BackendConfig struct {
	Bucket        string `json:"bucket"`
	Key           string `json:"key"`
	Region        string `json:"region"`
	DynamoDBTable string `json:"dynamodb_table"` // for locking
	Encrypt       bool   `json:"encrypt"`
	Profile       string `json:"profile"`
}

// GCSBackendConfig holds configuration for Google Cloud Storage backend.
type GCSBackendConfig struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
}

// HTTPBackendConfig holds configuration for HTTP state backend.
type HTTPBackendConfig struct {
	Address       string `json:"address"`
	LockAddress   string `json:"lock_address"`
	UnlockAddress string `json:"unlock_address"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

// NewBackend creates a state backend from configuration.
// The evaluator is needed for backends that must parse PKL state content.
func NewBackend(cfg *BackendConfig, evaluator *eval.Evaluator) (Backend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("backend configuration is nil")
	}

	switch cfg.Type {
	case "local", "":
		return nil, fmt.Errorf("use state.Manager for local backend")
	case "s3":
		return newS3Backend(cfg.Config, evaluator)
	case "gcs":
		return nil, fmt.Errorf("GCS backend not yet implemented")
	case "http":
		return nil, fmt.Errorf("HTTP backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Type)
	}
}

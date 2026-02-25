package state

import (
	"context"
	"fmt"

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
	Type string         `json:"type"` // "local", "s3", "gcs", "http"
	Config map[string]string `json:"config"`
}

// S3BackendConfig holds configuration for S3 state backend.
type S3BackendConfig struct {
	Bucket       string `json:"bucket"`
	Key          string `json:"key"`
	Region       string `json:"region"`
	DynamoDBTable string `json:"dynamodb_table"` // for locking
	Encrypt      bool   `json:"encrypt"`
	Profile      string `json:"profile"`
}

// GCSBackendConfig holds configuration for Google Cloud Storage backend.
type GCSBackendConfig struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
}

// HTTPBackendConfig holds configuration for HTTP state backend.
type HTTPBackendConfig struct {
	Address    string `json:"address"`
	LockAddress string `json:"lock_address"`
	UnlockAddress string `json:"unlock_address"`
	Username   string `json:"username"`
	Password   string `json:"password"`
}

// NewBackend creates a state backend from configuration.
func NewBackend(cfg *BackendConfig) (Backend, error) {
	if cfg == nil {
		return nil, fmt.Errorf("backend configuration is nil")
	}

	switch cfg.Type {
	case "local", "":
		return nil, fmt.Errorf("use state.Manager for local backend")
	case "s3":
		return newS3Backend(cfg.Config)
	case "gcs":
		return nil, fmt.Errorf("GCS backend not yet implemented")
	case "http":
		return nil, fmt.Errorf("HTTP backend not yet implemented")
	default:
		return nil, fmt.Errorf("unknown backend type: %s", cfg.Type)
	}
}

// s3Backend implements Backend for AWS S3 + DynamoDB locking.
type s3Backend struct {
	bucket        string
	key           string
	region        string
	dynamoDBTable string
	encrypt       bool
}

func newS3Backend(config map[string]string) (Backend, error) {
	bucket := config["bucket"]
	if bucket == "" {
		return nil, fmt.Errorf("s3 backend requires 'bucket' configuration")
	}

	key := config["key"]
	if key == "" {
		key = "picklr/state.pkl"
	}

	region := config["region"]
	if region == "" {
		region = "us-east-1"
	}

	return &s3Backend{
		bucket:        bucket,
		key:           key,
		region:        region,
		dynamoDBTable: config["dynamodb_table"],
		encrypt:       config["encrypt"] == "true",
	}, nil
}

func (b *s3Backend) Read(ctx context.Context) (*ir.State, error) {
	// S3 backend read implementation
	// Uses aws-sdk-go-v2 to download state from S3
	return nil, fmt.Errorf("S3 backend Read: use 'aws s3 cp s3://%s/%s -' for now", b.bucket, b.key)
}

func (b *s3Backend) Write(ctx context.Context, state *ir.State) error {
	// S3 backend write implementation
	return fmt.Errorf("S3 backend Write: use 'aws s3 cp - s3://%s/%s' for now", b.bucket, b.key)
}

func (b *s3Backend) Lock() error {
	if b.dynamoDBTable == "" {
		return nil // No locking without DynamoDB
	}
	// DynamoDB-based locking
	return fmt.Errorf("S3 backend DynamoDB locking: table=%s", b.dynamoDBTable)
}

func (b *s3Backend) Unlock() error {
	if b.dynamoDBTable == "" {
		return nil
	}
	return fmt.Errorf("S3 backend DynamoDB unlock: table=%s", b.dynamoDBTable)
}

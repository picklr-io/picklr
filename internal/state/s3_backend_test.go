package state

import (
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewS3BackendRequiresBucket(t *testing.T) {
	_, err := newS3Backend(map[string]string{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket")
}

func TestNewS3BackendDefaults(t *testing.T) {
	// This will fail due to missing AWS credentials, but validates config parsing
	config := map[string]string{
		"bucket": "my-bucket",
	}
	b, err := newS3Backend(config, nil)
	// May fail on AWS config load in CI without credentials, which is expected
	if err != nil {
		t.Skipf("Skipping S3 backend test (no AWS credentials): %v", err)
	}
	s3b, ok := b.(*s3Backend)
	require.True(t, ok)
	assert.Equal(t, "my-bucket", s3b.bucket)
	assert.Equal(t, "picklr/state.pkl", s3b.key)
	assert.Equal(t, "us-east-1", s3b.region)
	assert.Empty(t, s3b.dynamoDBTable)
	assert.False(t, s3b.encrypt)
}

func TestNewS3BackendCustomConfig(t *testing.T) {
	config := map[string]string{
		"bucket":         "custom-bucket",
		"key":            "custom/path/state.pkl",
		"region":         "eu-west-1",
		"dynamodb_table": "picklr-locks",
		"encrypt":        "true",
		"profile":        "staging",
	}
	b, err := newS3Backend(config, nil)
	if err != nil {
		t.Skipf("Skipping S3 backend test (no AWS credentials): %v", err)
	}
	s3b, ok := b.(*s3Backend)
	require.True(t, ok)
	assert.Equal(t, "custom-bucket", s3b.bucket)
	assert.Equal(t, "custom/path/state.pkl", s3b.key)
	assert.Equal(t, "eu-west-1", s3b.region)
	assert.Equal(t, "picklr-locks", s3b.dynamoDBTable)
	assert.True(t, s3b.encrypt)
}

func TestSerializeState(t *testing.T) {
	state := &ir.State{
		Version: 1,
		Serial:  2,
		Lineage: "abc-123",
	}
	content := SerializeState(state)
	assert.Contains(t, content, "version = 1")
	assert.Contains(t, content, "serial = 3") // Serial is incremented
	assert.Contains(t, content, `lineage = "abc-123"`)
	assert.Contains(t, content, "resources {")
}

func TestNewBackendRejectsNilConfig(t *testing.T) {
	_, err := NewBackend(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

func TestNewBackendRejectsUnknownType(t *testing.T) {
	_, err := NewBackend(&BackendConfig{Type: "redis"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown backend type")
}

func TestNewBackendLocalFallback(t *testing.T) {
	_, err := NewBackend(&BackendConfig{Type: "local"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "state.Manager")
}

func TestNewBackendGCSNotImplemented(t *testing.T) {
	_, err := NewBackend(&BackendConfig{Type: "gcs"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestNewBackendHTTPNotImplemented(t *testing.T) {
	_, err := NewBackend(&BackendConfig{Type: "http"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

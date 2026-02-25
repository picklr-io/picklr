package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_ReadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.pkl")

	evaluator := eval.NewEvaluator(tmpDir)
	mgr := NewManager(statePath, evaluator)
	ctx := context.Background()

	// 1. Read non-existent state
	s, err := mgr.Read(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, s.Version)
	assert.Equal(t, 0, s.Serial)

	// 2. Write state
	s.Lineage = "test-lineage"
	s.Resources = []*ir.ResourceState{
		{
			Type:       "aws.s3.Bucket",
			Name:       "my-bucket",
			Provider:   "aws",
			InputsHash: "hash123",
		},
	}

	err = mgr.Write(ctx, s)
	require.NoError(t, err)

	// Verify file exists
	_, err = os.Stat(statePath)
	require.NoError(t, err)

	// 3. Read back and verify content
	content, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `type = "aws.s3.Bucket"`)
	assert.Contains(t, string(content), `name = "my-bucket"`)
}

func TestManager_WriteWithInputsOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.pkl")

	evaluator := eval.NewEvaluator(tmpDir)
	mgr := NewManager(statePath, evaluator)
	ctx := context.Background()

	s := &ir.State{
		Version: 1,
		Serial:  5,
		Lineage: "test-lineage-uuid",
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "test1",
				Provider: "null",
				Inputs: map[string]any{
					"triggers": map[string]any{"key": "value"},
				},
				Outputs: map[string]any{
					"id":       "null-test1",
					"triggers": map[string]any{"key": "value"},
				},
			},
		},
		Outputs: map[string]any{
			"test_output": "hello",
		},
	}

	err := mgr.Write(ctx, s)
	require.NoError(t, err)

	content, err := os.ReadFile(statePath)
	require.NoError(t, err)

	// Verify inputs are serialized (not empty)
	contentStr := string(content)
	assert.Contains(t, contentStr, `"triggers"`)
	assert.Contains(t, contentStr, `"key"`)
	assert.Contains(t, contentStr, `"value"`)
	assert.Contains(t, contentStr, `"id"`)
	assert.Contains(t, contentStr, `"null-test1"`)
	assert.Contains(t, contentStr, `"test_output"`)
	assert.Contains(t, contentStr, `"hello"`)
	// Ensure inputs are NOT empty
	assert.NotContains(t, contentStr, "inputs = new {}\n    inputsHash")
}

func TestManager_LockUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.pkl")

	evaluator := eval.NewEvaluator(tmpDir)
	mgr := NewManager(statePath, evaluator)

	// Lock should succeed
	err := mgr.Lock()
	require.NoError(t, err)

	// Second lock should fail
	mgr2 := NewManager(statePath, evaluator)
	err = mgr2.Lock()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "locked")

	// Unlock should succeed
	err = mgr.Unlock()
	require.NoError(t, err)

	// Now lock should succeed again
	err = mgr2.Lock()
	require.NoError(t, err)
	mgr2.Unlock()
}

func TestSerializePklValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		contains []string
	}{
		{
			name:     "string",
			input:    "hello",
			contains: []string{`"hello"`},
		},
		{
			name:     "bool",
			input:    true,
			contains: []string{"true"},
		},
		{
			name:     "int",
			input:    42,
			contains: []string{"42"},
		},
		{
			name:     "float as int",
			input:    float64(42),
			contains: []string{"42"},
		},
		{
			name:     "float",
			input:    3.14,
			contains: []string{"3.14"},
		},
		{
			name:     "nil",
			input:    nil,
			contains: []string{"null"},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			contains: []string{"new {}"},
		},
		{
			name:  "nested map",
			input: map[string]any{"key": "val"},
			contains: []string{
				"new {",
				`"key"`,
				`"val"`,
			},
		},
		{
			name:     "empty list",
			input:    []any{},
			contains: []string{"new Listing {}"},
		},
		{
			name:  "list",
			input: []any{"a", "b"},
			contains: []string{
				"new Listing {",
				`"a"`,
				`"b"`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serializePklValue(tt.input, 0)
			for _, c := range tt.contains {
				assert.Contains(t, result, c)
			}
		})
	}
}

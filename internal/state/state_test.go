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

	// 3. Read back (Mocking or verifying content)
	// Since we can't easily evaluate the generated PKL without real dependencies,
	// checking file content is a good proxy for now.
	content, err := os.ReadFile(statePath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `type = "aws.s3.Bucket"`)
	assert.Contains(t, string(content), `name = "my-bucket"`)
}

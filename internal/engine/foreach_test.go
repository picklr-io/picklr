package engine

import (
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandForEach_NoIteration(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", Properties: map[string]any{"key": "val"}},
	}
	expanded := ExpandForEach(resources)
	assert.Len(t, expanded, 1)
	assert.Equal(t, "a", expanded[0].Name)
}

func TestExpandForEach_Count(t *testing.T) {
	resources := []*ir.Resource{
		{
			Type:     "null_resource",
			Name:     "server",
			Provider: "null",
			Count:    3,
			Properties: map[string]any{
				"index": "${count.index}",
			},
		},
	}
	expanded := ExpandForEach(resources)
	require.Len(t, expanded, 3)

	assert.Equal(t, "server[0]", expanded[0].Name)
	assert.Equal(t, "0", expanded[0].Properties["index"])

	assert.Equal(t, "server[1]", expanded[1].Name)
	assert.Equal(t, "1", expanded[1].Properties["index"])

	assert.Equal(t, "server[2]", expanded[2].Name)
	assert.Equal(t, "2", expanded[2].Properties["index"])
}

func TestExpandForEach_ForEach(t *testing.T) {
	resources := []*ir.Resource{
		{
			Type:     "aws:S3.Bucket",
			Name:     "bucket",
			Provider: "aws",
			ForEach: map[string]any{
				"logs":  "logs-bucket",
				"data":  "data-bucket",
			},
			Properties: map[string]any{
				"bucket": "${each.value}",
				"tag":    "${each.key}",
			},
		},
	}
	expanded := ExpandForEach(resources)
	require.Len(t, expanded, 2)

	// Order may vary due to map iteration
	names := make(map[string]bool)
	for _, r := range expanded {
		names[r.Name] = true
	}
	assert.True(t, names["bucket[\"logs\"]"])
	assert.True(t, names["bucket[\"data\"]"])
}

func TestExpandForEach_PreservesLifecycle(t *testing.T) {
	resources := []*ir.Resource{
		{
			Type:     "null_resource",
			Name:     "server",
			Provider: "null",
			Count:    2,
			Lifecycle: &ir.Lifecycle{
				PreventDestroy: true,
				IgnoreChanges:  []string{"tags"},
			},
			Properties: map[string]any{},
		},
	}
	expanded := ExpandForEach(resources)
	require.Len(t, expanded, 2)

	for _, r := range expanded {
		require.NotNil(t, r.Lifecycle)
		assert.True(t, r.Lifecycle.PreventDestroy)
		assert.Equal(t, []string{"tags"}, r.Lifecycle.IgnoreChanges)
	}
}

func TestTransitiveDeps(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", DependsOn: []string{"null_resource.b"}, Properties: map[string]any{}},
		{Type: "null_resource", Name: "b", Provider: "null", DependsOn: []string{"null_resource.c"}, Properties: map[string]any{}},
		{Type: "null_resource", Name: "c", Provider: "null", Properties: map[string]any{}},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	// a -> b -> c, so transitive deps of a should be {b, c}
	deps := dag.TransitiveDeps("null_resource.a")
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "null_resource.b")
	assert.Contains(t, deps, "null_resource.c")

	// b -> c, so transitive deps of b should be {c}
	deps = dag.TransitiveDeps("null_resource.b")
	assert.Len(t, deps, 1)
	assert.Contains(t, deps, "null_resource.c")

	// c has no deps
	deps = dag.TransitiveDeps("null_resource.c")
	assert.Empty(t, deps)
}

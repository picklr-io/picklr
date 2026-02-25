package engine

import (
	"context"
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyPlan_Create(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.test1",
				Action:  "CREATE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "test1",
					Provider: "null",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "b"},
					},
				},
			},
		},
		Summary: &ir.PlanSummary{Create: 1},
		Outputs: map[string]any{},
	}

	state := &ir.State{
		Version: 1,
	}

	newState, err := eng.ApplyPlan(ctx, plan, state)
	require.NoError(t, err)
	require.Len(t, newState.Resources, 1)
	assert.Equal(t, "null_resource", newState.Resources[0].Type)
	assert.Equal(t, "test1", newState.Resources[0].Name)
	assert.Equal(t, "null-test1", newState.Resources[0].Outputs["id"])
	assert.Equal(t, 1, newState.Serial)
}

func TestApplyPlan_Delete(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.test1",
				Action:  "DELETE",
				Prior: &ir.Resource{
					Type:     "null_resource",
					Name:     "test1",
					Provider: "null",
				},
			},
		},
		Summary: &ir.PlanSummary{Delete: 1},
		Outputs: map[string]any{},
	}

	state := &ir.State{
		Version: 1,
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "test1",
				Provider: "null",
				Outputs:  map[string]any{"id": "null-test1"},
			},
		},
	}

	newState, err := eng.ApplyPlan(ctx, plan, state)
	require.NoError(t, err)
	assert.Len(t, newState.Resources, 0)
}

func TestApplyPlan_Update_NoDuplicates(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.test1",
				Action:  "REPLACE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "test1",
					Provider: "null",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "new_value"},
					},
				},
			},
		},
		Summary: &ir.PlanSummary{Replace: 1},
		Outputs: map[string]any{},
	}

	state := &ir.State{
		Version: 1,
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "test1",
				Provider: "null",
				Outputs:  map[string]any{"id": "null-test1", "triggers": map[string]any{"a": "old_value"}},
			},
		},
	}

	newState, err := eng.ApplyPlan(ctx, plan, state)
	require.NoError(t, err)
	// Should still have exactly 1 resource, not 2 (no duplicate)
	assert.Len(t, newState.Resources, 1)
	assert.Equal(t, "null-test1", newState.Resources[0].Outputs["id"])
}

func TestApplyPlan_ProgressCallback(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.test1",
				Action:  "CREATE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "test1",
					Provider: "null",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "b"},
					},
				},
			},
		},
		Summary: &ir.PlanSummary{Create: 1},
		Outputs: map[string]any{},
	}

	state := &ir.State{Version: 1}

	var events []ApplyEvent
	callback := func(event ApplyEvent) {
		events = append(events, event)
	}

	_, err := eng.ApplyPlanWithCallback(ctx, plan, state, callback)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "started", events[0].Status)
	assert.Equal(t, "completed", events[1].Status)
	assert.Equal(t, "null_resource.test1", events[0].Address)
}

func TestApplyPlan_ContinueOnError(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	eng.ContinueOnError = true
	ctx := context.Background()

	// Create a plan with two independent resources: one valid, one with a bad provider
	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.good",
				Action:  "CREATE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "good",
					Provider: "null",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "b"},
					},
				},
			},
			{
				Address: "null_resource.bad",
				Action:  "CREATE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "bad",
					Provider: "nonexistent",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "b"},
					},
				},
			},
		},
		Summary: &ir.PlanSummary{Create: 2},
		Outputs: map[string]any{},
	}

	state := &ir.State{Version: 1}

	newState, err := eng.ApplyPlanWithCallback(ctx, plan, state, nil)
	// Should get an error about the bad resource
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed")
	// The good resource should still have been applied
	assert.GreaterOrEqual(t, len(newState.Resources), 1)
}

func TestApplyPlan_FailFastByDefault(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	// ContinueOnError is false by default
	ctx := context.Background()

	plan := &ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: "null_resource.bad",
				Action:  "CREATE",
				Desired: &ir.Resource{
					Type:     "null_resource",
					Name:     "bad",
					Provider: "nonexistent",
					Properties: map[string]any{
						"triggers": map[string]any{"a": "b"},
					},
				},
			},
		},
		Summary: &ir.PlanSummary{Create: 1},
		Outputs: map[string]any{},
	}

	state := &ir.State{Version: 1}

	_, err := eng.ApplyPlan(ctx, plan, state)
	require.Error(t, err)
}

func TestApplyPlan_ResolveReferences(t *testing.T) {
	state := &ir.State{
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "test",
				Provider: "null",
				Outputs:  map[string]any{"id": "null-test", "value": "resolved"},
			},
		},
	}

	// Test resolving a ptr:// reference
	result := resolveReferences("ptr://null:null_resource/test/id", state)
	assert.Equal(t, "null-test", result)

	result = resolveReferences("ptr://null:null_resource/test/value", state)
	assert.Equal(t, "resolved", result)

	// Test non-reference stays unchanged
	result = resolveReferences("plain-string", state)
	assert.Equal(t, "plain-string", result)

	// Test nested map resolution
	result = resolveReferences(map[string]any{
		"ref":  "ptr://null:null_resource/test/id",
		"name": "test",
	}, state)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "null-test", m["ref"])
	assert.Equal(t, "test", m["name"])

	// Test list resolution
	result = resolveReferences([]any{
		"ptr://null:null_resource/test/id",
		"literal",
	}, state)
	list, ok := result.([]any)
	require.True(t, ok)
	assert.Equal(t, "null-test", list[0])
	assert.Equal(t, "literal", list[1])
}

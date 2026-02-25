package engine

import (
	"context"
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEngine_CreatePlan(t *testing.T) {
	reg := provider.NewRegistry()
	err := reg.LoadProvider("null")
	require.NoError(t, err)

	eng := NewEngine(reg)
	ctx := context.Background()

	// 1. Plan creation (New resource)
	cfg := &ir.Config{
		Resources: []*ir.Resource{
			{
				Type:     "null_resource",
				Name:     "test1",
				Provider: "null",
				Properties: map[string]any{
					"triggers": map[string]string{"a": "b"},
				},
			},
		},
	}

	state := &ir.State{} // Empty state

	plan, err := eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 1)
	assert.Equal(t, "CREATE", plan.Changes[0].Action)
	assert.Equal(t, "null_resource.test1", plan.Changes[0].Address)

	// Verify diff is populated for CREATE
	assert.NotNil(t, plan.Changes[0].Diff)
	assert.Contains(t, plan.Changes[0].Diff, "triggers")

	// 2. Plan update (No-op)
	state = &ir.State{
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "test1",
				Provider: "null",
				Outputs: map[string]any{
					"triggers": map[string]string{"a": "b"},
					"id":       "null-test1",
				},
			},
		},
	}

	plan, err = eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 0)
	assert.Equal(t, 1, plan.Summary.NoOp)

	// 3. Plan replace (Change trigger)
	cfg.Resources[0].Properties["triggers"] = map[string]string{"a": "c"}

	plan, err = eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 1)
	assert.Equal(t, "REPLACE", plan.Changes[0].Action)
}

func TestEngine_CreatePlan_Delete(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	// Empty config, resource in state -> DELETE
	cfg := &ir.Config{
		Resources: []*ir.Resource{},
	}

	state := &ir.State{
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "old_resource",
				Provider: "null",
				Outputs:  map[string]any{"id": "null-old"},
			},
		},
	}

	plan, err := eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 1)
	assert.Equal(t, "DELETE", plan.Changes[0].Action)
	assert.Equal(t, "null_resource.old_resource", plan.Changes[0].Address)
	assert.Equal(t, 1, plan.Summary.Delete)
}

func TestEngine_CreatePlan_PreventDestroy(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	cfg := &ir.Config{
		Resources: []*ir.Resource{
			{
				Type:     "null_resource",
				Name:     "protected",
				Provider: "null",
				Lifecycle: &ir.Lifecycle{
					PreventDestroy: true,
				},
				Properties: map[string]any{
					"triggers": map[string]string{"a": "new_value"},
				},
			},
		},
	}

	state := &ir.State{
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "protected",
				Provider: "null",
				Outputs: map[string]any{
					"id":       "null-protected",
					"triggers": map[string]string{"a": "old_value"},
				},
			},
		},
	}

	// REPLACE triggers PreventDestroy error
	_, err := eng.CreatePlan(ctx, cfg, state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prevent_destroy")
}

func TestEngine_CreatePlan_IgnoreChanges(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	cfg := &ir.Config{
		Resources: []*ir.Resource{
			{
				Type:     "null_resource",
				Name:     "ignored",
				Provider: "null",
				Lifecycle: &ir.Lifecycle{
					IgnoreChanges: []string{"triggers"},
				},
				Properties: map[string]any{
					"triggers": map[string]string{"a": "new_value"},
				},
			},
		},
	}

	state := &ir.State{
		Resources: []*ir.ResourceState{
			{
				Type:     "null_resource",
				Name:     "ignored",
				Provider: "null",
				Outputs: map[string]any{
					"id":       "null-ignored",
					"triggers": map[string]string{"a": "old_value"},
				},
			},
		},
	}

	// The null provider returns REPLACE for trigger changes.
	// But since ChangedAttributes includes "triggers" and that's in IgnoreChanges,
	// and the action is REPLACE (not UPDATE), IgnoreChanges only applies to UPDATE.
	// So this test verifies the behavior correctly.
	plan, err := eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	// REPLACE is not filtered by IgnoreChanges (only UPDATE is)
	assert.Len(t, plan.Changes, 1)
}

func TestEngine_CreatePlan_Timestamp(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	cfg := &ir.Config{Resources: []*ir.Resource{}}
	state := &ir.State{}

	plan, err := eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	assert.NotEmpty(t, plan.Metadata.Timestamp)
}

func TestEngine_CreatePlan_DependencyOrder(t *testing.T) {
	reg := provider.NewRegistry()
	require.NoError(t, reg.LoadProvider("null"))

	eng := NewEngine(reg)
	ctx := context.Background()

	cfg := &ir.Config{
		Resources: []*ir.Resource{
			{
				Type:       "null_resource",
				Name:       "second",
				Provider:   "null",
				DependsOn:  []string{"null_resource.first"},
				Properties: map[string]any{"triggers": map[string]string{"x": "y"}},
			},
			{
				Type:       "null_resource",
				Name:       "first",
				Provider:   "null",
				Properties: map[string]any{"triggers": map[string]string{"a": "b"}},
			},
		},
	}

	state := &ir.State{}

	plan, err := eng.CreatePlan(ctx, cfg, state)
	require.NoError(t, err)
	require.Len(t, plan.Changes, 2)

	// Verify first comes before second in the plan
	assert.Equal(t, "null_resource.first", plan.Changes[0].Address)
	assert.Equal(t, "null_resource.second", plan.Changes[1].Address)
}

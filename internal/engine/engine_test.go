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
	// Setup registry with null provider
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

	// 2. Plan update (No-op)
	// Mock state that matches config
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

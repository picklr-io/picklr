package null

import (
	"context"
	"encoding/json"
	"testing"

	pb "github.com/picklr-io/picklr/pkg/proto/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProvider_Plan(t *testing.T) {
	p := New()
	ctx := context.Background()

	// 1. Create plan (New resource)
	desired := Config{Triggers: map[string]string{"foo": "bar"}}
	desiredJSON, _ := json.Marshal(desired)

	resp, err := p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: desiredJSON,
		PriorStateJson:    nil,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_CREATE, resp.Action)

	// 2. No-op plan (Same triggers)
	state := State{
		ID:       "null-test",
		Triggers: desired.Triggers,
	}
	stateJSON, _ := json.Marshal(state)

	resp, err = p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: desiredJSON,
		PriorStateJson:    stateJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_NOOP, resp.Action)

	// 3. Update plan (Changed triggers -> Replace)
	newDesired := Config{Triggers: map[string]string{"foo": "baz"}}
	newDesiredJSON, _ := json.Marshal(newDesired)

	resp, err = p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: newDesiredJSON,
		PriorStateJson:    stateJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_REPLACE, resp.Action)
	assert.Contains(t, resp.ChangedAttributes, "triggers")
}

func TestProvider_Apply(t *testing.T) {
	p := New()
	ctx := context.Background()

	desired := Config{Triggers: map[string]string{"foo": "bar"}}
	desiredJSON, _ := json.Marshal(desired)

	resp, err := p.Apply(ctx, &pb.ApplyRequest{
		Name:              "test",
		DesiredConfigJson: desiredJSON,
	})
	require.NoError(t, err)

	var newState State
	err = json.Unmarshal(resp.NewStateJson, &newState)
	require.NoError(t, err)
	assert.Equal(t, "null-test", newState.ID)
	assert.Equal(t, "bar", newState.Triggers["foo"])
}

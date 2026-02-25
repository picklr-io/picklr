package null

import (
	"context"
	"encoding/json"
	"testing"

	pb "github.com/picklr-io/picklr/pkg/proto/provider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Provider conformance test suite.
// These tests verify that a provider correctly implements the full lifecycle:
// Configure -> Plan (CREATE) -> Apply -> Read -> Plan (NOOP) -> Plan (UPDATE) -> Apply -> Delete

func TestConformance_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	p := New()

	// 1. Configure
	configResp, err := p.Configure(ctx, &pb.ConfigureRequest{})
	require.NoError(t, err)
	assert.Empty(t, configResp.Diagnostics)

	// 2. Plan (CREATE) - no prior state
	desired := map[string]any{"triggers": map[string]string{"key": "value"}}
	desiredJSON, _ := json.Marshal(desired)

	planResp, err := p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: desiredJSON,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_CREATE, planResp.Action)

	// 3. Apply
	applyResp, err := p.Apply(ctx, &pb.ApplyRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: desiredJSON,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, applyResp.NewStateJson)

	var state map[string]any
	require.NoError(t, json.Unmarshal(applyResp.NewStateJson, &state))
	assert.NotEmpty(t, state["id"])

	// 4. Read
	readResp, err := p.Read(ctx, &pb.ReadRequest{
		Type:             "null_resource",
		Id:               state["id"].(string),
		CurrentStateJson: applyResp.NewStateJson,
	})
	require.NoError(t, err)
	assert.True(t, readResp.Exists)

	// 5. Plan (NOOP) - same desired as current
	planResp2, err := p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: desiredJSON,
		PriorStateJson:    applyResp.NewStateJson,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_NOOP, planResp2.Action)

	// 6. Plan (UPDATE/REPLACE) - changed triggers
	newDesired := map[string]any{"triggers": map[string]string{"key": "new-value"}}
	newDesiredJSON, _ := json.Marshal(newDesired)

	planResp3, err := p.Plan(ctx, &pb.PlanRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: newDesiredJSON,
		PriorStateJson:    applyResp.NewStateJson,
	})
	require.NoError(t, err)
	assert.Equal(t, pb.PlanResponse_REPLACE, planResp3.Action)

	// 7. Apply update
	applyResp2, err := p.Apply(ctx, &pb.ApplyRequest{
		Type:              "null_resource",
		Name:              "test",
		DesiredConfigJson: newDesiredJSON,
		PriorStateJson:    applyResp.NewStateJson,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, applyResp2.NewStateJson)

	// 8. Delete
	deleteResp, err := p.Delete(ctx, &pb.DeleteRequest{
		Type:             "null_resource",
		Id:               state["id"].(string),
		CurrentStateJson: applyResp2.NewStateJson,
	})
	require.NoError(t, err)
	assert.NotNil(t, deleteResp)
}

func TestConformance_GetSchema(t *testing.T) {
	ctx := context.Background()
	p := New()

	resp, err := p.GetSchema(ctx, &pb.GetSchemaRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.PklSchema)
	assert.NotEmpty(t, resp.PklVersion)
}

func TestConformance_ConfigureIdempotent(t *testing.T) {
	ctx := context.Background()
	p := New()

	// Configure should be idempotent
	for i := 0; i < 3; i++ {
		resp, err := p.Configure(ctx, &pb.ConfigureRequest{})
		require.NoError(t, err)
		assert.Empty(t, resp.Diagnostics)
	}
}

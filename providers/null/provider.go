package null

import (
	"context"
	"encoding/json"
	"fmt"

	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type Provider struct {
	pb.UnimplementedProviderServer
}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) GetSchema(ctx context.Context, req *pb.GetSchemaRequest) (*pb.GetSchemaResponse, error) {
	// Ideally we load this from the .pkl file we created, or embed it.
	// For now, we'll return a placeholder or empty since we aren't fully using this yet.
	return &pb.GetSchemaResponse{
		PklSchema:  "import \"...\"",
		PklVersion: "0.25.0",
	}, nil
}

func (p *Provider) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	return &pb.ConfigureResponse{}, nil
}

func (p *Provider) Plan(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {
	// For null provider, we just diff the triggers
	// In a real provider, we'd check if resource exists, etc.

	var desired Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	var prior State
	if len(req.PriorStateJson) > 0 {
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
	}

	// Simple logic: if triggers changed, replace. If new, create.
	action := pb.PlanResponse_NOOP
	var changes []string

	if req.PriorStateJson == nil {
		action = pb.PlanResponse_CREATE
	} else {
		// Compare triggers
		if !equal(desired.Triggers, prior.Triggers) {
			action = pb.PlanResponse_REPLACE
			changes = append(changes, "triggers")
		}
	}

	// If delete logic is needed (usually handled by core seeing nil desired), but PlanRequest has Desired.
	// If desired is nil/empty in a specific way implies delete?
	// Usually core determines delete if resource is missing from config.
	// Here PlanRequest assumes existence in config unless we define otherwise.
	// The core engine drives delete.

	return &pb.PlanResponse{
		Action:            action,
		ChangedAttributes: changes,
	}, nil
}

func (p *Provider) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// Just return the config as state
	var desired Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	state := State{
		Triggers: desired.Triggers,
	}
	// Add an ID
	state.ID = fmt.Sprintf("null-%s", req.Name)

	stateBytes, _ := json.Marshal(state)

	return &pb.ApplyResponse{
		NewStateJson: stateBytes,
	}, nil
}

func (p *Provider) Read(ctx context.Context, req *pb.ReadRequest) (*pb.ReadResponse, error) {
	// Null provider resources don't really exist, but we can return the input as state if we wanted.
	return &pb.ReadResponse{
		Exists:       true,
		NewStateJson: req.CurrentStateJson,
	}, nil
}

func (p *Provider) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	return &pb.DeleteResponse{}, nil
}

// Internal structs for JSON handling
type Config struct {
	Triggers map[string]string `json:"triggers"`
}

type State struct {
	ID       string            `json:"id"`
	Triggers map[string]string `json:"triggers"`
}

func equal(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/picklr-io/picklr/internal/ir"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// ApplyPlan executes a plan and updates the state.
func (e *Engine) ApplyPlan(ctx context.Context, plan *ir.Plan, state *ir.State) (*ir.State, error) {
	// Build a lookup map for existing resources in state
	// internal/engine/apply.go

	// 1. Iterate over changes

	for _, change := range plan.Changes {
		addr := change.Address
		// Split address to get name/type if needed

		var desiredJSON []byte
		var priorJSON []byte
		var name, typ string

		// Infer type/name from address or change object
		// Change object should probably have full resource info.
		// ir.ResourceChange has Desired and Prior logic.
		// Let's use that.

		if change.Desired != nil {
			name = change.Desired.Name
			typ = change.Desired.Type
			name = change.Desired.Name
			typ = change.Desired.Type
			props := normalizeValue(change.Desired.Properties)

			// Resolve References
			resolvedProps := resolveReferences(props, state)

			desiredJSON, _ = json.Marshal(resolvedProps)

			// fmt.Printf("DEBUG APPLY: Resource %s JSON: %s\n", name, string(desiredJSON))
		} else if change.Prior != nil {
			name = change.Prior.Name
			typ = change.Prior.Type
		}

		if change.Prior != nil && state != nil {
			// Find existing state
			// But for deletion, prior is all we have.
			// Ideally we use the state passed in.
			// Or we rely on change.Prior having the state info.
			// change.Prior is ir.Resource, which might not have the provider output state.
			// FIXME: ir.ResourceChange.Prior is ir.Resource (config-like), not ir.ResourceState (state-like).
			// We need to look up the ResourceState from the input state.
			// Using the address.

			// For now, let's implement basic CREATE logic which is critical for MVP.
		}

		provName := "null" // FIXME: Get from resource
		if change.Desired != nil {
			provName = change.Desired.Provider
		} else if change.Prior != nil {
			provName = change.Prior.Provider
		}

		prov, err := e.registry.Get(provName)
		if err != nil {
			return nil, fmt.Errorf("provider not found: %s", provName)
		}

		switch change.Action {
		case "CREATE", "UPDATE", "REPLACE":
			resp, err := prov.Apply(ctx, &pb.ApplyRequest{
				Type:              typ, // "null_resource"
				Name:              name,
				DesiredConfigJson: desiredJSON,
				PriorStateJson:    priorJSON, // Need to implement prior logic
			})
			if err != nil {
				return nil, fmt.Errorf("apply failed for %s: %w", addr, err)
			}

			// Update state
			var outputs map[string]any
			if len(resp.NewStateJson) > 0 {
				if err := json.Unmarshal(resp.NewStateJson, &outputs); err != nil {
					return nil, fmt.Errorf("failed to unmarshal state: %w", err)
				}
			}

			newState := &ir.ResourceState{
				Type:     typ,
				Name:     name,
				Provider: provName,
				Inputs:   change.Desired.Properties, // Map
				Outputs:  outputs,
			}

			// Simple append or replace in state
			state.Resources = append(state.Resources, newState)

		case "DELETE":
			// Call Delete
			// Remove from state
		}
	}

	// Re-assign generic state fields if needed
	state.Serial++
	state.Outputs = plan.Outputs

	return state, nil
}

func resolveReferences(val any, state *ir.State) any {
	switch v := val.(type) {
	case string:
		// Check for ptr:// prefix
		// Format: ptr://${provider}:${type}/${name}/${attribute}
		if len(v) > 6 && v[:6] == "ptr://" {
			// Parse
			// We can just iterate state to find matching resource
			// Ideally parse properly.
			// for now, let's just loop state.
			// TODO: Optimize lookup

			// Expected: ptr://aws:EC2.Vpc/my-vpc/id
			// Logic:
			// 1. Remove ptr:// -> aws:EC2.Vpc/my-vpc/id
			// 2. We need to match this against state resources.
			//    State resources have Type, Name, Provider.
			//    Address in state is usually Type.Name

			// Let's assume the string contains enough info to match.
			// We need to match provider:type/name.

			for _, res := range state.Resources {
				// Construct ID string for resource to match
				// Res: Provider=aws, Type=EC2.Vpc, Name=my-vpc
				// MatchStr = aws:EC2.Vpc/my-vpc/

				matchPrefix := fmt.Sprintf("ptr://%s:%s/%s/", res.Provider, res.Type, res.Name)
				if len(v) > len(matchPrefix) && v[:len(matchPrefix)] == matchPrefix {
					// Match found!
					attr := v[len(matchPrefix):]

					// Look up attribute in outputs
					if val, ok := res.Outputs[attr]; ok {
						return val
					}
					if val, ok := res.Inputs[attr]; ok {
						return val
					}
					// Not found
					return v // or error?
				}
			}
		}
		return v
	case map[string]any:
		newMap := make(map[string]any)
		for k, v := range v {
			newMap[k] = resolveReferences(v, state)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(v))
		for i, v := range v {
			newSlice[i] = resolveReferences(v, state)
		}
		return newSlice
	default:
		return v
	}
}

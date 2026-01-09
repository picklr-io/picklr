package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// Engine orchestrates the lifecycle of resources.
type Engine struct {
	registry *provider.Registry
}

func NewEngine(registry *provider.Registry) *Engine {
	return &Engine{
		registry: registry,
	}
}

// CreatePlan generates an execution plan by comparing desired config with current state.
func (e *Engine) CreatePlan(ctx context.Context, cfg *ir.Config, state *ir.State) (*ir.Plan, error) {
	plan := &ir.Plan{
		Metadata: &ir.PlanMetadata{
			// Timestamp and hashes would be set here
		},
		Changes: []*ir.ResourceChange{},
		Summary: &ir.PlanSummary{},
		Outputs: cfg.Outputs,
	}

	// 1. Load all required providers
	for _, res := range cfg.Resources {
		// Ensure provider is loaded
		if err := e.registry.LoadProvider(res.Provider); err != nil {
			return nil, fmt.Errorf("failed to load provider %s: %w", res.Provider, err)
		}
	}

	// 2. Build state map for quick lookup
	stateMap := make(map[string]*ir.ResourceState)
	for _, res := range state.Resources {
		// Address: type.name (e.g., null_resource.my_test)
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		stateMap[addr] = res
	}

	// 3. Iterate desired resources (Create/Update logic)
	for _, res := range cfg.Resources {
		// fmt.Printf("DEBUG: Resource %s Properties: %+v\n", res.Name, res.Properties)

		// Infer or use Type.
		resourceType := res.Type
		if resourceType == "" {
			// Fallback for null provider if missing
			resourceType = "null_resource"
		}

		addr := fmt.Sprintf("%s.%s", resourceType, res.Name)

		prov, err := e.registry.Get(res.Provider)
		if err != nil {
			return nil, err
		}

		// Prepare request
		props := normalizeValue(res.Properties)
		desiredJSON, err := json.Marshal(props)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal properties for %s: %w", res.Name, err)
		}

		// fmt.Printf("DEBUG: Resource %s JSON: %s\n", res.Name, string(desiredJSON))

		var priorJSON []byte
		if prior, ok := stateMap[addr]; ok {
			// Pass prior outputs as state
			priorJSON, _ = json.Marshal(prior.Outputs)
		}

		resp, err := prov.Plan(ctx, &pb.PlanRequest{
			Type:              resourceType,
			Name:              res.Name,
			DesiredConfigJson: desiredJSON,
			PriorStateJson:    priorJSON,
		})
		if err != nil {
			return nil, fmt.Errorf("plan failed for %s: %w", addr, err)
		}

		if resp.Action != pb.PlanResponse_NOOP {
			change := &ir.ResourceChange{
				Address: addr,
				Action:  resp.Action.String(), // "CREATE", "UPDATE", "REPLACE"
				Desired: res,                  // Set Desired
				// Prior: ... (prior is ResourceState, not Resource. We need to map properly or at least set something)
				// For now, Desired is what matters for Create/Update.
			}
			// If we have prior, ideally we map it to Resource struct if possible, or we need to change IR?
			// The IR expects *Resource.
			// ResourceState has Inputs. We can reconstruct Resource from ResourceState inputs.
			if prior, ok := stateMap[addr]; ok {
				change.Prior = &ir.Resource{
					Type:       prior.Type,
					Name:       prior.Name,
					Provider:   prior.Provider,
					Properties: prior.Inputs,
				}
			}
			plan.Changes = append(plan.Changes, change)

			switch resp.Action {
			case pb.PlanResponse_CREATE:
				plan.Summary.Create++
			case pb.PlanResponse_UPDATE:
				plan.Summary.Update++
			case pb.PlanResponse_REPLACE:
				plan.Summary.Replace++
			case pb.PlanResponse_DELETE:
				plan.Summary.Delete++
			}
		} else {
			plan.Summary.NoOp++
		}
	}

	// 4. Handle Deletions
	// Iterate valid resources in state, if not in config, plan delete.
	configMap := make(map[string]bool)
	for _, res := range cfg.Resources {
		resourceType := res.Type
		if resourceType == "" {
			resourceType = "null_resource"
		}
		addr := fmt.Sprintf("%s.%s", resourceType, res.Name)
		configMap[addr] = true
	}

	for _, res := range state.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if !configMap[addr] {
			// Resource exists in state but not in config -> DELETE
			prov, err := e.registry.Get(res.Provider)
			if err == nil {
				priorJSON, _ := json.Marshal(res.Outputs)
				resp, err := prov.Plan(ctx, &pb.PlanRequest{
					Type:              res.Type,
					Name:              res.Name,
					DesiredConfigJson: nil, // Indicates deletion
					PriorStateJson:    priorJSON,
				})

				if err == nil && resp.Action == pb.PlanResponse_DELETE {
					change := &ir.ResourceChange{
						Address: addr,
						Action:  "DELETE",
					}
					plan.Changes = append(plan.Changes, change)
					plan.Summary.Delete++
				} else {
					change := &ir.ResourceChange{
						Address: addr,
						Action:  "DELETE",
					}
					plan.Changes = append(plan.Changes, change)
					plan.Summary.Delete++
				}
			}
		}
	}

	return plan, nil
}

func normalizeValue(v any) any {
	switch val := v.(type) {
	case map[any]any:
		newMap := make(map[string]any)
		for k, v := range val {
			newMap[fmt.Sprintf("%v", k)] = normalizeValue(v)
		}
		return newMap
	case map[string]any:
		newMap := make(map[string]any)
		for k, v := range val {
			newMap[k] = normalizeValue(v)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(val))
		for i, v := range val {
			newSlice[i] = normalizeValue(v)
		}
		return newSlice
	default:
		return val
	}
}

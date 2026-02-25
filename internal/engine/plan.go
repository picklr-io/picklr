package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/logging"
	"github.com/picklr-io/picklr/internal/provider"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// Engine orchestrates the lifecycle of resources.
type Engine struct {
	registry        *provider.Registry
	ContinueOnError bool // If true, apply continues past failures instead of stopping
}

func NewEngine(registry *provider.Registry) *Engine {
	return &Engine{
		registry: registry,
	}
}

// CreatePlan generates an execution plan by comparing desired config with current state.
func (e *Engine) CreatePlan(ctx context.Context, cfg *ir.Config, state *ir.State) (*ir.Plan, error) {
	return e.CreatePlanWithTargets(ctx, cfg, state, nil)
}

// CreatePlanWithTargets generates a plan filtered to specific resource addresses.
// If targets is nil or empty, all resources are planned.
func (e *Engine) CreatePlanWithTargets(ctx context.Context, cfg *ir.Config, state *ir.State, targets []string) (*ir.Plan, error) {
	logging.Debug("creating plan", "resources", len(cfg.Resources), "state_resources", len(state.Resources), "targets", len(targets))
	plan := &ir.Plan{
		Metadata: &ir.PlanMetadata{
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Changes: []*ir.ResourceChange{},
		Summary: &ir.PlanSummary{},
		Outputs: cfg.Outputs,
	}

	// 1. Load all required providers
	for _, res := range cfg.Resources {
		if err := e.registry.LoadProvider(res.Provider); err != nil {
			return nil, fmt.Errorf("failed to load provider %s: %w", res.Provider, err)
		}
	}

	// 1.5 Expand for_each/count resources
	cfg.Resources = ExpandForEach(cfg.Resources)

	// 2. Build dependency graph for ordering
	dag, err := BuildDAG(cfg.Resources)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// 3. Build state map for quick lookup
	stateMap := make(map[string]*ir.ResourceState)
	for _, res := range state.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		stateMap[addr] = res
	}

	// 4. Build config map for quick lookup
	configByAddr := make(map[string]*ir.Resource)
	for _, res := range cfg.Resources {
		addr := resourceAddr(res)
		configByAddr[addr] = res
	}

	// 5. Build target set (if targets specified, include their dependencies)
	var targetSet map[string]bool
	if len(targets) > 0 {
		targetSet = make(map[string]bool)
		for _, t := range targets {
			targetSet[t] = true
		}
		// Add transitive dependencies of targets
		for _, t := range targets {
			for _, dep := range dag.TransitiveDeps(t) {
				targetSet[dep] = true
			}
		}
	}

	// 6. Iterate desired resources in dependency order
	for _, addr := range dag.CreationOrder() {
		res, ok := configByAddr[addr]
		if !ok {
			continue
		}

		// Skip non-targeted resources
		if targetSet != nil && !targetSet[addr] {
			plan.Summary.NoOp++
			continue
		}

		resourceType := res.Type
		if resourceType == "" {
			resourceType = "null_resource"
		}

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

		var priorJSON []byte
		if prior, ok := stateMap[addr]; ok {
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
			// Enforce lifecycle rules
			if err := enforceLifecycle(res, resp.Action, addr); err != nil {
				return nil, err
			}

			// Apply IgnoreChanges filtering
			action := resp.Action
			if res.Lifecycle != nil && len(res.Lifecycle.IgnoreChanges) > 0 && action == pb.PlanResponse_UPDATE {
				action = filterIgnoredChanges(res, resp, stateMap[addr])
			}

			if action == pb.PlanResponse_NOOP {
				plan.Summary.NoOp++
				continue
			}

			change := &ir.ResourceChange{
				Address: addr,
				Action:  action.String(),
				Desired: res,
			}

			if prior, ok := stateMap[addr]; ok {
				change.Prior = &ir.Resource{
					Type:       prior.Type,
					Name:       prior.Name,
					Provider:   prior.Provider,
					Properties: prior.Inputs,
				}
				change.Diff = buildPropertyDiff(prior.Inputs, res.Properties)
			} else {
				change.Diff = buildCreateDiff(res.Properties)
			}

			plan.Changes = append(plan.Changes, change)

			switch action {
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

	// 7. Handle Deletions (resources in state but not in config)
	configMap := make(map[string]bool)
	for _, res := range cfg.Resources {
		addr := resourceAddr(res)
		configMap[addr] = true
	}

	for _, res := range state.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if !configMap[addr] {
			// Skip non-targeted resources for deletion too
			if targetSet != nil && !targetSet[addr] {
				continue
			}
			change := &ir.ResourceChange{
				Address: addr,
				Action:  "DELETE",
				Prior: &ir.Resource{
					Type:       res.Type,
					Name:       res.Name,
					Provider:   res.Provider,
					Properties: res.Inputs,
				},
				Diff: buildDeleteDiff(res.Inputs),
			}
			plan.Changes = append(plan.Changes, change)
			plan.Summary.Delete++
		}
	}

	return plan, nil
}

// enforceLifecycle checks lifecycle rules and returns an error if violated.
func enforceLifecycle(res *ir.Resource, action pb.PlanResponse_Action, addr string) error {
	if res.Lifecycle == nil {
		return nil
	}

	if res.Lifecycle.PreventDestroy && (action == pb.PlanResponse_DELETE || action == pb.PlanResponse_REPLACE) {
		return fmt.Errorf("resource %s has prevent_destroy set but plan requires destruction", addr)
	}

	return nil
}

// filterIgnoredChanges checks if all changed attributes are in IgnoreChanges.
// If so, downgrades the action to NOOP.
func filterIgnoredChanges(res *ir.Resource, resp *pb.PlanResponse, prior *ir.ResourceState) pb.PlanResponse_Action {
	if prior == nil || res.Lifecycle == nil {
		return resp.Action
	}

	ignoreSet := make(map[string]bool)
	for _, attr := range res.Lifecycle.IgnoreChanges {
		ignoreSet[attr] = true
	}

	if len(resp.ChangedAttributes) > 0 {
		allIgnored := true
		for _, attr := range resp.ChangedAttributes {
			if !ignoreSet[attr] {
				allIgnored = false
				break
			}
		}
		if allIgnored {
			return pb.PlanResponse_NOOP
		}
	}

	return resp.Action
}

// buildPropertyDiff compares prior and desired properties and returns a diff map.
func buildPropertyDiff(prior, desired map[string]any) map[string]*ir.PropertyDiff {
	diff := make(map[string]*ir.PropertyDiff)

	allKeys := make(map[string]bool)
	for k := range prior {
		allKeys[k] = true
	}
	for k := range desired {
		allKeys[k] = true
	}

	for k := range allKeys {
		priorVal, inPrior := prior[k]
		desiredVal, inDesired := desired[k]

		if !inPrior {
			diff[k] = &ir.PropertyDiff{
				After:  desiredVal,
				Action: "create",
			}
		} else if !inDesired {
			diff[k] = &ir.PropertyDiff{
				Before: priorVal,
				Action: "delete",
			}
		} else if fmt.Sprintf("%v", priorVal) != fmt.Sprintf("%v", desiredVal) {
			diff[k] = &ir.PropertyDiff{
				Before: priorVal,
				After:  desiredVal,
				Action: "update",
			}
		}
	}

	return diff
}

func buildCreateDiff(props map[string]any) map[string]*ir.PropertyDiff {
	diff := make(map[string]*ir.PropertyDiff)
	for k, v := range props {
		diff[k] = &ir.PropertyDiff{
			After:  v,
			Action: "create",
		}
	}
	return diff
}

func buildDeleteDiff(props map[string]any) map[string]*ir.PropertyDiff {
	diff := make(map[string]*ir.PropertyDiff)
	for k, v := range props {
		diff[k] = &ir.PropertyDiff{
			Before: v,
			Action: "delete",
		}
	}
	return diff
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

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/picklr-io/picklr/internal/ir"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// ApplyEvent represents a progress event during apply.
type ApplyEvent struct {
	Address  string
	Action   string
	Status   string // "started", "completed", "failed"
	Duration time.Duration
	Error    error
}

// ApplyCallback is called for each apply event if set.
type ApplyCallback func(event ApplyEvent)

// ApplyPlan executes a plan and updates the state.
func (e *Engine) ApplyPlan(ctx context.Context, plan *ir.Plan, state *ir.State) (*ir.State, error) {
	return e.ApplyPlanWithCallback(ctx, plan, state, nil)
}

// ApplyPlanWithCallback executes a plan with progress event callbacks.
func (e *Engine) ApplyPlanWithCallback(ctx context.Context, plan *ir.Plan, state *ir.State, callback ApplyCallback) (*ir.State, error) {
	var mu sync.Mutex

	emit := func(event ApplyEvent) {
		if callback != nil {
			callback(event)
		}
	}

	// Build a lookup map for existing resources in state by address
	stateIndex := make(map[string]int)
	for i, res := range state.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		stateIndex[addr] = i
	}

	// Group changes: separate creates/updates from deletes
	// Deletes happen after creates (for REPLACE, create-before-destroy)
	var createUpdates, deletes []*ir.ResourceChange
	for _, change := range plan.Changes {
		if change.Action == "DELETE" {
			deletes = append(deletes, change)
		} else {
			createUpdates = append(createUpdates, change)
		}
	}

	// Apply creates/updates (sequentially to respect dependency order, which is already sorted)
	for _, change := range createUpdates {
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("apply cancelled: %w", err)
		}

		start := time.Now()
		emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "started"})

		if err := e.applyChange(ctx, change, state, &stateIndex, &mu); err != nil {
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "failed", Duration: time.Since(start), Error: err})
			return state, err
		}

		emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "completed", Duration: time.Since(start)})
	}

	// Apply deletes
	for _, change := range deletes {
		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("apply cancelled: %w", err)
		}

		start := time.Now()
		emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "started"})

		if err := e.applyChange(ctx, change, state, &stateIndex, &mu); err != nil {
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "failed", Duration: time.Since(start), Error: err})
			return state, err
		}

		emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "completed", Duration: time.Since(start)})
	}

	state.Serial++
	state.Outputs = plan.Outputs

	return state, nil
}

func (e *Engine) applyChange(ctx context.Context, change *ir.ResourceChange, state *ir.State, stateIndex *map[string]int, mu *sync.Mutex) error {
	addr := change.Address

	var desiredJSON []byte
	var priorJSON []byte
	var name, typ string

	if change.Desired != nil {
		name = change.Desired.Name
		typ = change.Desired.Type
		props := normalizeValue(change.Desired.Properties)
		resolvedProps := resolveReferences(props, state)
		desiredJSON, _ = json.Marshal(resolvedProps)
	} else if change.Prior != nil {
		name = change.Prior.Name
		typ = change.Prior.Type
	}

	mu.Lock()
	if idx, ok := (*stateIndex)[addr]; ok {
		priorState := state.Resources[idx]
		if priorState.Outputs != nil {
			priorJSON, _ = json.Marshal(priorState.Outputs)
		}
	}
	mu.Unlock()

	provName := "null"
	if change.Desired != nil {
		provName = change.Desired.Provider
	} else if change.Prior != nil {
		provName = change.Prior.Provider
	}

	prov, err := e.registry.Get(provName)
	if err != nil {
		return fmt.Errorf("provider not found: %s", provName)
	}

	switch change.Action {
	case "CREATE", "UPDATE", "REPLACE":
		resp, err := prov.Apply(ctx, &pb.ApplyRequest{
			Type:              typ,
			Name:              name,
			DesiredConfigJson: desiredJSON,
			PriorStateJson:    priorJSON,
		})
		if err != nil {
			return fmt.Errorf("apply failed for %s: %w", addr, err)
		}

		var outputs map[string]any
		if len(resp.NewStateJson) > 0 {
			if err := json.Unmarshal(resp.NewStateJson, &outputs); err != nil {
				return fmt.Errorf("failed to unmarshal state: %w", err)
			}
		}

		newResState := &ir.ResourceState{
			Type:     typ,
			Name:     name,
			Provider: provName,
			Inputs:   change.Desired.Properties,
			Outputs:  outputs,
		}

		mu.Lock()
		if idx, ok := (*stateIndex)[addr]; ok {
			state.Resources[idx] = newResState
		} else {
			(*stateIndex)[addr] = len(state.Resources)
			state.Resources = append(state.Resources, newResState)
		}
		mu.Unlock()

	case "DELETE":
		var resourceID string
		mu.Lock()
		if idx, ok := (*stateIndex)[addr]; ok {
			if id, exists := state.Resources[idx].Outputs["id"]; exists {
				resourceID = fmt.Sprintf("%v", id)
			}
		}
		mu.Unlock()

		_, err := prov.Delete(ctx, &pb.DeleteRequest{
			Type:             typ,
			Id:               resourceID,
			CurrentStateJson: priorJSON,
		})
		if err != nil {
			return fmt.Errorf("delete failed for %s: %w", addr, err)
		}

		mu.Lock()
		if idx, ok := (*stateIndex)[addr]; ok {
			state.Resources = append(state.Resources[:idx], state.Resources[idx+1:]...)
			// Rebuild index after removal
			*stateIndex = make(map[string]int)
			for i, res := range state.Resources {
				a := fmt.Sprintf("%s.%s", res.Type, res.Name)
				(*stateIndex)[a] = i
			}
		}
		mu.Unlock()
	}

	return nil
}

func resolveReferences(val any, state *ir.State) any {
	switch v := val.(type) {
	case string:
		if len(v) > 6 && v[:6] == "ptr://" {
			for _, res := range state.Resources {
				matchPrefix := fmt.Sprintf("ptr://%s:%s/%s/", res.Provider, res.Type, res.Name)
				if len(v) > len(matchPrefix) && v[:len(matchPrefix)] == matchPrefix {
					attr := v[len(matchPrefix):]
					if val, ok := res.Outputs[attr]; ok {
						return val
					}
					if val, ok := res.Inputs[attr]; ok {
						return val
					}
					return v
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

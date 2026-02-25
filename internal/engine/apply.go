package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/logging"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

const defaultParallelism = 10

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
// It applies resources in parallel respecting dependency ordering.
// If e.ContinueOnError is true, apply will continue past individual resource
// failures and return an aggregated error at the end.
func (e *Engine) ApplyPlanWithCallback(ctx context.Context, plan *ir.Plan, state *ir.State, callback ApplyCallback) (*ir.State, error) {
	var mu sync.Mutex
	var errs []error

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
	var createUpdates, deletes []*ir.ResourceChange
	for _, change := range plan.Changes {
		if change.Action == "DELETE" {
			deletes = append(deletes, change)
		} else {
			createUpdates = append(createUpdates, change)
		}
	}

	// Build dependency graph for parallel execution of creates/updates
	if len(createUpdates) > 1 {
		if err := e.applyParallel(ctx, createUpdates, state, &stateIndex, &mu, emit); err != nil {
			if !e.ContinueOnError {
				return state, err
			}
			errs = append(errs, err)
		}
	} else {
		// Single change or empty - apply sequentially
		for _, change := range createUpdates {
			if err := ctx.Err(); err != nil {
				return state, fmt.Errorf("apply cancelled: %w", err)
			}
			start := time.Now()
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "started"})
			if err := e.applyChange(ctx, change, state, &stateIndex, &mu); err != nil {
				emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "failed", Duration: time.Since(start), Error: err})
				if !e.ContinueOnError {
					return state, err
				}
				errs = append(errs, err)
				continue
			}
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "completed", Duration: time.Since(start)})
		}
	}

	// Apply deletes (reverse dependency order, parallel where possible)
	if len(deletes) > 1 {
		if err := e.applyParallel(ctx, deletes, state, &stateIndex, &mu, emit); err != nil {
			if !e.ContinueOnError {
				return state, err
			}
			errs = append(errs, err)
		}
	} else {
		for _, change := range deletes {
			if err := ctx.Err(); err != nil {
				return state, fmt.Errorf("apply cancelled: %w", err)
			}
			start := time.Now()
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "started"})
			if err := e.applyChange(ctx, change, state, &stateIndex, &mu); err != nil {
				emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "failed", Duration: time.Since(start), Error: err})
				if !e.ContinueOnError {
					return state, err
				}
				errs = append(errs, err)
				continue
			}
			emit(ApplyEvent{Address: change.Address, Action: change.Action, Status: "completed", Duration: time.Since(start)})
		}
	}

	state.Serial++
	state.Outputs = plan.Outputs

	if len(errs) > 0 {
		return state, fmt.Errorf("%d resource(s) failed: %w", len(errs), errors.Join(errs...))
	}

	return state, nil
}

// applyParallel applies changes concurrently, respecting the dependency order
// embedded in the plan (which is already topologically sorted).
func (e *Engine) applyParallel(ctx context.Context, changes []*ir.ResourceChange, state *ir.State, stateIndex *map[string]int, mu *sync.Mutex, emit func(ApplyEvent)) error {
	// Build a mini dependency graph from the changes
	// The plan is already in dependency order, so we can use the position
	// to determine which resources can run in parallel.
	changeMap := make(map[string]*ir.ResourceChange)
	for _, c := range changes {
		changeMap[c.Address] = c
	}

	// Track dependencies: for each change, find which other changes it depends on
	deps := make(map[string]map[string]bool)
	for _, c := range changes {
		deps[c.Address] = make(map[string]bool)
		if c.Desired != nil {
			// Check DependsOn
			for _, d := range c.Desired.DependsOn {
				if _, ok := changeMap[d]; ok {
					deps[c.Address][d] = true
				}
			}
			// Check ptr:// references
			refs := extractPtrRefs(c.Desired.Properties)
			for _, ref := range refs {
				depAddr := ptrRefToAddr(ref)
				if _, ok := changeMap[depAddr]; ok {
					deps[c.Address][depAddr] = true
				}
			}
		}
	}

	// Parallel execution using a semaphore and dependency tracking
	completed := make(map[string]bool)
	failed := make(map[string]bool)
	completedMu := sync.Mutex{}
	completedCond := sync.NewCond(&completedMu)
	var firstErr error
	var allErrs []error
	sem := make(chan struct{}, defaultParallelism)

	var wg sync.WaitGroup

	for _, change := range changes {
		wg.Add(1)
		go func(c *ir.ResourceChange) {
			defer wg.Done()

			// Wait for dependencies to complete
			completedMu.Lock()
			for {
				if firstErr != nil && !e.ContinueOnError {
					completedMu.Unlock()
					return
				}
				allDepsReady := true
				depFailed := false
				for dep := range deps[c.Address] {
					if failed[dep] {
						depFailed = true
						break
					}
					if !completed[dep] {
						allDepsReady = false
						break
					}
				}
				// If a dependency failed, skip this resource
				if depFailed {
					failed[c.Address] = true
					completedMu.Unlock()
					completedCond.Broadcast()
					return
				}
				if allDepsReady {
					break
				}
				completedCond.Wait()
			}
			completedMu.Unlock()

			if err := ctx.Err(); err != nil {
				completedMu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("apply cancelled: %w", err)
				}
				completedMu.Unlock()
				completedCond.Broadcast()
				return
			}

			// Acquire semaphore slot
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			emit(ApplyEvent{Address: c.Address, Action: c.Action, Status: "started"})

			if err := e.applyChange(ctx, c, state, stateIndex, mu); err != nil {
				emit(ApplyEvent{Address: c.Address, Action: c.Action, Status: "failed", Duration: time.Since(start), Error: err})
				completedMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				allErrs = append(allErrs, err)
				failed[c.Address] = true
				completedMu.Unlock()
				completedCond.Broadcast()
				return
			}

			emit(ApplyEvent{Address: c.Address, Action: c.Action, Status: "completed", Duration: time.Since(start)})

			completedMu.Lock()
			completed[c.Address] = true
			completedMu.Unlock()
			completedCond.Broadcast()
		}(change)
	}

	wg.Wait()

	if e.ContinueOnError && len(allErrs) > 0 {
		return fmt.Errorf("%d resource(s) failed: %w", len(allErrs), errors.Join(allErrs...))
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func (e *Engine) applyChange(ctx context.Context, change *ir.ResourceChange, state *ir.State, stateIndex *map[string]int, mu *sync.Mutex) error {
	addr := change.Address
	logging.Debug("applying change", "address", addr, "action", change.Action)

	// Apply per-resource timeout if configured
	var timeout time.Duration
	if change.Desired != nil && change.Desired.Timeout != "" {
		if d, err := time.ParseDuration(change.Desired.Timeout); err == nil {
			timeout = d
		}
	}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var desiredJSON []byte
	var priorJSON []byte
	var name, typ string

	if change.Desired != nil {
		name = change.Desired.Name
		typ = change.Desired.Type
		props := normalizeValue(change.Desired.Properties)
		mu.Lock()
		resolvedProps := resolveReferences(props, state)
		mu.Unlock()
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

	retryPolicy := DefaultRetryPolicy()

	switch change.Action {
	case "CREATE", "UPDATE", "REPLACE":
		var resp *pb.ApplyResponse
		err := RetryWithBackoff(ctx, retryPolicy, func() error {
			var applyErr error
			resp, applyErr = prov.Apply(ctx, &pb.ApplyRequest{
				Type:              typ,
				Name:              name,
				DesiredConfigJson: desiredJSON,
				PriorStateJson:    priorJSON,
			})
			return applyErr
		}, IsTransientError)
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

		err := RetryWithBackoff(ctx, retryPolicy, func() error {
			_, deleteErr := prov.Delete(ctx, &pb.DeleteRequest{
				Type:             typ,
				Id:               resourceID,
				CurrentStateJson: priorJSON,
			})
			return deleteErr
		}, IsTransientError)
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

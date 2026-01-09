package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
)

// Manager handles reading and writing of state.
type Manager struct {
	path      string
	evaluator *eval.Evaluator
}

func NewManager(path string, evaluator *eval.Evaluator) *Manager {
	return &Manager{
		path:      path,
		evaluator: evaluator,
	}
}

// Read loads the state from the configured path.
func (m *Manager) Read(ctx context.Context) (*ir.State, error) {
	// If state file doesn't exist, return empty state
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return &ir.State{
			Version: 1,
			Serial:  0,
		}, nil
	}

	state, err := m.evaluator.LoadState(ctx, m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to load state from %s: %w", m.path, err)
	}

	return state, nil
}

// Write saves the state to the configured path.
// For MVP, we write a simplified PKL representation.
// Detailed PKL generation will be improved later.
func (m *Manager) Write(ctx context.Context, state *ir.State) error {
	// TODO: Use a proper PKL generator. For now, we'll write a basic template.
	// This is a placeholder to verify the flow.
	// Real implementation needs to serialize the `ir.State` struct back to PKL syntax.

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	f, err := os.Create(m.path)
	if err != nil {
		return fmt.Errorf("failed to create state file %s: %w", m.path, err)
	}
	defer f.Close()

	// Write header
	fmt.Fprintf(f, "// Picklr state file\n")
	fmt.Fprintf(f, "amends \"../../pkg/schemas/core/State.pkl\"\n\n")
	fmt.Fprintf(f, "version = %d\n", state.Version)
	fmt.Fprintf(f, "serial = %d\n", state.Serial+1)
	fmt.Fprintf(f, "lineage = %q\n\n", state.Lineage)

	// Write outputs
	if len(state.Outputs) > 0 {
		fmt.Fprintf(f, "outputs {\n")
		for k, v := range state.Outputs {
			switch val := v.(type) {
			case string:
				fmt.Fprintf(f, "  [%q] = %q\n", k, val)
			case int, int64, float64:
				fmt.Fprintf(f, "  [%q] = %v\n", k, val)
			case bool:
				fmt.Fprintf(f, "  [%q] = %t\n", k, val)
			default:
				// Fallback for complex types (maps, lists) - not ideal but better than nothing
				// For real implementation we need full recursion.
				fmt.Fprintf(f, "  [%q] = \"<complex type>\"\n", k)
			}
		}
		fmt.Fprintf(f, "}\n\n")
	} else {
		fmt.Fprintf(f, "outputs = new {}\n\n")
	}

	// Write resources
	fmt.Fprintf(f, "resources {\n")
	for _, res := range state.Resources {
		fmt.Fprintf(f, "  new {\n")
		fmt.Fprintf(f, "    type = %q\n", res.Type)
		fmt.Fprintf(f, "    name = %q\n", res.Name)
		fmt.Fprintf(f, "    provider = %q\n", res.Provider)
		// TODO: Serialize inputs/outputs map to PKL
		fmt.Fprintf(f, "    inputs = new {}\n")
		fmt.Fprintf(f, "    inputsHash = %q\n", res.InputsHash)
		fmt.Fprintf(f, "    outputs = new {}\n")
		fmt.Fprintf(f, "  }\n")
	}
	fmt.Fprintf(f, "}\n")

	return nil
}

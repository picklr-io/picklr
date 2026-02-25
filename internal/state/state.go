package state

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
func (m *Manager) Write(ctx context.Context, state *ir.State) error {
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
	fmt.Fprintf(f, "amends \"../../pkg/schemas/State.pkl\"\n\n")
	fmt.Fprintf(f, "version = %d\n", state.Version)
	fmt.Fprintf(f, "serial = %d\n", state.Serial+1)
	fmt.Fprintf(f, "lineage = %q\n\n", state.Lineage)

	// Write outputs
	if len(state.Outputs) > 0 {
		fmt.Fprintf(f, "outputs {\n")
		for k, v := range state.Outputs {
			fmt.Fprintf(f, "  [%q] = %s\n", k, serializePklValue(v, 1))
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

		// Serialize inputs
		if len(res.Inputs) > 0 {
			fmt.Fprintf(f, "    inputs {\n")
			for k, v := range res.Inputs {
				fmt.Fprintf(f, "      [%q] = %s\n", k, serializePklValue(v, 3))
			}
			fmt.Fprintf(f, "    }\n")
		} else {
			fmt.Fprintf(f, "    inputs = new {}\n")
		}

		fmt.Fprintf(f, "    inputsHash = %q\n", res.InputsHash)

		// Serialize outputs
		if len(res.Outputs) > 0 {
			fmt.Fprintf(f, "    outputs {\n")
			for k, v := range res.Outputs {
				fmt.Fprintf(f, "      [%q] = %s\n", k, serializePklValue(v, 3))
			}
			fmt.Fprintf(f, "    }\n")
		} else {
			fmt.Fprintf(f, "    outputs = new {}\n")
		}

		fmt.Fprintf(f, "  }\n")
	}
	fmt.Fprintf(f, "}\n")

	return nil
}

// serializePklValue recursively serializes a Go value to PKL syntax.
func serializePklValue(v any, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)

	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case bool:
		return fmt.Sprintf("%t", val)
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%g", val)
	case nil:
		return "null"
	case map[string]any:
		if len(val) == 0 {
			return "new {}"
		}
		var b strings.Builder
		b.WriteString("new {\n")
		for k, v := range val {
			b.WriteString(fmt.Sprintf("%s  [%q] = %s\n", indent, k, serializePklValue(v, indentLevel+1)))
		}
		b.WriteString(indent + "}")
		return b.String()
	case map[any]any:
		if len(val) == 0 {
			return "new {}"
		}
		var b strings.Builder
		b.WriteString("new {\n")
		for k, v := range val {
			b.WriteString(fmt.Sprintf("%s  [%q] = %s\n", indent, fmt.Sprintf("%v", k), serializePklValue(v, indentLevel+1)))
		}
		b.WriteString(indent + "}")
		return b.String()
	case []any:
		if len(val) == 0 {
			return "new Listing {}"
		}
		var b strings.Builder
		b.WriteString("new Listing {\n")
		for _, v := range val {
			b.WriteString(fmt.Sprintf("%s  %s\n", indent, serializePklValue(v, indentLevel+1)))
		}
		b.WriteString(indent + "}")
		return b.String()
	default:
		return fmt.Sprintf("%q", fmt.Sprintf("%v", val))
	}
}

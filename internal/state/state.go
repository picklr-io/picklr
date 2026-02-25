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
// If the state file is encrypted, it is transparently decrypted before loading.
func (m *Manager) Read(ctx context.Context) (*ir.State, error) {
	// If state file doesn't exist, return empty state
	if _, err := os.Stat(m.path); os.IsNotExist(err) {
		return &ir.State{
			Version: 1,
			Serial:  0,
		}, nil
	}

	// Check if file is encrypted and decrypt if needed
	raw, err := os.ReadFile(m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file %s: %w", m.path, err)
	}

	if IsEncrypted(raw) {
		decrypted, err := DecryptState(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt state: %w", err)
		}
		// Write decrypted content to a temp file for the PKL evaluator
		tmpFile := m.path + ".dec"
		if err := os.WriteFile(tmpFile, decrypted, 0600); err != nil {
			return nil, fmt.Errorf("failed to write decrypted state: %w", err)
		}
		defer os.Remove(tmpFile)

		state, err := m.evaluator.LoadState(ctx, tmpFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load decrypted state: %w", err)
		}
		return state, nil
	}

	state, err := m.evaluator.LoadState(ctx, m.path)
	if err != nil {
		return nil, fmt.Errorf("failed to load state from %s: %w", m.path, err)
	}

	return state, nil
}

// Write saves the state to the configured path.
// If PICKLR_STATE_ENCRYPTION_KEY is set, the file is transparently encrypted.
func (m *Manager) Write(ctx context.Context, state *ir.State) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(m.path), 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	content := []byte(SerializeState(state))

	// Encrypt if encryption key is configured
	encrypted, err := EncryptState(content)
	if err != nil {
		return fmt.Errorf("failed to encrypt state: %w", err)
	}

	if err := os.WriteFile(m.path, encrypted, 0644); err != nil {
		return fmt.Errorf("failed to write state file %s: %w", m.path, err)
	}

	return nil
}

// SerializeState converts a State to its PKL text representation.
func SerializeState(state *ir.State) string {
	var b strings.Builder

	// Write header
	fmt.Fprintf(&b, "// Picklr state file\n")
	fmt.Fprintf(&b, "amends \"../../pkg/schemas/State.pkl\"\n\n")
	fmt.Fprintf(&b, "version = %d\n", state.Version)
	fmt.Fprintf(&b, "serial = %d\n", state.Serial+1)
	fmt.Fprintf(&b, "lineage = %q\n\n", state.Lineage)

	// Write outputs
	if len(state.Outputs) > 0 {
		fmt.Fprintf(&b, "outputs {\n")
		for k, v := range state.Outputs {
			fmt.Fprintf(&b, "  [%q] = %s\n", k, serializePklValue(v, 1))
		}
		fmt.Fprintf(&b, "}\n\n")
	} else {
		fmt.Fprintf(&b, "outputs = new {}\n\n")
	}

	// Write resources
	fmt.Fprintf(&b, "resources {\n")
	for _, res := range state.Resources {
		fmt.Fprintf(&b, "  new {\n")
		fmt.Fprintf(&b, "    type = %q\n", res.Type)
		fmt.Fprintf(&b, "    name = %q\n", res.Name)
		fmt.Fprintf(&b, "    provider = %q\n", res.Provider)

		// Serialize inputs
		if len(res.Inputs) > 0 {
			fmt.Fprintf(&b, "    inputs {\n")
			for k, v := range res.Inputs {
				fmt.Fprintf(&b, "      [%q] = %s\n", k, serializePklValue(v, 3))
			}
			fmt.Fprintf(&b, "    }\n")
		} else {
			fmt.Fprintf(&b, "    inputs = new {}\n")
		}

		fmt.Fprintf(&b, "    inputsHash = %q\n", res.InputsHash)

		// Serialize outputs
		if len(res.Outputs) > 0 {
			fmt.Fprintf(&b, "    outputs {\n")
			for k, v := range res.Outputs {
				fmt.Fprintf(&b, "      [%q] = %s\n", k, serializePklValue(v, 3))
			}
			fmt.Fprintf(&b, "    }\n")
		} else {
			fmt.Fprintf(&b, "    outputs = new {}\n")
		}

		fmt.Fprintf(&b, "  }\n")
	}
	fmt.Fprintf(&b, "}\n")

	return b.String()
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

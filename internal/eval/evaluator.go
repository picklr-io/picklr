package eval

import (
	"context"
	"fmt"
	"net/url"

	"github.com/apple/pkl-go/pkl"
	"github.com/picklr-io/picklr/internal/ir"
)

// Evaluator handles PKL evaluation into IR types.
type Evaluator struct {
	projectDir string
}

func NewEvaluator(projectDir string) *Evaluator {
	return &Evaluator{
		projectDir: projectDir,
	}
}

// LoadConfig evaluates the main configuration file and returns the IR.
func (e *Evaluator) LoadConfig(ctx context.Context, entryPoint string, properties map[string]string) (*ir.Config, error) {
	u, err := url.Parse("file://" + e.projectDir + "/")
	if err != nil {
		return nil, fmt.Errorf("failed to parse project directory URL: %w", err)
	}

	opts := []func(*pkl.EvaluatorOptions){pkl.PreconfiguredOptions}
	if len(properties) > 0 {
		opts = append(opts, func(o *pkl.EvaluatorOptions) {
			if o.Properties == nil {
				o.Properties = make(map[string]string)
			}
			for k, v := range properties {
				o.Properties[k] = v
			}
		})
	}

	evaluator, err := pkl.NewProjectEvaluator(ctx, u, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create PKL evaluator: %w", err)
	}
	defer evaluator.Close()

	var cfg ir.Config
	if err := evaluator.EvaluateModule(ctx, pkl.FileSource(entryPoint), &cfg); err != nil {
		return nil, fmt.Errorf("failed to evaluate config: %w", err)
	}

	return &cfg, nil
}

// LoadState evaluates a state file and returns the IR.
func (e *Evaluator) LoadState(ctx context.Context, stateFile string) (*ir.State, error) {
	evaluator, err := pkl.NewEvaluator(ctx, pkl.PreconfiguredOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to create PKL evaluator: %w", err)
	}
	defer evaluator.Close()

	var state ir.State
	if err := evaluator.EvaluateModule(ctx, pkl.FileSource(stateFile), &state); err != nil {
		return nil, fmt.Errorf("failed to evaluate state: %w", err)
	}

	return &state, nil
}

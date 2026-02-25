package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/engine"
	"github.com/picklr-io/picklr/internal/eval"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Output the dependency graph in DOT format",
	Long: `Generates a visual representation of the resource dependency graph
in Graphviz DOT format. Pipe the output to 'dot' to generate an image:

  picklr graph | dot -Tpng > graph.png`,
	RunE: runGraph,
}

func runGraph(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	entryPoint := "main.pkl"

	if len(args) > 0 {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("failed to resolve path %s: %w", args[0], err)
		}
		info, err := os.Stat(absPath)
		if err != nil {
			return fmt.Errorf("failed to stat path %s: %w", args[0], err)
		}
		if info.IsDir() {
			wd = absPath
		} else {
			wd = filepath.Dir(absPath)
			entryPoint = filepath.Base(absPath)
		}
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)

	cfg, err := evaluator.LoadConfig(ctx, entryPoint, nil)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	dag, err := engine.BuildDAG(cfg.Resources)
	if err != nil {
		return fmt.Errorf("failed to build graph: %w", err)
	}

	// Output DOT format
	fmt.Println("digraph picklr {")
	fmt.Println("  rankdir = \"BT\";")
	fmt.Println("  node [shape = rect];")
	fmt.Println()

	for _, res := range cfg.Resources {
		addr := engine.ResourceAddrPublic(res)
		fmt.Printf("  %q;\n", addr)
	}
	fmt.Println()

	// Output edges from the DAG
	for _, res := range cfg.Resources {
		addr := engine.ResourceAddrPublic(res)
		deps := dag.Dependencies(addr)
		for _, dep := range deps {
			fmt.Printf("  %q -> %q;\n", addr, dep)
		}
	}

	fmt.Println("}")
	return nil
}

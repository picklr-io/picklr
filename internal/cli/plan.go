package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/engine"
	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var (
	planOutFile string
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Generate an execution plan",
	Long: `Generates an execution plan showing what actions Picklr will take
to reach the desired state defined in your configuration.

The plan shows:
  • Resources to be created
  • Resources to be updated (with diff)
  • Resources to be deleted`,
	RunE: runPlan,
}

func init() {
	planCmd.Flags().StringVarP(&planOutFile, "out", "o", "", "Write plan to file")
}

func runPlan(cmd *cobra.Command, args []string) error {
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
	fmt.Printf("DEBUG: wd=%s, entryPoint=%s\n", wd, entryPoint)
	ctx := cmd.Context()

	// 1. Initialize Components
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)

	registry := provider.NewRegistry()
	if err := registry.LoadProvider("null"); err != nil {
		return fmt.Errorf("failed to load null provider: %w", err)
	}

	eng := engine.NewEngine(registry)

	// 2. Load Config
	fmt.Print("Loading configuration... ")
	cfg, err := evaluator.LoadConfig(ctx, entryPoint, nil)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Println("OK")

	// 3. Load State
	state, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// 4. Create Plan
	fmt.Print("Calculating plan... ")
	plan, err := eng.CreatePlan(ctx, cfg, state)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("plan generation failed: %w", err)
	}
	fmt.Println("OK")

	// 5. Output Summary
	fmt.Println("\nPlan Summary:")
	fmt.Printf("  Create:  %d\n", plan.Summary.Create)
	fmt.Printf("  Update:  %d\n", plan.Summary.Update)
	fmt.Printf("  Delete:  %d\n", plan.Summary.Delete)
	fmt.Printf("  Replace: %d\n", plan.Summary.Replace)
	fmt.Printf("  NoOp:    %d\n", plan.Summary.NoOp)

	if len(plan.Changes) > 0 {
		fmt.Println("\nTerraform will perform the following actions:")

		for _, change := range plan.Changes {
			symbol := "~"
			switch change.Action {
			case "CREATE":
				symbol = "+"
			case "DELETE":
				symbol = "-"
			case "REPLACE":
				symbol = "-/+"
			case "NOOP":
				symbol = " "
			}

			// Colorize output based on action
			color := "\033[0m" // Reset
			if change.Action == "CREATE" {
				color = "\033[32m" // Green
			} else if change.Action == "DELETE" {
				color = "\033[31m" // Red
			} else if change.Action == "UPDATE" || change.Action == "REPLACE" {
				color = "\033[33m" // Yellow
			}

			var resourceType, resourceName string
			if change.Desired != nil {
				resourceType = change.Desired.Type
				resourceName = change.Desired.Name
			} else if change.Prior != nil {
				resourceType = change.Prior.Type
				resourceName = change.Prior.Name
			}

			fmt.Printf("\n%s  # %s will be %s%s\n", color, change.Address, change.Action, "\033[0m")
			fmt.Printf("%s  %s resource \"%s\" \"%s\" {\n", color, symbol, resourceType, resourceName)

			// TODO: Render detailed diff of properties
			// For now, identifying the resource is a massive step up.
			fmt.Printf("%s      %s\n", color, "...")
			fmt.Printf("%s    }%s\n", color, "\033[0m")
		}
	} else {
		fmt.Println("\nNo changes. Infrastructure is up-to-date.")
	}

	// TODO: Save plan to file if requested

	return nil
}

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
	applyAutoApprove bool
	applyProperties  map[string]string
)

var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply a configuration",
	Long:  `Build or changes infrastructure according to Picklr configuration files.`,
	RunE:  runApply,
}

func init() {
	applyCmd.Flags().BoolVar(&applyAutoApprove, "auto-approve", false, "Skip interactive approval of plan before applying")
	applyCmd.Flags().StringToStringVarP(&applyProperties, "prop", "D", nil, "Set external properties (format: key=value)")
}

func runApply(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	ctx := cmd.Context()

	// 1. Initialize Components
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)

	registry := provider.NewRegistry()
	if err := registry.LoadProvider("null"); err != nil {
		return fmt.Errorf("failed to load null provider: %w", err)
	}

	eng := engine.NewEngine(registry)

	// 2. Load Config & State
	fmt.Print("Loading configuration... ")
	cfg, err := evaluator.LoadConfig(ctx, "main.pkl", applyProperties)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Println("OK")

	state, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// 3. Create Plan
	fmt.Print("Calculating plan... ")
	plan, err := eng.CreatePlan(ctx, cfg, state)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("plan generation failed: %w", err)
	}
	fmt.Println("OK")

	if len(plan.Changes) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
	}

	// 4. Output Summary & Confirm
	fmt.Println("\nPlan Summary:")
	fmt.Printf("  Create:  %d\n", plan.Summary.Create)
	fmt.Printf("  Update:  %d\n", plan.Summary.Update)
	fmt.Printf("  Delete:  %d\n", plan.Summary.Delete)
	fmt.Printf("  Replace: %d\n", plan.Summary.Replace)
	fmt.Printf("  NoOp:    %d\n", plan.Summary.NoOp)

	if !applyAutoApprove {
		fmt.Print("\nDo you want to perform these actions? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "yes" {
			fmt.Println("Apply cancelled.")
			return nil
		}
	}

	// 5. Apply Plan
	fmt.Printf("\nApplying %d changes...\n", len(plan.Changes))
	newState, err := eng.ApplyPlan(ctx, plan, state)
	if err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}

	// 6. Save State
	if err := stateMgr.Write(ctx, newState); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	fmt.Println("\nApply complete!")

	if len(newState.Outputs) > 0 {
		fmt.Println("\nOutputs:")
		for k, v := range newState.Outputs {
			fmt.Printf("  %s = %v\n", k, v)
		}
	}

	return nil
}

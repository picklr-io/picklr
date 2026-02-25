package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/engine"
	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var (
	destroyAutoApprove bool
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy all managed infrastructure",
	Long: `Destroys all resources managed by Picklr.

This command is the inverse of 'picklr apply'. It will delete all resources
tracked in the state file.`,
	RunE: runDestroy,
}

func init() {
	destroyCmd.Flags().BoolVar(&destroyAutoApprove, "auto-approve", false, "Skip interactive approval before destroying")
}

func runDestroy(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

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
		}
	}

	ctx := cmd.Context()

	// 1. Initialize components
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)
	registry := provider.NewRegistry()
	eng := engine.NewEngine(registry)

	// 2. Read state
	fmt.Print("Reading state... ")
	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to read state: %w", err)
	}
	fmt.Println("OK")

	if len(currentState.Resources) == 0 {
		fmt.Println("No resources to destroy. State is empty.")
		return nil
	}

	// 3. Load providers for all resources in state
	if err := loadStateProviders(registry, currentState); err != nil {
		return err
	}

	// 4. Generate destroy plan (empty config = delete everything)
	emptyCfg := &ir.Config{
		Resources: []*ir.Resource{},
		Outputs:   map[string]any{},
	}

	fmt.Print("Calculating destroy plan... ")
	plan, err := eng.CreatePlan(ctx, emptyCfg, currentState)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("destroy plan failed: %w", err)
	}
	fmt.Println("OK")

	if len(plan.Changes) == 0 {
		fmt.Println("No resources to destroy.")
		return nil
	}

	// 5. Show what will be destroyed
	fmt.Printf("\nPicklr will destroy the following %d resource(s):\n", len(plan.Changes))
	renderPlanChanges(plan)
	renderPlanSummary(plan)

	// 6. Confirm
	if !destroyAutoApprove {
		fmt.Print("\nDo you really want to destroy all resources? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "yes" {
			fmt.Println("Destroy cancelled.")
			return nil
		}
	}

	// 7. Execute
	fmt.Printf("\nDestroying %d resources...\n", len(plan.Changes))
	newState, err := eng.ApplyPlan(ctx, plan, currentState)
	if err != nil {
		// Write partial state on failure
		_ = stateMgr.Write(ctx, currentState)
		return fmt.Errorf("destroy failed: %w", err)
	}

	// 8. Write empty state
	if err := stateMgr.Write(ctx, newState); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	fmt.Printf("\nDestroy complete! %d resources destroyed.\n", plan.Summary.Delete)
	return nil
}

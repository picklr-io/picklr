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

	// 1. Initialize Components
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)
	registry := provider.NewRegistry()
	eng := engine.NewEngine(registry)

	// 2. Lock state
	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	// 3. Load Config & State
	fmt.Print("Loading configuration... ")
	cfg, err := evaluator.LoadConfig(ctx, entryPoint, applyProperties)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Println("OK")

	// Auto-load providers required by the config
	if err := loadRequiredProviders(registry, cfg); err != nil {
		return err
	}

	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Also load providers for resources already in state (needed for DELETE)
	if err := loadStateProviders(registry, currentState); err != nil {
		return err
	}

	// 3. Create Plan
	fmt.Print("Calculating plan... ")
	plan, err := eng.CreatePlan(ctx, cfg, currentState)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("plan generation failed: %w", err)
	}
	fmt.Println("OK")

	if len(plan.Changes) == 0 {
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return nil
	}

	fmt.Println("\nPicklr will perform the following actions:")
	renderPlanChanges(plan)

	// 4. Output Summary & Confirm
	renderPlanSummary(plan)

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

	newState, err := eng.ApplyPlan(ctx, plan, currentState)
	if err != nil {
		// Write partial state on failure so successful changes aren't lost
		_ = stateMgr.Write(ctx, currentState)
		return fmt.Errorf("apply failed: %w", err)
	}

	// 6. Persist state
	if err := stateMgr.Write(ctx, newState); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	fmt.Println("\nApply complete! Resources: " +
		fmt.Sprintf("%d added, %d changed, %d destroyed.", plan.Summary.Create, plan.Summary.Update, plan.Summary.Delete))

	if len(newState.Outputs) > 0 {
		fmt.Println("\nOutputs:")
		for k, v := range newState.Outputs {
			fmt.Printf("  %s = %v\n", k, v)
		}
	}

	return nil
}

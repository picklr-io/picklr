package cli

import (
	"encoding/json"
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
	ctx := cmd.Context()

	// 1. Initialize Components
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)
	registry := provider.NewRegistry()
	eng := engine.NewEngine(registry)

	// 2. Load Config
	fmt.Print("Loading configuration... ")
	cfg, err := evaluator.LoadConfig(ctx, entryPoint, nil)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to load config: %w", err)
	}
	fmt.Println("OK")

	// Auto-load providers required by the config
	if err := loadRequiredProviders(registry, cfg); err != nil {
		return err
	}

	// 3. Load State
	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Also load providers for resources in state (needed for DELETE planning)
	if err := loadStateProviders(registry, currentState); err != nil {
		return err
	}

	// 4. Create Plan
	fmt.Print("Calculating plan... ")
	plan, err := eng.CreatePlan(ctx, cfg, currentState)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("plan generation failed: %w", err)
	}
	fmt.Println("OK")

	// 5. Output
	if len(plan.Changes) > 0 {
		fmt.Println("\nPicklr will perform the following actions:")
		renderPlanChanges(plan)
	} else {
		fmt.Println("\nNo changes. Infrastructure is up-to-date.")
	}

	renderPlanSummary(plan)

	// Save plan to file if requested
	if planOutFile != "" {
		planJSON, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal plan: %w", err)
		}
		if err := os.WriteFile(planOutFile, planJSON, 0644); err != nil {
			return fmt.Errorf("failed to write plan to %s: %w", planOutFile, err)
		}
		fmt.Printf("\nPlan saved to %s\n", planOutFile)
	}

	return nil
}

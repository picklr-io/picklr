package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/picklr-io/picklr/internal/engine"
	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var (
	applyAutoApprove bool
	applyProperties  map[string]string
	applyJSON        bool
	applyRefresh     bool
)

var applyCmd = &cobra.Command{
	Use:   "apply [path|plan-file]",
	Short: "Apply a configuration",
	Long: `Build or change infrastructure according to Picklr configuration files.

If a saved plan file (JSON) is provided, it will be applied directly
without recalculating the plan.`,
	RunE: runApply,
}

func init() {
	applyCmd.Flags().BoolVar(&applyAutoApprove, "auto-approve", false, "Skip interactive approval of plan before applying")
	applyCmd.Flags().StringToStringVarP(&applyProperties, "prop", "D", nil, "Set external properties (format: key=value)")
	applyCmd.Flags().BoolVar(&applyJSON, "json", false, "Output in JSON format")
	applyCmd.Flags().BoolVar(&applyRefresh, "refresh", false, "Refresh state before applying")
}

func runApply(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	entryPoint := "main.pkl"

	// Check if argument is a saved plan file
	var savedPlan *ir.Plan
	if len(args) > 0 {
		if data, err := os.ReadFile(args[0]); err == nil {
			var plan ir.Plan
			if json.Unmarshal(data, &plan) == nil && plan.Summary != nil {
				savedPlan = &plan
			}
		}
	}

	if savedPlan == nil && len(args) > 0 {
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
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)
	registry := provider.NewRegistry()
	eng := engine.NewEngine(registry)

	// 2. Lock state
	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	var plan *ir.Plan

	if savedPlan != nil {
		// Apply saved plan
		if !applyJSON {
			fmt.Println("Using saved plan file...")
		}
		plan = savedPlan

		// Load providers referenced by the plan changes
		providersSeen := make(map[string]bool)
		for _, change := range plan.Changes {
			provName := ""
			if change.Desired != nil {
				provName = change.Desired.Provider
			} else if change.Prior != nil {
				provName = change.Prior.Provider
			}
			if provName != "" && !providersSeen[provName] {
				providersSeen[provName] = true
				if err := registry.LoadProvider(provName); err != nil {
					return fmt.Errorf("failed to load provider %s: %w", provName, err)
				}
			}
		}
	} else {
		// 3. Load Config & generate plan
		if !applyJSON {
			fmt.Print("Loading configuration... ")
		}
		cfg, err := evaluator.LoadConfig(ctx, entryPoint, applyProperties)
		if err != nil {
			if !applyJSON {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("failed to load config: %w", err)
		}
		if !applyJSON {
			fmt.Println("OK")
		}

		// Auto-load providers
		if err := loadRequiredProviders(registry, cfg); err != nil {
			return err
		}
		if err := loadStateProviders(registry, currentState); err != nil {
			return err
		}

		// Auto-refresh if requested
		if applyRefresh && len(currentState.Resources) > 0 {
			if !applyJSON {
				fmt.Print("Refreshing state... ")
			}
			drifted := refreshStateInPlace(ctx, currentState, registry)
			if !applyJSON {
				fmt.Println("OK")
				renderDriftChanges(drifted)
			}
		}

		if !applyJSON {
			fmt.Print("Calculating plan... ")
		}
		plan, err = eng.CreatePlanWithTargets(ctx, cfg, currentState, targets)
		if err != nil {
			if !applyJSON {
				fmt.Println("FAILED")
			}
			return fmt.Errorf("plan generation failed: %w", err)
		}
		if !applyJSON {
			fmt.Println("OK")
		}
	}

	if len(plan.Changes) == 0 {
		if applyJSON {
			return renderApplyResultJSON(plan, currentState, cliOutput())
		}
		fmt.Println("No changes. Infrastructure is up-to-date.")
		return nil
	}

	if !applyJSON {
		fmt.Println("\nPicklr will perform the following actions:")
		renderPlanChanges(plan)
		renderPlanSummary(plan)
	}

	if !applyAutoApprove && !applyJSON {
		fmt.Print("\nDo you want to perform these actions? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "yes" {
			fmt.Println("Apply cancelled.")
			return nil
		}
	}

	// 5. Apply Plan with progress events
	if !applyJSON {
		fmt.Printf("\nApplying %d changes...\n", len(plan.Changes))
	}

	callback := func(event engine.ApplyEvent) {
		if applyJSON {
			return // Suppress progress in JSON mode
		}
		switch event.Status {
		case "started":
			actionVerb := "Creating"
			color := colorize("\033[32m")
			switch event.Action {
			case "UPDATE":
				actionVerb = "Modifying"
				color = colorize("\033[33m")
			case "REPLACE":
				actionVerb = "Replacing"
				color = colorize("\033[33m")
			case "DELETE":
				actionVerb = "Destroying"
				color = colorize("\033[31m")
			}
			fmt.Printf("%s%s: %s...%s\n", color, event.Address, actionVerb, colorize("\033[0m"))
		case "completed":
			actionVerb := "Creation complete"
			color := colorize("\033[32m")
			switch event.Action {
			case "UPDATE":
				actionVerb = "Modification complete"
				color = colorize("\033[33m")
			case "REPLACE":
				actionVerb = "Replacement complete"
				color = colorize("\033[33m")
			case "DELETE":
				actionVerb = "Destruction complete"
				color = colorize("\033[31m")
			}
			fmt.Printf("%s%s: %s after %s%s\n", color, event.Address, actionVerb, event.Duration.Round(time.Millisecond), colorize("\033[0m"))
		case "failed":
			fmt.Printf("%s%s: FAILED (%v)%s\n", colorize("\033[31m"), event.Address, event.Error, colorize("\033[0m"))
		}
	}

	newState, err := eng.ApplyPlanWithCallback(ctx, plan, currentState, callback)
	if err != nil {
		// Write partial state on failure so successful changes aren't lost
		_ = stateMgr.Write(ctx, currentState)
		return fmt.Errorf("apply failed: %w", err)
	}

	// 6. Persist state
	if err := stateMgr.Write(ctx, newState); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	// 7. Audit log
	auditChanges := make([]AuditChange, 0, len(plan.Changes))
	for _, c := range plan.Changes {
		auditChanges = append(auditChanges, AuditChange{Address: c.Address, Action: c.Action})
	}
	_ = writeAuditLog(AuditEntry{
		Operation: "apply",
		Changes:   auditChanges,
		Summary: map[string]int{
			"create":  plan.Summary.Create,
			"update":  plan.Summary.Update,
			"delete":  plan.Summary.Delete,
			"replace": plan.Summary.Replace,
		},
	})

	if applyJSON {
		return renderApplyResultJSON(plan, newState, cliOutput())
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

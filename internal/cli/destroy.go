package cli

import (
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
	destroyAutoApprove bool
	destroyJSON        bool
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
	destroyCmd.Flags().BoolVar(&destroyJSON, "json", false, "Output in JSON format")
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
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)
	registry := provider.NewRegistry()
	eng := engine.NewEngine(registry)

	// 2. Lock state
	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	// 3. Read state
	if !destroyJSON {
		fmt.Print("Reading state... ")
	}
	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		if !destroyJSON {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("failed to read state: %w", err)
	}
	if !destroyJSON {
		fmt.Println("OK")
	}

	if len(currentState.Resources) == 0 {
		if !destroyJSON {
			fmt.Println("No resources to destroy. State is empty.")
		}
		return nil
	}

	// Load providers for all resources in state
	if err := loadStateProviders(registry, currentState); err != nil {
		return err
	}

	// 4. Generate destroy plan (empty config = delete everything)
	emptyCfg := &ir.Config{
		Resources: []*ir.Resource{},
		Outputs:   map[string]any{},
	}

	if !destroyJSON {
		fmt.Print("Calculating destroy plan... ")
	}
	plan, err := eng.CreatePlanWithTargets(ctx, emptyCfg, currentState, targets)
	if err != nil {
		if !destroyJSON {
			fmt.Println("FAILED")
		}
		return fmt.Errorf("destroy plan failed: %w", err)
	}
	if !destroyJSON {
		fmt.Println("OK")
	}

	if len(plan.Changes) == 0 {
		if !destroyJSON {
			fmt.Println("No resources to destroy.")
		}
		return nil
	}

	// 5. Show what will be destroyed
	if !destroyJSON {
		fmt.Printf("\nPicklr will destroy the following %d resource(s):\n", len(plan.Changes))
		renderPlanChanges(plan)
		renderPlanSummary(plan)
	}

	// 6. Confirm
	if !destroyAutoApprove && !destroyJSON {
		fmt.Print("\nDo you really want to destroy all resources? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "yes" {
			fmt.Println("Destroy cancelled.")
			return nil
		}
	}

	// 7. Execute
	if !destroyJSON {
		fmt.Printf("\nDestroying %d resources...\n", len(plan.Changes))
	}

	callback := func(event engine.ApplyEvent) {
		if destroyJSON {
			return
		}
		switch event.Status {
		case "started":
			fmt.Printf("%s%s: Destroying...%s\n", colorize("\033[31m"), event.Address, colorize("\033[0m"))
		case "completed":
			fmt.Printf("%s%s: Destruction complete after %s%s\n", colorize("\033[31m"), event.Address, event.Duration.Round(time.Millisecond), colorize("\033[0m"))
		case "failed":
			fmt.Printf("%s%s: FAILED (%v)%s\n", colorize("\033[31m"), event.Address, event.Error, colorize("\033[0m"))
		}
	}

	newState, err := eng.ApplyPlanWithCallback(ctx, plan, currentState, callback)
	if err != nil {
		_ = stateMgr.Write(ctx, currentState)
		return fmt.Errorf("destroy failed: %w", err)
	}

	// 8. Write state
	if err := stateMgr.Write(ctx, newState); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	if destroyJSON {
		return renderApplyResultJSON(plan, newState, cliOutput())
	}

	fmt.Printf("\nDestroy complete! %d resources destroyed.\n", plan.Summary.Delete)
	return nil
}

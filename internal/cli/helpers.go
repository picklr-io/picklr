package cli

import (
	"fmt"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
)

// loadRequiredProviders auto-loads all providers referenced by config resources.
func loadRequiredProviders(registry *provider.Registry, cfg *ir.Config) error {
	seen := make(map[string]bool)
	for _, res := range cfg.Resources {
		if res.Provider != "" && !seen[res.Provider] {
			seen[res.Provider] = true
			if err := registry.LoadProvider(res.Provider); err != nil {
				return fmt.Errorf("failed to load provider %s: %w", res.Provider, err)
			}
		}
	}
	return nil
}

// loadStateProviders auto-loads all providers referenced by state resources (needed for DELETE).
func loadStateProviders(registry *provider.Registry, state *ir.State) error {
	seen := make(map[string]bool)
	for _, res := range state.Resources {
		if res.Provider != "" && !seen[res.Provider] {
			seen[res.Provider] = true
			if err := registry.LoadProvider(res.Provider); err != nil {
				return fmt.Errorf("failed to load provider %s: %w", res.Provider, err)
			}
		}
	}
	return nil
}

// renderPlanChanges prints the detailed change list for a plan.
func renderPlanChanges(plan *ir.Plan) {
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

		color := "\033[0m"
		if change.Action == "CREATE" {
			color = "\033[32m"
		} else if change.Action == "DELETE" {
			color = "\033[31m"
		} else if change.Action == "UPDATE" || change.Action == "REPLACE" {
			color = "\033[33m"
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

		// Render property diffs if available
		if change.Diff != nil && len(change.Diff) > 0 {
			renderPropertyDiff(change, color)
		} else {
			// Fall back to showing desired properties for CREATE, or prior for DELETE
			if change.Action == "CREATE" && change.Desired != nil {
				for k, v := range change.Desired.Properties {
					fmt.Printf("%s      + %s = %v\n", color, k, formatValue(v))
				}
			} else if change.Action == "DELETE" && change.Prior != nil {
				for k, v := range change.Prior.Properties {
					fmt.Printf("%s      - %s = %v\n", color, k, formatValue(v))
				}
			} else if change.Desired != nil && change.Prior != nil {
				renderInlineDiff(change.Prior.Properties, change.Desired.Properties, color)
			} else {
				fmt.Printf("%s      ...\n", color)
			}
		}
		fmt.Printf("%s    }%s\n", color, "\033[0m")
	}
}

// renderPropertyDiff prints structured property diffs.
func renderPropertyDiff(change *ir.ResourceChange, color string) {
	for key, diff := range change.Diff {
		switch diff.Action {
		case "create":
			fmt.Printf("\033[32m      + %s = %v\033[0m\n", key, formatValue(diff.After))
		case "delete":
			fmt.Printf("\033[31m      - %s = %v\033[0m\n", key, formatValue(diff.Before))
		case "update":
			fmt.Printf("\033[33m      ~ %s = %v -> %v\033[0m\n", key, formatValue(diff.Before), formatValue(diff.After))
		default:
			fmt.Printf("%s        %s = %v\n", color, key, formatValue(diff.After))
		}
	}
}

// renderInlineDiff compares prior and desired property maps and prints a diff.
func renderInlineDiff(prior, desired map[string]any, color string) {
	allKeys := make(map[string]bool)
	for k := range prior {
		allKeys[k] = true
	}
	for k := range desired {
		allKeys[k] = true
	}

	for k := range allKeys {
		priorVal, inPrior := prior[k]
		desiredVal, inDesired := desired[k]

		if !inPrior {
			fmt.Printf("\033[32m      + %s = %v\033[0m\n", k, formatValue(desiredVal))
		} else if !inDesired {
			fmt.Printf("\033[31m      - %s = %v\033[0m\n", k, formatValue(priorVal))
		} else if fmt.Sprintf("%v", priorVal) != fmt.Sprintf("%v", desiredVal) {
			fmt.Printf("\033[33m      ~ %s = %v -> %v\033[0m\n", k, formatValue(priorVal), formatValue(desiredVal))
		} else {
			fmt.Printf("        %s = %v\n", k, formatValue(desiredVal))
		}
	}
}

// formatValue returns a human-readable representation of a value.
func formatValue(v any) string {
	if v == nil {
		return "null"
	}
	switch val := v.(type) {
	case string:
		return fmt.Sprintf("%q", val)
	case map[string]any:
		return fmt.Sprintf("%v", val)
	case map[any]any:
		return fmt.Sprintf("%v", val)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// renderPlanSummary prints the plan summary counts.
func renderPlanSummary(plan *ir.Plan) {
	fmt.Println("\nPlan Summary:")
	fmt.Printf("  Create:  %d\n", plan.Summary.Create)
	fmt.Printf("  Update:  %d\n", plan.Summary.Update)
	fmt.Printf("  Delete:  %d\n", plan.Summary.Delete)
	fmt.Printf("  Replace: %d\n", plan.Summary.Replace)
	fmt.Printf("  NoOp:    %d\n", plan.Summary.NoOp)
}

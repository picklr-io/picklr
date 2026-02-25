package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/picklr-io/picklr/internal/ir"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
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

		color := ""
		reset := colorize("\033[0m")
		if change.Action == "CREATE" {
			color = colorize("\033[32m")
		} else if change.Action == "DELETE" {
			color = colorize("\033[31m")
		} else if change.Action == "UPDATE" || change.Action == "REPLACE" {
			color = colorize("\033[33m")
		}

		var resourceType, resourceName string
		if change.Desired != nil {
			resourceType = change.Desired.Type
			resourceName = change.Desired.Name
		} else if change.Prior != nil {
			resourceType = change.Prior.Type
			resourceName = change.Prior.Name
		}

		fmt.Printf("\n%s  # %s will be %s%s\n", color, change.Address, change.Action, reset)
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
		fmt.Printf("%s    }%s\n", color, reset)
	}
}

// renderPropertyDiff prints structured property diffs.
func renderPropertyDiff(change *ir.ResourceChange, color string) {
	for key, diff := range change.Diff {
		val := func(v any) string {
			if diff.Sensitive {
				return "(sensitive)"
			}
			return formatValue(v)
		}
		switch diff.Action {
		case "create":
			fmt.Printf("%s      + %s = %v%s\n", colorize("\033[32m"), key, val(diff.After), colorize("\033[0m"))
		case "delete":
			fmt.Printf("%s      - %s = %v%s\n", colorize("\033[31m"), key, val(diff.Before), colorize("\033[0m"))
		case "update":
			fmt.Printf("%s      ~ %s = %v -> %v%s\n", colorize("\033[33m"), key, val(diff.Before), val(diff.After), colorize("\033[0m"))
		default:
			fmt.Printf("%s        %s = %v\n", color, key, val(diff.After))
		}
	}
}

// colorize returns the ANSI color code, or empty string if --no-color is set.
func colorize(code string) string {
	if noColor {
		return ""
	}
	return code
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
			fmt.Printf("%s      + %s = %v%s\n", colorize("\033[32m"), k, formatValue(desiredVal), colorize("\033[0m"))
		} else if !inDesired {
			fmt.Printf("%s      - %s = %v%s\n", colorize("\033[31m"), k, formatValue(priorVal), colorize("\033[0m"))
		} else if fmt.Sprintf("%v", priorVal) != fmt.Sprintf("%v", desiredVal) {
			fmt.Printf("%s      ~ %s = %v -> %v%s\n", colorize("\033[33m"), k, formatValue(priorVal), formatValue(desiredVal), colorize("\033[0m"))
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

// renderPlanJSON outputs the plan as structured JSON.
func renderPlanJSON(plan *ir.Plan, w io.Writer) error {
	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plan: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

// renderApplyResultJSON outputs the apply result as structured JSON.
func renderApplyResultJSON(plan *ir.Plan, state *ir.State, w io.Writer) error {
	result := map[string]any{
		"summary": map[string]int{
			"create":  plan.Summary.Create,
			"update":  plan.Summary.Update,
			"delete":  plan.Summary.Delete,
			"replace": plan.Summary.Replace,
		},
		"outputs":   state.Outputs,
		"resources": len(state.Resources),
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Fprintln(w, string(data))
	return nil
}

// renderDriftChanges prints drift detected during refresh.
func renderDriftChanges(drifted []DriftChange) {
	if len(drifted) == 0 {
		return
	}
	fmt.Printf("\n%sNote: Objects have changed outside of Picklr%s\n\n", colorize("\033[33m"), colorize("\033[0m"))
	fmt.Println("Picklr detected the following changes made outside of Picklr since the")
	fmt.Println("last \"picklr apply\":")
	fmt.Println()

	for _, d := range drifted {
		fmt.Printf("  %s# %s has been changed%s\n", colorize("\033[33m"), d.Address, colorize("\033[0m"))
		if d.Deleted {
			fmt.Printf("  %s- %s has been deleted%s\n", colorize("\033[31m"), d.Address, colorize("\033[0m"))
		} else {
			fmt.Printf("  %s~ %s has drifted%s\n", colorize("\033[33m"), d.Address, colorize("\033[0m"))
		}
	}
	fmt.Println()
}

// DriftChange represents a detected drift in a resource.
type DriftChange struct {
	Address string
	Deleted bool
}

// sortedKeys returns sorted keys of a string map for deterministic output.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// cliOutput writes to stdout by default, allowing JSON mode switching.
func cliOutput() io.Writer {
	return os.Stdout
}

// refreshStateInPlace reads all resources from their providers and updates state in place.
// Returns a list of drift changes detected.
func refreshStateInPlace(ctx context.Context, state *ir.State, registry *provider.Registry) []DriftChange {
	var drifted []DriftChange

	for _, res := range state.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		prov, err := registry.Get(res.Provider)
		if err != nil {
			continue
		}

		var resourceID string
		if id, ok := res.Outputs["id"]; ok {
			resourceID = fmt.Sprintf("%v", id)
		}

		var currentJSON []byte
		if res.Outputs != nil {
			currentJSON, _ = json.Marshal(res.Outputs)
		}

		resp, err := prov.Read(ctx, &pb.ReadRequest{
			Type:             res.Type,
			Id:               resourceID,
			CurrentStateJson: currentJSON,
		})
		if err != nil {
			continue
		}

		if !resp.Exists {
			drifted = append(drifted, DriftChange{Address: addr, Deleted: true})
			continue
		}

		if len(resp.NewStateJson) > 0 {
			var newOutputs map[string]any
			if err := json.Unmarshal(resp.NewStateJson, &newOutputs); err == nil {
				if fmt.Sprintf("%v", newOutputs) != fmt.Sprintf("%v", res.Outputs) {
					drifted = append(drifted, DriftChange{Address: addr, Deleted: false})
					res.Outputs = newOutputs
				}
			}
		}
	}

	return drifted
}

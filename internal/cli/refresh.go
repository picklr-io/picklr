package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/picklr-io/picklr/internal/state"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
	"github.com/spf13/cobra"
)

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Update state to match real infrastructure",
	Long: `Reads the current state of all managed resources from their providers
and updates the state file to reflect actual infrastructure.

This detects drift between what Picklr thinks exists and what actually exists.`,
	RunE: runRefresh,
}

func runRefresh(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)
	registry := provider.NewRegistry()

	// Lock state
	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	// Read state
	fmt.Print("Reading state... ")
	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("failed to read state: %w", err)
	}
	fmt.Println("OK")

	if len(currentState.Resources) == 0 {
		fmt.Println("No resources to refresh.")
		return nil
	}

	// Load providers
	if err := loadStateProviders(registry, currentState); err != nil {
		return err
	}

	fmt.Printf("Refreshing %d resource(s)...\n\n", len(currentState.Resources))

	drifted := 0
	deleted := 0

	for _, res := range currentState.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		prov, err := registry.Get(res.Provider)
		if err != nil {
			fmt.Printf("  %s: SKIP (provider %s not available)\n", addr, res.Provider)
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
			fmt.Printf("  %s: ERROR (%v)\n", addr, err)
			continue
		}

		if !resp.Exists {
			fmt.Printf("  \033[31m%s: DELETED (no longer exists in provider)\033[0m\n", addr)
			deleted++
			continue
		}

		// Compare returned state with stored state
		if len(resp.NewStateJson) > 0 {
			var newOutputs map[string]any
			if err := json.Unmarshal(resp.NewStateJson, &newOutputs); err == nil {
				if fmt.Sprintf("%v", newOutputs) != fmt.Sprintf("%v", res.Outputs) {
					fmt.Printf("  \033[33m%s: DRIFTED (state updated)\033[0m\n", addr)
					res.Outputs = newOutputs
					drifted++
				} else {
					fmt.Printf("  %s: OK\n", addr)
				}
			}
		} else {
			fmt.Printf("  %s: OK\n", addr)
		}
	}

	// Write updated state
	if drifted > 0 || deleted > 0 {
		currentState.Serial++
		if err := stateMgr.Write(ctx, currentState); err != nil {
			return fmt.Errorf("failed to write state: %w", err)
		}
	}

	fmt.Printf("\nRefresh complete. %d drifted, %d deleted.\n", drifted, deleted)
	return nil
}

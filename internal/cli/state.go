package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage Picklr state",
	Long:  `Commands for inspecting and modifying Picklr state.`,
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources in state",
	RunE:  runStateList,
}

var stateShowCmd = &cobra.Command{
	Use:   "show <address>",
	Short: "Show attributes of a single resource",
	Args:  cobra.ExactArgs(1),
	RunE:  runStateShow,
}

var stateMvCmd = &cobra.Command{
	Use:   "mv <source> <destination>",
	Short: "Move a resource to a new address",
	Args:  cobra.ExactArgs(2),
	RunE:  runStateMv,
}

var stateRmCmd = &cobra.Command{
	Use:   "rm <address>",
	Short: "Remove a resource from state (does not destroy)",
	Args:  cobra.ExactArgs(1),
	RunE:  runStateRm,
}

func init() {
	stateCmd.AddCommand(stateListCmd)
	stateCmd.AddCommand(stateShowCmd)
	stateCmd.AddCommand(stateMvCmd)
	stateCmd.AddCommand(stateRmCmd)
}

func loadStateMgr() (*state.Manager, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	evaluator := eval.NewEvaluator(wd)
	return state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator), nil
}

func runStateList(cmd *cobra.Command, args []string) error {
	mgr, err := loadStateMgr()
	if err != nil {
		return err
	}

	s, err := mgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	if len(s.Resources) == 0 {
		fmt.Println("No resources in state.")
		return nil
	}

	fmt.Printf("State version: %d, serial: %d, lineage: %s\n\n", s.Version, s.Serial, s.Lineage)
	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		fmt.Printf("  %s (provider: %s)\n", addr, res.Provider)
	}
	fmt.Printf("\nTotal: %d resource(s)\n", len(s.Resources))

	return nil
}

func runStateShow(cmd *cobra.Command, args []string) error {
	mgr, err := loadStateMgr()
	if err != nil {
		return err
	}

	s, err := mgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	target := args[0]
	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if addr == target {
			fmt.Printf("# %s\n", addr)
			fmt.Printf("  provider = %s\n", res.Provider)
			fmt.Printf("  type     = %s\n", res.Type)
			fmt.Printf("  name     = %s\n", res.Name)

			if len(res.Inputs) > 0 {
				fmt.Println("\n  Inputs:")
				for k, v := range res.Inputs {
					fmt.Printf("    %s = %v\n", k, v)
				}
			}

			if len(res.Outputs) > 0 {
				fmt.Println("\n  Outputs:")
				for k, v := range res.Outputs {
					fmt.Printf("    %s = %v\n", k, v)
				}
			}

			if res.InputsHash != "" {
				fmt.Printf("\n  inputs_hash = %s\n", res.InputsHash)
			}

			return nil
		}
	}

	return fmt.Errorf("resource %s not found in state", target)
}

func runStateMv(cmd *cobra.Command, args []string) error {
	mgr, err := loadStateMgr()
	if err != nil {
		return err
	}

	if err := mgr.Lock(); err != nil {
		return err
	}
	defer mgr.Unlock()

	s, err := mgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	src, dst := args[0], args[1]
	found := false

	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if addr == src {
			// Parse new address
			parts := strings.SplitN(dst, ".", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid destination address %q, expected format type.name", dst)
			}
			res.Type = parts[0]
			res.Name = parts[1]
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("resource %s not found in state", src)
	}

	if err := mgr.Write(cmd.Context(), s); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	fmt.Printf("Moved %s to %s\n", src, dst)
	return nil
}

func runStateRm(cmd *cobra.Command, args []string) error {
	mgr, err := loadStateMgr()
	if err != nil {
		return err
	}

	if err := mgr.Lock(); err != nil {
		return err
	}
	defer mgr.Unlock()

	s, err := mgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	target := args[0]
	newResources := make([]*ir.ResourceState, 0, len(s.Resources))
	found := false

	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if addr == target {
			found = true
			continue
		}
		newResources = append(newResources, res)
	}

	if !found {
		return fmt.Errorf("resource %s not found in state", target)
	}

	s.Resources = newResources
	if err := mgr.Write(cmd.Context(), s); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	fmt.Printf("Removed %s from state (resource was NOT destroyed)\n", target)
	return nil
}

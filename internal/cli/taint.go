package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var taintCmd = &cobra.Command{
	Use:   "taint <address>",
	Short: "Mark a resource for recreation",
	Long: `Marks a resource as tainted, forcing it to be destroyed and recreated
on the next apply.`,
	Args: cobra.ExactArgs(1),
	RunE: runTaint,
}

var untaintCmd = &cobra.Command{
	Use:   "untaint <address>",
	Short: "Remove taint from a resource",
	Long:  `Removes the taint mark from a resource, preventing forced recreation.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runUntaint,
}

func runTaint(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)

	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	s, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	target := args[0]
	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if addr == target {
			if res.Outputs == nil {
				res.Outputs = make(map[string]any)
			}
			res.Outputs["_tainted"] = true
			if err := stateMgr.Write(ctx, s); err != nil {
				return fmt.Errorf("failed to write state: %w", err)
			}
			fmt.Printf("Resource %s has been tainted. It will be recreated on next apply.\n", target)
			return nil
		}
	}

	return fmt.Errorf("resource %s not found in state", target)
}

func runUntaint(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)

	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	s, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	target := args[0]
	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		if addr == target {
			if res.Outputs != nil {
				delete(res.Outputs, "_tainted")
			}
			if err := stateMgr.Write(ctx, s); err != nil {
				return fmt.Errorf("failed to write state: %w", err)
			}
			fmt.Printf("Resource %s has been untainted.\n", target)
			return nil
		}
	}

	return fmt.Errorf("resource %s not found in state", target)
}

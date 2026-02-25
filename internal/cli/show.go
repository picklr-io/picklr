package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var (
	showJSON bool
)

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current state",
	Long:  `Displays a human-readable view of the current state file.`,
	RunE:  runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "Output in JSON format")
}

func runShow(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)

	s, err := stateMgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	if showJSON {
		data, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal state: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("State: version=%d serial=%d lineage=%s\n", s.Version, s.Serial, s.Lineage)
	fmt.Printf("Resources: %d\n\n", len(s.Resources))

	for _, res := range s.Resources {
		addr := fmt.Sprintf("%s.%s", res.Type, res.Name)
		fmt.Printf("# %s\n", addr)
		fmt.Printf("  provider = %s\n", res.Provider)

		if len(res.Outputs) > 0 {
			for k, v := range res.Outputs {
				fmt.Printf("  %s = %v\n", k, v)
			}
		}
		fmt.Println()
	}

	if len(s.Outputs) > 0 {
		fmt.Println("Outputs:")
		for k, v := range s.Outputs {
			fmt.Printf("  %s = %v\n", k, v)
		}
	}

	return nil
}

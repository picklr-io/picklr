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
	outputJSON bool
)

var outputCmd = &cobra.Command{
	Use:   "output [name]",
	Short: "Show output values from state",
	Long: `Reads output values from the state file.

If no name is given, all outputs are displayed. If a name is given,
only that output's value is printed.`,
	RunE: runOutput,
}

func init() {
	outputCmd.Flags().BoolVar(&outputJSON, "json", false, "Output in JSON format")
}

func runOutput(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)

	s, err := stateMgr.Read(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	if len(args) > 0 {
		// Show specific output
		name := args[0]
		val, ok := s.Outputs[name]
		if !ok {
			return fmt.Errorf("output %q not found", name)
		}
		if outputJSON {
			data, _ := json.Marshal(val)
			fmt.Println(string(data))
		} else {
			fmt.Println(val)
		}
		return nil
	}

	// Show all outputs
	if len(s.Outputs) == 0 {
		fmt.Println("No outputs defined.")
		return nil
	}

	if outputJSON {
		data, _ := json.MarshalIndent(s.Outputs, "", "  ")
		fmt.Println(string(data))
	} else {
		for k, v := range s.Outputs {
			fmt.Printf("%s = %v\n", k, v)
		}
	}

	return nil
}

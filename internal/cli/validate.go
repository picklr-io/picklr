package cli

import (
	"fmt"
	"os"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate PKL configuration files",
	Long:  `Validates the syntax and types of all PKL configuration files.`,
	RunE:  runValidate,
}

func runValidate(cmd *cobra.Command, args []string) error {
	fmt.Println("Validating configuration...")

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	evaluator := eval.NewEvaluator(wd)

	// Validate main.pkl
	fmt.Print("Checking main.pkl... ")
	if _, err := evaluator.LoadConfig(cmd.Context(), "main.pkl", nil); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("validation failed: %w", err)
	}
	fmt.Println("OK")

	fmt.Println("\nConfiguration is valid!")
	return nil
}

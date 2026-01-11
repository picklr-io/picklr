package cli

import (
	"fmt"
	"os"

	"path/filepath"

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
	entryPoint := "main.pkl"

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
			entryPoint = filepath.Base(absPath)
		}
	}

	evaluator := eval.NewEvaluator(wd)

	// Validate main.pkl
	fmt.Printf("Checking %s... ", entryPoint)
	if _, err := evaluator.LoadConfig(cmd.Context(), entryPoint, nil); err != nil {
		fmt.Println("FAILED")
		return fmt.Errorf("validation failed: %w", err)
	}
	fmt.Println("OK")

	fmt.Println("\nConfiguration is valid!")
	return nil
}

package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Picklr project",
	Long:  `Creates a new Picklr project with default configuration files.`,
	RunE:  runInit,
}

func runInit(cmd *cobra.Command, args []string) error {
	// Create .picklr directory
	if err := os.MkdirAll(".picklr", 0755); err != nil {
		return fmt.Errorf("failed to create .picklr directory: %w", err)
	}

	// Create main.pkl if it doesn't exist
	mainPkl := "main.pkl"
	if _, err := os.Stat(mainPkl); os.IsNotExist(err) {
		content := `// Picklr configuration
// See: https://github.com/picklr-io/picklr

amends "picklr:Config"

providers {
  // Add your provider configurations here
}

resources {
  // Add your resources here
}

outputs {
  // Add your outputs here
}
`
		if err := os.WriteFile(mainPkl, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create %s: %w", mainPkl, err)
		}
		fmt.Printf("Created %s\n", mainPkl)
	}

	// Create empty state file
	statePath := filepath.Join(".picklr", "state.pkl")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		content := `// Picklr state file - DO NOT EDIT MANUALLY unless you know what you're doing
amends "picklr:State"

version = 1
serial = 0
lineage = ""

resources {}

outputs {}
`
		if err := os.WriteFile(statePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create state file: %w", err)
		}
		fmt.Printf("Created %s\n", statePath)
	}

	fmt.Println("\nPicklr initialized successfully!")
	fmt.Println("Next steps:")
	fmt.Println("  1. Edit main.pkl to define your infrastructure")
	fmt.Println("  2. Run 'picklr plan' to see what will be created")
	fmt.Println("  3. Run 'picklr apply' to create your infrastructure")

	return nil
}

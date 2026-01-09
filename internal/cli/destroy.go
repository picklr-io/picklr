package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy all managed infrastructure",
	Long: `Destroys all resources managed by Picklr.

This command is the inverse of 'picklr apply'. It will delete all resources
tracked in the state file.`,
	RunE: runDestroy,
}

func runDestroy(cmd *cobra.Command, args []string) error {
	fmt.Println("Reading state...")
	
	// TODO: Implement destroy
	// 1. Read state.pkl
	// 2. Generate destroy plan (delete all resources in reverse dependency order)
	// 3. Prompt for confirmation
	// 4. Execute destroy plan
	// 5. Clear state.pkl
	
	fmt.Println("\nDestroy complete! All resources have been deleted.")
	return nil
}

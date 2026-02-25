package cli

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "picklr",
	Short: "PKL-native Infrastructure as Code",
	Long: `Picklr is a type-safe infrastructure as code tool built on Apple's PKL language.

It provides a clean, deterministic way to manage cloud infrastructure with:
  • Type-safe resource definitions
  • Git-native state management  
  • Human-readable plans and state files
  • Unified language for config, plans, and state`,
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(planCmd)
	rootCmd.AddCommand(applyCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(stateCmd)
	rootCmd.AddCommand(importCmd)
	rootCmd.AddCommand(outputCmd)
	rootCmd.AddCommand(showCmd)
	rootCmd.AddCommand(refreshCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(taintCmd)
	rootCmd.AddCommand(untaintCmd)
}

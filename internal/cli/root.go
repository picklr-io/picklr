package cli

import (
	"github.com/picklr-io/picklr/internal/logging"
	"github.com/spf13/cobra"
)

var (
	noColor  bool
	targets  []string
	logLevel string
)

var rootCmd = &cobra.Command{
	Use:   "picklr",
	Short: "PKL-native Infrastructure as Code",
	Long: `Picklr is a type-safe infrastructure as code tool built on Apple's PKL language.

It provides a clean, deterministic way to manage cloud infrastructure with:
  - Type-safe resource definitions
  - Git-native state management
  - Human-readable plans and state files
  - Unified language for config, plans, and state`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(logLevel)
	},
}

// Execute runs the root command
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable color output")
	rootCmd.PersistentFlags().StringSliceVar(&targets, "target", nil, "Restrict operations to specific resources (can be specified multiple times)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info", "Set log level (debug, info, warn, error)")

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
	rootCmd.AddCommand(workspaceCmd)
	rootCmd.AddCommand(fmtCmd)
	rootCmd.AddCommand(consoleCmd)
	rootCmd.AddCommand(policyCmd)
	rootCmd.AddCommand(migrateCmd)
	rootCmd.AddCommand(versionCmd)
}

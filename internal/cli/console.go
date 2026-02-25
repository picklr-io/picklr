package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/state"
	"github.com/spf13/cobra"
)

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Interactive console for exploring state and config",
	Long: `Opens an interactive console that allows you to inspect the current
state and configuration.

Available commands:
  state              Show current state summary
  state.resources    List all resources
  state.outputs      Show all outputs
  resource <addr>    Show a specific resource
  output <name>      Show a specific output
  config             Show current config summary
  help               Show available commands
  exit / quit        Exit the console`,
	RunE: runConsole,
}

func runConsole(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, WorkspaceStatePath()), evaluator)

	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Try to load config
	cfg, _ := evaluator.LoadConfig(ctx, "main.pkl", nil)

	fmt.Println("Picklr Console (type 'help' for commands, 'exit' to quit)")
	fmt.Printf("State: %d resources, serial %d\n", len(currentState.Resources), currentState.Serial)
	if cfg != nil {
		fmt.Printf("Config: %d resources defined\n", len(cfg.Resources))
	}
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("picklr> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		command := parts[0]

		switch command {
		case "exit", "quit":
			fmt.Println("Bye!")
			return nil

		case "help":
			fmt.Println("Available commands:")
			fmt.Println("  state              - Show state summary")
			fmt.Println("  state.resources    - List all resources in state")
			fmt.Println("  state.outputs      - Show all state outputs")
			fmt.Println("  resource <addr>    - Show a specific resource")
			fmt.Println("  output <name>      - Show a specific output")
			fmt.Println("  config             - Show config summary")
			fmt.Println("  config.resources   - List all resources in config")
			fmt.Println("  json <expression>  - Output as JSON")
			fmt.Println("  exit / quit        - Exit the console")

		case "state":
			fmt.Printf("Version:   %d\n", currentState.Version)
			fmt.Printf("Serial:    %d\n", currentState.Serial)
			fmt.Printf("Lineage:   %s\n", currentState.Lineage)
			fmt.Printf("Resources: %d\n", len(currentState.Resources))
			fmt.Printf("Outputs:   %d\n", len(currentState.Outputs))

		case "state.resources":
			if len(currentState.Resources) == 0 {
				fmt.Println("No resources in state.")
			} else {
				for _, res := range currentState.Resources {
					fmt.Printf("  %s.%s (provider: %s)\n", res.Type, res.Name, res.Provider)
				}
			}

		case "state.outputs":
			if len(currentState.Outputs) == 0 {
				fmt.Println("No outputs.")
			} else {
				for k, v := range currentState.Outputs {
					fmt.Printf("  %s = %v\n", k, v)
				}
			}

		case "resource":
			if len(parts) < 2 {
				fmt.Println("Usage: resource <address>")
				continue
			}
			addr := parts[1]
			found := false
			for _, res := range currentState.Resources {
				resAddr := fmt.Sprintf("%s.%s", res.Type, res.Name)
				if resAddr == addr {
					data, _ := json.MarshalIndent(res, "", "  ")
					fmt.Println(string(data))
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("Resource %s not found in state.\n", addr)
			}

		case "output":
			if len(parts) < 2 {
				fmt.Println("Usage: output <name>")
				continue
			}
			name := parts[1]
			if val, ok := currentState.Outputs[name]; ok {
				fmt.Printf("%s = %v\n", name, val)
			} else {
				fmt.Printf("Output %s not found.\n", name)
			}

		case "config":
			if cfg == nil {
				fmt.Println("No configuration loaded.")
			} else {
				fmt.Printf("Resources: %d\n", len(cfg.Resources))
				fmt.Printf("Outputs:   %d\n", len(cfg.Outputs))
			}

		case "config.resources":
			if cfg == nil {
				fmt.Println("No configuration loaded.")
			} else if len(cfg.Resources) == 0 {
				fmt.Println("No resources in config.")
			} else {
				for _, res := range cfg.Resources {
					fmt.Printf("  %s.%s (provider: %s)\n", res.Type, res.Name, res.Provider)
				}
			}

		case "json":
			if len(parts) < 2 {
				fmt.Println("Usage: json <expression>")
				continue
			}
			expr := parts[1]
			switch expr {
			case "state":
				data, _ := json.MarshalIndent(currentState, "", "  ")
				fmt.Println(string(data))
			case "state.resources":
				data, _ := json.MarshalIndent(currentState.Resources, "", "  ")
				fmt.Println(string(data))
			case "state.outputs":
				data, _ := json.MarshalIndent(currentState.Outputs, "", "  ")
				fmt.Println(string(data))
			default:
				fmt.Printf("Unknown expression: %s\n", expr)
			}

		default:
			fmt.Printf("Unknown command: %s (type 'help' for available commands)\n", command)
		}
	}

	return nil
}

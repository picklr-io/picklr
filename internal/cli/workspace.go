package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage workspaces",
	Long: `Workspaces allow you to manage multiple distinct sets of infrastructure
resources with the same configuration. Each workspace has its own state file.

The default workspace is called "default".`,
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List workspaces",
	RunE:  runWorkspaceList,
}

var workspaceNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceNew,
}

var workspaceSelectCmd = &cobra.Command{
	Use:   "select <name>",
	Short: "Switch to another workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceSelect,
}

var workspaceDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a workspace",
	Args:  cobra.ExactArgs(1),
	RunE:  runWorkspaceDelete,
}

var workspaceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current workspace name",
	RunE:  runWorkspaceShow,
}

func init() {
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceNewCmd)
	workspaceCmd.AddCommand(workspaceSelectCmd)
	workspaceCmd.AddCommand(workspaceDeleteCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
}

func picklrDir() string {
	return ".picklr"
}

func workspaceFile() string {
	return filepath.Join(picklrDir(), "workspace")
}

func currentWorkspace() string {
	data, err := os.ReadFile(workspaceFile())
	if err != nil {
		return "default"
	}
	ws := strings.TrimSpace(string(data))
	if ws == "" {
		return "default"
	}
	return ws
}

// WorkspaceStatePath returns the state file path for the current workspace.
func WorkspaceStatePath() string {
	ws := currentWorkspace()
	if ws == "default" {
		return filepath.Join(picklrDir(), "state.pkl")
	}
	return filepath.Join(picklrDir(), fmt.Sprintf("state.%s.pkl", ws))
}

func listWorkspaces() ([]string, error) {
	entries, err := os.ReadDir(picklrDir())
	if err != nil {
		return nil, fmt.Errorf("failed to read .picklr directory: %w", err)
	}

	workspaces := []string{"default"}
	seen := map[string]bool{"default": true}

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "state.") && strings.HasSuffix(name, ".pkl") {
			// state.<name>.pkl
			ws := strings.TrimPrefix(name, "state.")
			ws = strings.TrimSuffix(ws, ".pkl")
			if ws != "" && !seen[ws] {
				workspaces = append(workspaces, ws)
				seen[ws] = true
			}
		}
	}

	return workspaces, nil
}

func runWorkspaceList(cmd *cobra.Command, args []string) error {
	workspaces, err := listWorkspaces()
	if err != nil {
		return err
	}

	current := currentWorkspace()
	for _, ws := range workspaces {
		if ws == current {
			fmt.Printf("* %s\n", ws)
		} else {
			fmt.Printf("  %s\n", ws)
		}
	}
	return nil
}

func runWorkspaceNew(cmd *cobra.Command, args []string) error {
	name := args[0]
	if name == "default" {
		return fmt.Errorf("cannot create a workspace named 'default' - it already exists")
	}

	statePath := filepath.Join(picklrDir(), fmt.Sprintf("state.%s.pkl", name))
	if _, err := os.Stat(statePath); err == nil {
		return fmt.Errorf("workspace %q already exists", name)
	}

	// Create empty state for new workspace
	lineage := generateUUID()
	content := fmt.Sprintf(`// Picklr state file - DO NOT EDIT MANUALLY unless you know what you're doing
amends "picklr:State"

version = 1
serial = 0
lineage = %q

resources {}

outputs {}
`, lineage)
	if err := os.WriteFile(statePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to create workspace state: %w", err)
	}

	// Switch to the new workspace
	if err := os.WriteFile(workspaceFile(), []byte(name), 0644); err != nil {
		return fmt.Errorf("failed to switch workspace: %w", err)
	}

	fmt.Printf("Created and switched to workspace %q\n", name)
	return nil
}

func runWorkspaceSelect(cmd *cobra.Command, args []string) error {
	name := args[0]

	if name != "default" {
		statePath := filepath.Join(picklrDir(), fmt.Sprintf("state.%s.pkl", name))
		if _, err := os.Stat(statePath); os.IsNotExist(err) {
			return fmt.Errorf("workspace %q does not exist", name)
		}
	}

	if err := os.WriteFile(workspaceFile(), []byte(name), 0644); err != nil {
		return fmt.Errorf("failed to switch workspace: %w", err)
	}

	fmt.Printf("Switched to workspace %q\n", name)
	return nil
}

func runWorkspaceDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	if name == "default" {
		return fmt.Errorf("cannot delete the default workspace")
	}

	if currentWorkspace() == name {
		return fmt.Errorf("cannot delete the currently active workspace %q - switch to another workspace first", name)
	}

	statePath := filepath.Join(picklrDir(), fmt.Sprintf("state.%s.pkl", name))
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		return fmt.Errorf("workspace %q does not exist", name)
	}

	if err := os.Remove(statePath); err != nil {
		return fmt.Errorf("failed to delete workspace state: %w", err)
	}

	// Also remove lock file if exists
	os.Remove(statePath + ".lock")

	fmt.Printf("Deleted workspace %q\n", name)
	return nil
}

func runWorkspaceShow(cmd *cobra.Command, args []string) error {
	fmt.Println(currentWorkspace())
	return nil
}

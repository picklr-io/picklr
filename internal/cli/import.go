package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/picklr-io/picklr/internal/eval"
	"github.com/picklr-io/picklr/internal/ir"
	"github.com/picklr-io/picklr/internal/provider"
	"github.com/picklr-io/picklr/internal/state"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <resource-address> <cloud-id>",
	Short: "Import existing infrastructure into Picklr state",
	Long: `Import an existing resource into the Picklr state file.

This does not generate configuration - you must write the corresponding
PKL configuration manually. It only adds the resource to the state so
that Picklr will manage it going forward.

Example:
  picklr import aws:S3.Bucket.my-bucket my-bucket-name`,
	Args: cobra.ExactArgs(2),
	RunE: runImport,
}

func runImport(cmd *cobra.Command, args []string) error {
	addr := args[0]
	cloudID := args[1]

	// Parse address: type.name
	parts := strings.SplitN(addr, ".", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid resource address %q, expected format type.name", addr)
	}
	resourceType := parts[0]
	resourceName := parts[1]

	// Determine provider from type
	providerName := "null"
	if strings.Contains(resourceType, ":") {
		providerName = strings.SplitN(resourceType, ":", 2)[0]
	} else if strings.HasPrefix(resourceType, "docker_") {
		providerName = "docker"
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	ctx := cmd.Context()
	evaluator := eval.NewEvaluator(wd)
	stateMgr := state.NewManager(filepath.Join(wd, ".picklr", "state.pkl"), evaluator)
	registry := provider.NewRegistry()

	// Lock state
	if err := stateMgr.Lock(); err != nil {
		return err
	}
	defer stateMgr.Unlock()

	// Load provider
	if err := registry.LoadProvider(providerName); err != nil {
		return fmt.Errorf("failed to load provider %s: %w", providerName, err)
	}

	prov, err := registry.Get(providerName)
	if err != nil {
		return fmt.Errorf("provider not available: %w", err)
	}

	// Read state from cloud
	fmt.Printf("Importing %s (id: %s)...\n", addr, cloudID)
	resp, err := prov.Read(ctx, &pb.ReadRequest{
		Type: resourceType,
		Id:   cloudID,
	})
	if err != nil {
		return fmt.Errorf("failed to read resource from provider: %w", err)
	}

	if !resp.Exists {
		return fmt.Errorf("resource %s with id %s does not exist", resourceType, cloudID)
	}

	// Parse outputs
	var outputs map[string]any
	if len(resp.NewStateJson) > 0 {
		if err := json.Unmarshal(resp.NewStateJson, &outputs); err != nil {
			return fmt.Errorf("failed to parse provider response: %w", err)
		}
	}

	// Read current state
	currentState, err := stateMgr.Read(ctx)
	if err != nil {
		return fmt.Errorf("failed to read state: %w", err)
	}

	// Check for duplicate
	for _, res := range currentState.Resources {
		if res.Type == resourceType && res.Name == resourceName {
			return fmt.Errorf("resource %s already exists in state", addr)
		}
	}

	// Add to state
	currentState.Resources = append(currentState.Resources, &ir.ResourceState{
		Type:     resourceType,
		Name:     resourceName,
		Provider: providerName,
		Inputs:   map[string]any{},
		Outputs:  outputs,
	})
	currentState.Serial++

	// Write state
	if err := stateMgr.Write(ctx, currentState); err != nil {
		return fmt.Errorf("failed to write state: %w", err)
	}

	fmt.Printf("Successfully imported %s\n", addr)
	fmt.Println("Note: You must also write the corresponding PKL configuration for this resource.")
	return nil
}

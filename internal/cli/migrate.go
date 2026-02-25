package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate-from-terraform [tf-dir]",
	Short: "Migrate from Terraform to Picklr",
	Long: `Converts Terraform state to Picklr state format.

This command reads a Terraform state file (terraform.tfstate) and converts
it to a Picklr state file (.picklr/state.pkl).

Note: This performs a best-effort conversion. You will still need to write
the corresponding PKL configuration manually, but the state conversion
ensures Picklr will manage the existing resources without recreating them.

Example:
  picklr migrate-from-terraform .
  picklr migrate-from-terraform /path/to/terraform/project`,
	RunE: runMigrate,
}

// TerraformState represents the Terraform state file format.
type TerraformState struct {
	Version          int                    `json:"version"`
	TerraformVersion string                 `json:"terraform_version"`
	Serial           int                    `json:"serial"`
	Lineage          string                 `json:"lineage"`
	Outputs          map[string]TFOutput    `json:"outputs"`
	Resources        []TFResource           `json:"resources"`
}

// TFOutput represents a Terraform state output.
type TFOutput struct {
	Value any    `json:"value"`
	Type  string `json:"type"`
}

// TFResource represents a Terraform state resource.
type TFResource struct {
	Module    string       `json:"module"`
	Mode      string       `json:"mode"` // "managed" or "data"
	Type      string       `json:"type"`
	Name      string       `json:"name"`
	Provider  string       `json:"provider"`
	Instances []TFInstance `json:"instances"`
}

// TFInstance represents a Terraform resource instance.
type TFInstance struct {
	SchemaVersion int            `json:"schema_version"`
	Attributes    map[string]any `json:"attributes"`
	Private       string         `json:"private"`
	Dependencies  []string       `json:"dependencies"`
}

func runMigrate(cmd *cobra.Command, args []string) error {
	tfDir := "."
	if len(args) > 0 {
		tfDir = args[0]
	}

	statePath := filepath.Join(tfDir, "terraform.tfstate")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("failed to read terraform state from %s: %w", statePath, err)
	}

	var tfState TerraformState
	if err := json.Unmarshal(data, &tfState); err != nil {
		return fmt.Errorf("failed to parse terraform state: %w", err)
	}

	fmt.Printf("Found Terraform state: version=%d serial=%d lineage=%s\n",
		tfState.Version, tfState.Serial, tfState.Lineage)
	fmt.Printf("Resources: %d\n", len(tfState.Resources))

	// Ensure .picklr directory exists
	if err := os.MkdirAll(".picklr", 0755); err != nil {
		return fmt.Errorf("failed to create .picklr directory: %w", err)
	}

	// Convert to Picklr state
	outPath := filepath.Join(".picklr", "state.pkl")
	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create state file: %w", err)
	}
	defer f.Close()

	lineage := tfState.Lineage
	if lineage == "" {
		lineage = generateUUID()
	}

	fmt.Fprintf(f, "// Picklr state file - migrated from Terraform\n")
	fmt.Fprintf(f, "amends \"../../pkg/schemas/State.pkl\"\n\n")
	fmt.Fprintf(f, "version = 1\n")
	fmt.Fprintf(f, "serial = %d\n", tfState.Serial)
	fmt.Fprintf(f, "lineage = %q\n\n", lineage)

	// Convert outputs
	if len(tfState.Outputs) > 0 {
		fmt.Fprintf(f, "outputs {\n")
		for k, v := range tfState.Outputs {
			fmt.Fprintf(f, "  [%q] = %q\n", k, fmt.Sprintf("%v", v.Value))
		}
		fmt.Fprintf(f, "}\n\n")
	} else {
		fmt.Fprintf(f, "outputs = new {}\n\n")
	}

	// Convert resources
	fmt.Fprintf(f, "resources {\n")
	converted := 0
	for _, res := range tfState.Resources {
		if res.Mode != "managed" {
			continue
		}

		providerName := mapTFProvider(res.Provider)
		resourceType := mapTFResourceType(res.Type, providerName)

		for _, inst := range res.Instances {
			fmt.Fprintf(f, "  new {\n")
			fmt.Fprintf(f, "    type = %q\n", resourceType)
			fmt.Fprintf(f, "    name = %q\n", res.Name)
			fmt.Fprintf(f, "    provider = %q\n", providerName)
			fmt.Fprintf(f, "    inputs = new {}\n")
			fmt.Fprintf(f, "    inputsHash = \"\"\n")

			// Write attributes as outputs
			if len(inst.Attributes) > 0 {
				fmt.Fprintf(f, "    outputs {\n")
				for k, v := range inst.Attributes {
					fmt.Fprintf(f, "      [%q] = %q\n", k, fmt.Sprintf("%v", v))
				}
				fmt.Fprintf(f, "    }\n")
			} else {
				fmt.Fprintf(f, "    outputs = new {}\n")
			}

			fmt.Fprintf(f, "  }\n")
			converted++
		}
	}
	fmt.Fprintf(f, "}\n")

	fmt.Printf("\nMigration complete! Converted %d resources to %s\n", converted, outPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Write corresponding PKL configuration in main.pkl")
	fmt.Println("  2. Run 'picklr plan' to verify no changes are needed")
	fmt.Println("  3. If plan shows changes, adjust your PKL config to match")
	return nil
}

// mapTFProvider maps a Terraform provider string to Picklr provider name.
func mapTFProvider(tfProvider string) string {
	// Terraform provider format: registry.terraform.io/hashicorp/aws
	parts := strings.Split(tfProvider, "/")
	name := parts[len(parts)-1]
	// Strip brackets like [\"registry.terraform.io/hashicorp/aws\"]
	name = strings.Trim(name, "[]\"")
	switch name {
	case "aws":
		return "aws"
	case "docker":
		return "docker"
	case "null":
		return "null"
	default:
		return name
	}
}

// mapTFResourceType maps a Terraform resource type to Picklr format.
func mapTFResourceType(tfType, provider string) string {
	// Terraform: aws_s3_bucket -> Picklr: aws:S3.Bucket
	typeMap := map[string]string{
		"aws_s3_bucket":               "aws:S3.Bucket",
		"aws_instance":                "aws:EC2.Instance",
		"aws_vpc":                     "aws:EC2.Vpc",
		"aws_subnet":                  "aws:EC2.Subnet",
		"aws_security_group":          "aws:EC2.SecurityGroup",
		"aws_iam_role":                "aws:IAM.Role",
		"aws_iam_policy":              "aws:IAM.Policy",
		"aws_lambda_function":         "aws:Lambda.Function",
		"aws_dynamodb_table":          "aws:DynamoDB.Table",
		"aws_db_instance":             "aws:RDS.Instance",
		"aws_sqs_queue":               "aws:SQS.Queue",
		"aws_sns_topic":               "aws:SNS.Topic",
		"aws_ecr_repository":          "aws:ECR.Repository",
		"aws_ecs_cluster":             "aws:ECS.Cluster",
		"aws_ecs_service":             "aws:ECS.Service",
		"aws_ecs_task_definition":     "aws:ECS.TaskDefinition",
		"aws_lb":                      "aws:ELBv2.LoadBalancer",
		"aws_lb_target_group":         "aws:ELBv2.TargetGroup",
		"aws_lb_listener":             "aws:ELBv2.Listener",
		"aws_route53_zone":            "aws:Route53.HostedZone",
		"aws_route53_record":          "aws:Route53.RecordSet",
		"aws_internet_gateway":        "aws:EC2.InternetGateway",
		"aws_nat_gateway":             "aws:EC2.NatGateway",
		"aws_eip":                     "aws:EC2.ElasticIP",
		"aws_route_table":             "aws:EC2.RouteTable",
		"aws_cloudwatch_log_group":    "aws:CloudWatch.LogGroup",
		"aws_cloudwatch_metric_alarm": "aws:CloudWatch.Alarm",
		"aws_kms_key":                 "aws:KMS.Key",
		"aws_secretsmanager_secret":   "aws:SecretsManager.Secret",
		"null_resource":               "null_resource",
		"docker_container":            "docker_container",
		"docker_image":                "docker_image",
		"docker_network":              "docker_network",
		"docker_volume":               "docker_volume",
	}

	if mapped, ok := typeMap[tfType]; ok {
		return mapped
	}

	// Best effort: convert underscore format to namespaced
	if provider == "aws" {
		return "aws:" + tfType
	}
	return tfType
}

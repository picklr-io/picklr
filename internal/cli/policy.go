package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/spf13/cobra"
)

var (
	policyFile string
)

var policyCmd = &cobra.Command{
	Use:   "policy-check [plan-file]",
	Short: "Check a plan against policy rules",
	Long: `Evaluates a saved plan against policy rules defined in a JSON policy file.

Policy rules can enforce constraints like:
  - No public S3 buckets
  - All resources must have tags
  - Prevent deletion of critical resources
  - Restrict allowed resource types

Example policy file:
  {
    "rules": [
      {
        "name": "no-public-s3",
        "description": "S3 buckets must not have public-read ACL",
        "resource_type": "aws:S3.Bucket",
        "condition": "property_not_equals",
        "property": "acl",
        "value": "public-read",
        "severity": "error"
      }
    ]
  }`,
	Args: cobra.ExactArgs(1),
	RunE: runPolicyCheck,
}

func init() {
	policyCmd.Flags().StringVarP(&policyFile, "policy", "p", ".picklr/policies.json", "Path to policy file")
}

// PolicyFile represents a collection of policy rules.
type PolicyFile struct {
	Rules []PolicyRule `json:"rules"`
}

// PolicyRule defines a single policy check.
type PolicyRule struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	ResourceType string `json:"resource_type"` // empty = all types
	Condition    string `json:"condition"`      // deny_action, property_equals, property_not_equals, require_property
	Property     string `json:"property"`
	Value        string `json:"value"`
	Severity     string `json:"severity"` // "error", "warning"
}

// PolicyViolation represents a policy check failure.
type PolicyViolation struct {
	Rule     PolicyRule
	Resource string
	Message  string
}

func runPolicyCheck(cmd *cobra.Command, args []string) error {
	planFile := args[0]

	// Load plan
	planData, err := os.ReadFile(planFile)
	if err != nil {
		return fmt.Errorf("failed to read plan file: %w", err)
	}

	var plan ir.Plan
	if err := json.Unmarshal(planData, &plan); err != nil {
		return fmt.Errorf("failed to parse plan: %w", err)
	}

	// Load policies
	policyData, err := os.ReadFile(policyFile)
	if err != nil {
		return fmt.Errorf("failed to read policy file %s: %w", policyFile, err)
	}

	var policies PolicyFile
	if err := json.Unmarshal(policyData, &policies); err != nil {
		return fmt.Errorf("failed to parse policy file: %w", err)
	}

	// Evaluate policies
	violations := evaluatePolicies(&plan, &policies)

	// Report results
	errors := 0
	warnings := 0

	for _, v := range violations {
		severity := strings.ToUpper(v.Rule.Severity)
		if severity == "" || severity == "ERROR" {
			errors++
			fmt.Printf("%s[ERROR]%s %s: %s\n", colorize("\033[31m"), colorize("\033[0m"), v.Rule.Name, v.Message)
		} else {
			warnings++
			fmt.Printf("%s[WARN]%s %s: %s\n", colorize("\033[33m"), colorize("\033[0m"), v.Rule.Name, v.Message)
		}
	}

	fmt.Printf("\nPolicy check complete: %d error(s), %d warning(s)\n", errors, warnings)

	if errors > 0 {
		return fmt.Errorf("policy check failed with %d error(s)", errors)
	}
	return nil
}

func evaluatePolicies(plan *ir.Plan, policies *PolicyFile) []PolicyViolation {
	var violations []PolicyViolation

	for _, rule := range policies.Rules {
		for _, change := range plan.Changes {
			// Check if rule applies to this resource type
			if rule.ResourceType != "" {
				resourceType := ""
				if change.Desired != nil {
					resourceType = change.Desired.Type
				} else if change.Prior != nil {
					resourceType = change.Prior.Type
				}
				if resourceType != rule.ResourceType {
					continue
				}
			}

			switch rule.Condition {
			case "deny_action":
				if strings.EqualFold(change.Action, rule.Value) {
					violations = append(violations, PolicyViolation{
						Rule:     rule,
						Resource: change.Address,
						Message:  fmt.Sprintf("Resource %s: action %s is denied by policy %q", change.Address, change.Action, rule.Description),
					})
				}

			case "property_equals":
				if change.Desired != nil {
					if val, ok := change.Desired.Properties[rule.Property]; ok {
						if fmt.Sprintf("%v", val) == rule.Value {
							violations = append(violations, PolicyViolation{
								Rule:     rule,
								Resource: change.Address,
								Message:  fmt.Sprintf("Resource %s: property %s=%v violates policy %q", change.Address, rule.Property, val, rule.Description),
							})
						}
					}
				}

			case "property_not_equals":
				if change.Desired != nil {
					if val, ok := change.Desired.Properties[rule.Property]; ok {
						if fmt.Sprintf("%v", val) != rule.Value {
							violations = append(violations, PolicyViolation{
								Rule:     rule,
								Resource: change.Address,
								Message:  fmt.Sprintf("Resource %s: property %s=%v violates policy %q (expected %s)", change.Address, rule.Property, val, rule.Description, rule.Value),
							})
						}
					}
				}

			case "require_property":
				if change.Desired != nil && (change.Action == "CREATE" || change.Action == "UPDATE") {
					if _, ok := change.Desired.Properties[rule.Property]; !ok {
						violations = append(violations, PolicyViolation{
							Rule:     rule,
							Resource: change.Address,
							Message:  fmt.Sprintf("Resource %s: missing required property %q per policy %q", change.Address, rule.Property, rule.Description),
						})
					}
				}
			}
		}
	}

	return violations
}

package cli

import (
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/stretchr/testify/assert"
)

func TestFormatPkl(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trailing whitespace",
			input:    "name = \"test\"   \ntype = \"foo\"  \n",
			expected: "name = \"test\"\ntype = \"foo\"\n",
		},
		{
			name:     "ensure trailing newline",
			input:    "name = \"test\"",
			expected: "name = \"test\"\n",
		},
		{
			name:     "collapse blank lines",
			input:    "a = 1\n\n\n\nb = 2\n",
			expected: "a = 1\n\nb = 2\n",
		},
		{
			name:     "already formatted",
			input:    "a = 1\nb = 2\n",
			expected: "a = 1\nb = 2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatPkl(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestColorize(t *testing.T) {
	// When noColor is false, colorize should return the code
	noColor = false
	assert.Equal(t, "\033[31m", colorize("\033[31m"))

	// When noColor is true, colorize should return empty string
	noColor = true
	assert.Equal(t, "", colorize("\033[31m"))

	// Reset
	noColor = false
}

func TestCurrentWorkspace(t *testing.T) {
	// When no workspace file exists, should return "default"
	ws := currentWorkspace()
	assert.Equal(t, "default", ws)
}

func TestWorkspaceStatePath(t *testing.T) {
	// Default workspace uses state.pkl
	path := WorkspaceStatePath()
	assert.Equal(t, ".picklr/state.pkl", path)
}

func TestMapTFProvider(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"registry.terraform.io/hashicorp/aws", "aws"},
		{"registry.terraform.io/hashicorp/docker", "docker"},
		{"registry.terraform.io/hashicorp/null", "null"},
		{"aws", "aws"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapTFProvider(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapTFResourceType(t *testing.T) {
	tests := []struct {
		tfType   string
		provider string
		expected string
	}{
		{"aws_s3_bucket", "aws", "aws:S3.Bucket"},
		{"aws_instance", "aws", "aws:EC2.Instance"},
		{"aws_vpc", "aws", "aws:EC2.Vpc"},
		{"null_resource", "null", "null_resource"},
		{"docker_container", "docker", "docker_container"},
		{"aws_custom_resource", "aws", "aws:aws_custom_resource"},
	}

	for _, tt := range tests {
		t.Run(tt.tfType, func(t *testing.T) {
			result := mapTFResourceType(tt.tfType, tt.provider)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEvaluatePolicies(t *testing.T) {
	t.Run("deny_action", func(t *testing.T) {
		p := testPlan("DELETE", "aws:S3.Bucket", "my-bucket")
		plan := &p
		policies := &PolicyFile{
			Rules: []PolicyRule{
				{
					Name:      "no-delete",
					Condition: "deny_action",
					Value:     "DELETE",
					Severity:  "error",
				},
			},
		}
		violations := evaluatePolicies(plan, policies)
		assert.Len(t, violations, 1)
	})

	t.Run("require_property", func(t *testing.T) {
		p2 := testPlanWithProps("CREATE", "aws:S3.Bucket", "my-bucket", map[string]any{
			"bucket": "test",
		})
		plan := &p2
		policies := &PolicyFile{
			Rules: []PolicyRule{
				{
					Name:         "require-tags",
					Condition:    "require_property",
					Property:     "tags",
					ResourceType: "aws:S3.Bucket",
					Severity:     "error",
				},
			},
		}
		violations := evaluatePolicies(plan, policies)
		assert.Len(t, violations, 1)
	})

	t.Run("property_equals", func(t *testing.T) {
		p3 := testPlanWithProps("CREATE", "aws:S3.Bucket", "my-bucket", map[string]any{
			"acl": "public-read",
		})
		plan := &p3
		policies := &PolicyFile{
			Rules: []PolicyRule{
				{
					Name:         "no-public-acl",
					Description:  "S3 buckets must not be public",
					Condition:    "property_equals",
					Property:     "acl",
					Value:        "public-read",
					ResourceType: "aws:S3.Bucket",
					Severity:     "error",
				},
			},
		}
		violations := evaluatePolicies(plan, policies)
		assert.Len(t, violations, 1)
	})
}

// Helper to create test plan
func testPlan(action, resourceType, name string) ir.Plan {
	return ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: resourceType + "." + name,
				Action:  action,
				Desired: &ir.Resource{
					Type:       resourceType,
					Name:       name,
					Provider:   "aws",
					Properties: map[string]any{},
				},
			},
		},
		Summary: &ir.PlanSummary{},
	}
}

func testPlanWithProps(action, resourceType, name string, props map[string]any) ir.Plan {
	return ir.Plan{
		Changes: []*ir.ResourceChange{
			{
				Address: resourceType + "." + name,
				Action:  action,
				Desired: &ir.Resource{
					Type:       resourceType,
					Name:       name,
					Provider:   "aws",
					Properties: props,
				},
			},
		},
		Summary: &ir.PlanSummary{},
	}
}

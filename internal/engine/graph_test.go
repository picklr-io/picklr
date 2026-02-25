package engine

import (
	"testing"

	"github.com/picklr-io/picklr/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDAG_NoDependencies(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null"},
		{Type: "null_resource", Name: "b", Provider: "null"},
		{Type: "null_resource", Name: "c", Provider: "null"},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	order := dag.CreationOrder()
	assert.Len(t, order, 3)
}

func TestBuildDAG_ExplicitDependsOn(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", DependsOn: []string{"null_resource.b"}},
		{Type: "null_resource", Name: "b", Provider: "null"},
		{Type: "null_resource", Name: "c", Provider: "null", DependsOn: []string{"null_resource.a"}},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	order := dag.CreationOrder()
	require.Len(t, order, 3)

	// b must come before a, a must come before c
	posB := indexOf(order, "null_resource.b")
	posA := indexOf(order, "null_resource.a")
	posC := indexOf(order, "null_resource.c")

	assert.Less(t, posB, posA, "b should come before a")
	assert.Less(t, posA, posC, "a should come before c")
}

func TestBuildDAG_ImplicitPtrRef(t *testing.T) {
	resources := []*ir.Resource{
		{
			Type:     "aws:EC2.Subnet",
			Name:     "my-subnet",
			Provider: "aws",
			Properties: map[string]any{
				"vpcId": "ptr://aws:EC2.Vpc/my-vpc/id",
			},
		},
		{Type: "aws:EC2.Vpc", Name: "my-vpc", Provider: "aws"},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	order := dag.CreationOrder()
	require.Len(t, order, 2)

	posVpc := indexOf(order, "aws:EC2.Vpc.my-vpc")
	posSubnet := indexOf(order, "aws:EC2.Subnet.my-subnet")

	assert.Less(t, posVpc, posSubnet, "VPC should be created before subnet")
}

func TestBuildDAG_CycleDetection(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", DependsOn: []string{"null_resource.b"}},
		{Type: "null_resource", Name: "b", Provider: "null", DependsOn: []string{"null_resource.a"}},
	}

	_, err := BuildDAG(resources)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestBuildDAG_DestructionOrder(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", DependsOn: []string{"null_resource.b"}},
		{Type: "null_resource", Name: "b", Provider: "null"},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	revOrder := dag.DestructionOrder()
	require.Len(t, revOrder, 2)

	// a depends on b, so a should be destroyed first (reverse of creation)
	posA := indexOf(revOrder, "null_resource.a")
	posB := indexOf(revOrder, "null_resource.b")

	assert.Less(t, posA, posB, "a should be destroyed before b")
}

func TestPtrRefToAddr(t *testing.T) {
	tests := []struct {
		ref  string
		want string
	}{
		{"ptr://aws:EC2.Vpc/my-vpc/id", "aws:EC2.Vpc.my-vpc"},
		{"ptr://aws:S3.Bucket/logs/arn", "aws:S3.Bucket.logs"},
		{"not-a-ref", ""},
		{"ptr://short", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			got := ptrRefToAddr(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractPtrRefs(t *testing.T) {
	props := map[string]any{
		"vpcId": "ptr://aws:EC2.Vpc/my-vpc/id",
		"name":  "my-subnet",
		"tags": map[string]any{
			"ref": "ptr://aws:S3.Bucket/logs/arn",
		},
		"list": []any{
			"ptr://aws:IAM.Role/role1/arn",
			"plain-string",
		},
	}

	refs := extractPtrRefs(props)
	assert.Len(t, refs, 3)
	assert.Contains(t, refs, "ptr://aws:EC2.Vpc/my-vpc/id")
	assert.Contains(t, refs, "ptr://aws:S3.Bucket/logs/arn")
	assert.Contains(t, refs, "ptr://aws:IAM.Role/role1/arn")
}

func TestDependencies(t *testing.T) {
	resources := []*ir.Resource{
		{Type: "null_resource", Name: "a", Provider: "null", DependsOn: []string{"null_resource.b", "null_resource.c"}},
		{Type: "null_resource", Name: "b", Provider: "null"},
		{Type: "null_resource", Name: "c", Provider: "null"},
	}

	dag, err := BuildDAG(resources)
	require.NoError(t, err)

	deps := dag.Dependencies("null_resource.a")
	assert.Len(t, deps, 2)
	assert.Contains(t, deps, "null_resource.b")
	assert.Contains(t, deps, "null_resource.c")
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

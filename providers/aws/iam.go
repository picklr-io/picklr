package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type RoleConfig struct {
	Name             string            `json:"name"`
	AssumeRolePolicy string            `json:"assumeRolePolicy"`
	Tags             map[string]string `json:"tags"`
}

type RoleState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

type PolicyConfig struct {
	Name   string `json:"name"`
	Policy string `json:"policy"`
}

type PolicyState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyRole(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior RoleState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.iamClient.DeleteRole(ctx, &iam.DeleteRoleInput{
				RoleName: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete role: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RoleConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// CREATE / UPDATE
	// IAM Roles are eventually consistent.
	input := &iam.CreateRoleInput{
		RoleName:                 &desired.Name,
		AssumeRolePolicyDocument: &desired.AssumeRolePolicy,
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.iamClient.CreateRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	newState := RoleState{
		Name: *resp.Role.RoleName,
		ARN:  *resp.Role.Arn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyPolicy(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior PolicyState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.iamClient.DeletePolicy(ctx, &iam.DeletePolicyInput{
				PolicyArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete policy: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired PolicyConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// CREATE
	resp, err := p.iamClient.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     &desired.Name,
		PolicyDocument: &desired.Policy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	newState := PolicyState{
		Name: *resp.Policy.PolicyName,
		ARN:  *resp.Policy.Arn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

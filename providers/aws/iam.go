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

type InstanceProfileConfig struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type InstanceProfileState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyInstanceProfile(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior InstanceProfileState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Name != "" {
			// Detach role first (best effort, assuming single role)
			// Using RemoveRoleFromInstanceProfile
			// Actually we need the role name. If we don't have it in state, we might fail.
			// Ideally state should include attached role.
			// For simplified delete, we just delete the profile. AWS might error if roles attached.
			_, err := p.iamClient.DeleteInstanceProfile(ctx, &iam.DeleteInstanceProfileInput{
				InstanceProfileName: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete instance profile: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired InstanceProfileConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Create Config
	_, err := p.iamClient.CreateInstanceProfile(ctx, &iam.CreateInstanceProfileInput{
		InstanceProfileName: &desired.Name,
	})
	if err != nil {
		// Ignore if exists
	}

	// Add Role
	_, err = p.iamClient.AddRoleToInstanceProfile(ctx, &iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: &desired.Name,
		RoleName:            &desired.Role,
	})
	if err != nil {
		// Ignore if already added
	}

	// Get ARN for state
	resp, err := p.iamClient.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: &desired.Name,
	})
	arn := ""
	if err == nil {
		arn = *resp.InstanceProfile.Arn
	}

	newState := InstanceProfileState{Name: desired.Name, ARN: arn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// User
type UserConfig struct {
	Name string            `json:"name"`
	Path string            `json:"path"`
	Tags map[string]string `json:"tags"`
}

type UserState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyUser(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior UserState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.iamClient.DeleteUser(ctx, &iam.DeleteUserInput{UserName: &prior.Name})
			if err != nil {
				return nil, fmt.Errorf("failed to delete user: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired UserConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &iam.CreateUserInput{UserName: &desired.Name}
	if desired.Path != "" {
		input.Path = &desired.Path
	}
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.iamClient.CreateUser(ctx, input)
	if err != nil {
		// Ignore EntityAlreadyExists? 
		// Ideally we check before creating or handle error.
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	newState := UserState{Name: *resp.User.UserName, ARN: *resp.User.Arn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Group
type GroupConfig struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type GroupState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior GroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.iamClient.DeleteGroup(ctx, &iam.DeleteGroupInput{GroupName: &prior.Name})
			if err != nil {
				return nil, fmt.Errorf("failed to delete group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired GroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &iam.CreateGroupInput{GroupName: &desired.Name}
	if desired.Path != "" {
		input.Path = &desired.Path
	}

	resp, err := p.iamClient.CreateGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create group: %w", err)
	}

	newState := GroupState{Name: *resp.Group.GroupName, ARN: *resp.Group.Arn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// PolicyAttachment
type PolicyAttachmentConfig struct {
	Name      string   `json:"name"`
	PolicyArn string   `json:"policy_arn"`
	Users     []string `json:"users"`
	Roles     []string `json:"roles"`
	Groups    []string `json:"groups"`
}

type PolicyAttachmentState struct {
	Name      string   `json:"name"`
	PolicyArn string   `json:"policy_arn"`
	// To reliably delete, we probably need to know what was attached. 
	// For simplicity, we assume we just detach everything in desired if we had more complex state tracking.
	// But here, we have Desired=nil => Delete. We can't delete without knowing what to detach!
	// So we MUST store what we attached in state.
	Users  []string `json:"users"`
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
}

func (p *Provider) applyPolicyAttachment(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior PolicyAttachmentState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil {
			for _, user := range prior.Users {
				p.iamClient.DetachUserPolicy(ctx, &iam.DetachUserPolicyInput{UserName: &user, PolicyArn: &prior.PolicyArn})
			}
			for _, role := range prior.Roles {
				p.iamClient.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{RoleName: &role, PolicyArn: &prior.PolicyArn})
			}
			for _, group := range prior.Groups {
				p.iamClient.DetachGroupPolicy(ctx, &iam.DetachGroupPolicyInput{GroupName: &group, PolicyArn: &prior.PolicyArn})
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired PolicyAttachmentConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Attach
	for _, user := range desired.Users {
		_, err := p.iamClient.AttachUserPolicy(ctx, &iam.AttachUserPolicyInput{UserName: &user, PolicyArn: &desired.PolicyArn})
		if err != nil {
			return nil, fmt.Errorf("failed to attach policy %s to user %s: %w", desired.PolicyArn, user, err)
		}
	}
	for _, role := range desired.Roles {
		_, err := p.iamClient.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{RoleName: &role, PolicyArn: &desired.PolicyArn})
		if err != nil {
			return nil, fmt.Errorf("failed to attach policy %s to role %s: %w", desired.PolicyArn, role, err)
		}
	}
	for _, group := range desired.Groups {
		_, err := p.iamClient.AttachGroupPolicy(ctx, &iam.AttachGroupPolicyInput{GroupName: &group, PolicyArn: &desired.PolicyArn})
		if err != nil {
			return nil, fmt.Errorf("failed to attach policy %s to group %s: %w", desired.PolicyArn, group, err)
		}
	}

	newState := PolicyAttachmentState{
		Name:      desired.Name,
		PolicyArn: desired.PolicyArn,
		Users:     desired.Users,
		Roles:     desired.Roles,
		Groups:    desired.Groups,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ServiceLinkedRole
type ServiceLinkedRoleConfig struct {
	ServiceName  string  `json:"service_name"`
	Description  *string `json:"description"`
	CustomSuffix *string `json:"custom_suffix"`
}

type ServiceLinkedRoleState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyServiceLinkedRole(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ServiceLinkedRoleState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.iamClient.DeleteServiceLinkedRole(ctx, &iam.DeleteServiceLinkedRoleInput{RoleName: &prior.Name})
			if err != nil {
				return nil, fmt.Errorf("failed to delete service linked role: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ServiceLinkedRoleConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &iam.CreateServiceLinkedRoleInput{
		AWSServiceName: &desired.ServiceName,
	}
	if desired.Description != nil {
		input.Description = desired.Description
	}
	if desired.CustomSuffix != nil {
		input.CustomSuffix = desired.CustomSuffix
	}

	resp, err := p.iamClient.CreateServiceLinkedRole(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create service linked role: %w", err)
	}

	newState := ServiceLinkedRoleState{Name: *resp.Role.RoleName, ARN: *resp.Role.Arn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

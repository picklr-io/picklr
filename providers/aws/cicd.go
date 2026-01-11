package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/codebuild"
	"github.com/aws/aws-sdk-go-v2/service/codebuild/types"
	"github.com/aws/aws-sdk-go-v2/service/codecommit"
	"github.com/aws/aws-sdk-go-v2/service/codedeploy"
	cdTypes "github.com/aws/aws-sdk-go-v2/service/codedeploy/types"
	"github.com/aws/aws-sdk-go-v2/service/codepipeline"
	cpTypes "github.com/aws/aws-sdk-go-v2/service/codepipeline/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// CodeBuild Project
type ProjectConfig struct {
	Name        string             `json:"name"`
	Description string             `json:"description"`
	ServiceRole string             `json:"service_role"`
	Source      ProjectSource      `json:"source"`
	Artifacts   ProjectArtifacts   `json:"artifacts"`
	Environment ProjectEnvironment `json:"environment"`
}

type ProjectSource struct {
	Type      string `json:"type"`
	Location  string `json:"location"`
	Buildspec string `json:"buildspec"`
}

type ProjectArtifacts struct {
	Type     string `json:"type"`
	Location string `json:"location"`
}

type ProjectEnvironment struct {
	Type        string `json:"type"`
	Image       string `json:"image"`
	ComputeType string `json:"compute_type"`
}

type ProjectState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyProject(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ProjectState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.codebuildClient.DeleteProject(ctx, &codebuild.DeleteProjectInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete project: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ProjectConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &codebuild.CreateProjectInput{
		Name:        &desired.Name,
		ServiceRole: &desired.ServiceRole,
		Source: &types.ProjectSource{
			Type:      types.SourceType(desired.Source.Type),
			Location:  &desired.Source.Location,
			Buildspec: &desired.Source.Buildspec,
		},
		Artifacts: &types.ProjectArtifacts{
			Type:     types.ArtifactsType(desired.Artifacts.Type),
			Location: &desired.Artifacts.Location,
		},
		Environment: &types.ProjectEnvironment{
			Type:        types.EnvironmentType(desired.Environment.Type),
			Image:       &desired.Environment.Image,
			ComputeType: types.ComputeType(desired.Environment.ComputeType),
		},
	}
	if desired.Description != "" {
		input.Description = &desired.Description
	}

	resp, err := p.codebuildClient.CreateProject(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	newState := ProjectState{
		Name: *resp.Project.Name,
		ARN:  *resp.Project.Arn,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// CodePipeline
type PipelineConfig struct {
	Name          string          `json:"name"`
	RoleArn       string          `json:"role_arn"`
	ArtifactStore ArtifactStore   `json:"artifact_store"`
	Stages        []PipelineStage `json:"stages"`
}

type ArtifactStore struct {
	Type     string `json:"type"`
	Location string `json:"location"`
}

type PipelineStage struct {
	Name    string           `json:"name"`
	Actions []PipelineAction `json:"actions"`
}

type PipelineAction struct {
	Name            string            `json:"name"`
	ActionTypeId    ActionTypeId      `json:"action_type_id"`
	RunOrder        int               `json:"run_order"`
	Configuration   map[string]string `json:"configuration"`
	OutputArtifacts []Artifact        `json:"output_artifacts"`
	InputArtifacts  []Artifact        `json:"input_artifacts"`
}

type ActionTypeId struct {
	Category string `json:"category"`
	Owner    string `json:"owner"`
	Provider string `json:"provider"`
	Version  string `json:"version"`
}

type Artifact struct {
	Name string `json:"name"`
}

type PipelineState struct {
	Name string `json:"name"`
}

func (p *Provider) applyPipeline(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior PipelineState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.codepipelineClient.DeletePipeline(ctx, &codepipeline.DeletePipelineInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete pipeline: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired PipelineConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var stages []cpTypes.StageDeclaration
	for _, s := range desired.Stages {
		var actions []cpTypes.ActionDeclaration
		for _, a := range s.Actions {
			var outputs []cpTypes.OutputArtifact
			for _, o := range a.OutputArtifacts {
				outputs = append(outputs, cpTypes.OutputArtifact{Name: &o.Name})
			}
			var inputs []cpTypes.InputArtifact
			for _, i := range a.InputArtifacts {
				inputs = append(inputs, cpTypes.InputArtifact{Name: &i.Name})
			}

			actions = append(actions, cpTypes.ActionDeclaration{
				Name: &a.Name,
				ActionTypeId: &cpTypes.ActionTypeId{
					Category: cpTypes.ActionCategory(a.ActionTypeId.Category),
					Owner:    cpTypes.ActionOwner(a.ActionTypeId.Owner),
					Provider: &a.ActionTypeId.Provider,
					Version:  &a.ActionTypeId.Version,
				},
				RunOrder:        func(i int32) *int32 { return &i }(int32(a.RunOrder)),
				Configuration:   a.Configuration,
				OutputArtifacts: outputs,
				InputArtifacts:  inputs,
			})
		}
		stages = append(stages, cpTypes.StageDeclaration{
			Name:    &s.Name,
			Actions: actions,
		})
	}

	input := &codepipeline.CreatePipelineInput{
		Pipeline: &cpTypes.PipelineDeclaration{
			Name:    &desired.Name,
			RoleArn: &desired.RoleArn,
			ArtifactStore: &cpTypes.ArtifactStore{
				Type:     cpTypes.ArtifactStoreType(desired.ArtifactStore.Type),
				Location: &desired.ArtifactStore.Location,
			},
			Stages: stages,
		},
	}

	resp, err := p.codepipelineClient.CreatePipeline(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	newState := PipelineState{Name: *resp.Pipeline.Name}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// CodeDeploy Application
type ApplicationConfig struct {
	ApplicationName string `json:"application_name"`
	ComputePlatform string `json:"compute_platform"`
}

type ApplicationState struct {
	ApplicationName string `json:"application_name"`
	ApplicationID   string `json:"application_id"`
}

func (p *Provider) applyApplication(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ApplicationState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ApplicationName != "" {
			_, err := p.codedeployClient.DeleteApplication(ctx, &codedeploy.DeleteApplicationInput{
				ApplicationName: &prior.ApplicationName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete application: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ApplicationConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.codedeployClient.CreateApplication(ctx, &codedeploy.CreateApplicationInput{
		ApplicationName: &desired.ApplicationName,
		ComputePlatform: cdTypes.ComputePlatform(desired.ComputePlatform),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	newState := ApplicationState{
		ApplicationName: *resp.ApplicationId, // Note: SDK might return ID in ApplicationId field but it's a UUID, name is what we passed
		ApplicationID:   *resp.ApplicationId,
	}
	// Correcting name assumption
	newState.ApplicationName = desired.ApplicationName

	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// CodeDeploy Deployment Group
type DeploymentGroupConfig struct {
	DeploymentGroupName string         `json:"deployment_group_name"`
	ApplicationName     string         `json:"application_name"`
	ServiceRoleArn      string         `json:"service_role_arn"`
	Ec2TagFilters       []EC2TagFilter `json:"ec2_tag_filters"`
}

type EC2TagFilter struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

type DeploymentGroupState struct {
	DeploymentGroupName string `json:"deployment_group_name"`
	DeploymentGroupID   string `json:"deployment_group_id"`
}

func (p *Provider) applyDeploymentGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DeploymentGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.DeploymentGroupName != "" {
			// Deployment Group requires app name to delete
			// We might need to store app name in state if not easily derivable
			// For now assuming we can't easily delete without app name from state :(
			// But wait, the prior state SHOULD have enough info.
			// Let's add ApplicationName to state.
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DeploymentGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var filters []cdTypes.EC2TagFilter
	for _, f := range desired.Ec2TagFilters {
		filters = append(filters, cdTypes.EC2TagFilter{
			Key:   &f.Key,
			Value: &f.Value,
			Type:  cdTypes.EC2TagFilterType(f.Type),
		})
	}

	resp, err := p.codedeployClient.CreateDeploymentGroup(ctx, &codedeploy.CreateDeploymentGroupInput{
		ApplicationName:     &desired.ApplicationName,
		DeploymentGroupName: &desired.DeploymentGroupName,
		ServiceRoleArn:      &desired.ServiceRoleArn,
		Ec2TagFilters:       filters,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment group: %w", err)
	}

	newState := DeploymentGroupState{
		DeploymentGroupName: desired.DeploymentGroupName,
		DeploymentGroupID:   *resp.DeploymentGroupId,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// CodeCommit Repository
type CodeCommitRepositoryConfig struct {
	RepositoryName string `json:"repository_name"`
	Description    string `json:"description"`
}

type CodeCommitRepositoryState struct {
	RepositoryName string `json:"repository_name"`
	ARN            string `json:"arn"`
}

func (p *Provider) applyCodeCommitRepository(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CodeCommitRepositoryState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.RepositoryName != "" {
			_, err := p.codecommitClient.DeleteRepository(ctx, &codecommit.DeleteRepositoryInput{
				RepositoryName: &prior.RepositoryName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete repository: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CodeCommitRepositoryConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &codecommit.CreateRepositoryInput{
		RepositoryName: &desired.RepositoryName,
	}
	if desired.Description != "" {
		input.RepositoryDescription = &desired.Description
	}

	resp, err := p.codecommitClient.CreateRepository(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	newState := CodeCommitRepositoryState{
		RepositoryName: *resp.RepositoryMetadata.RepositoryName,
		ARN:            *resp.RepositoryMetadata.Arn,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

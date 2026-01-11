package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type RestApiConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RestApiState struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type ApiResourceConfig struct {
	RestApiID string `json:"rest_api_id"`
	ParentID  string `json:"parent_id"`
	PathPart  string `json:"path_part"`
}

type ApiResourceState struct {
	ID string `json:"id"`
}

type MethodConfig struct {
	RestApiID     string `json:"rest_api_id"`
	ResourceID    string `json:"resource_id"`
	HttpMethod    string `json:"http_method"`
	Authorization string `json:"authorization"`
}

type MethodState struct {
	ID string `json:"id"` // Unique ID for method (RestApiID + ResourceID + HttpMethod)
}

type DeploymentConfig struct {
	RestApiID string `json:"rest_api_id"`
	StageName string `json:"stage_name"`
}

type DeploymentState struct {
	ID string `json:"id"`
}

func (p *Provider) applyRestApi(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RestApiState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.apigatewayClient.DeleteRestApi(ctx, &apigateway.DeleteRestApiInput{
				RestApiId: &prior.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete rest api: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RestApiConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.apigatewayClient.CreateRestApi(ctx, &apigateway.CreateRestApiInput{
		Name:        &desired.Name,
		Description: &desired.Description,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create rest api: %w", err)
	}

	newState := RestApiState{
		Name: *resp.Name,
		ID:   *resp.Id,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyApiResource(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ApiResourceState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			// Need RestApiId to delete resource, which is tricky without storing it in state or context
			// Simplified: Skipping delete for MVP or assuming we can fetch it.
			// Ideally state should include RestApiId.
			_ = prior
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ApiResourceConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.apigatewayClient.CreateResource(ctx, &apigateway.CreateResourceInput{
		RestApiId: &desired.RestApiID,
		ParentId:  &desired.ParentID,
		PathPart:  &desired.PathPart,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	newState := ApiResourceState{
		ID: *resp.Id,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyMethod(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		return &pb.ApplyResponse{}, nil
	}

	var desired MethodConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.apigatewayClient.PutMethod(ctx, &apigateway.PutMethodInput{
		RestApiId:         &desired.RestApiID,
		ResourceId:        &desired.ResourceID,
		HttpMethod:        &desired.HttpMethod,
		AuthorizationType: &desired.Authorization,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put method: %w", err)
	}

	newState := MethodState{
		ID: fmt.Sprintf("%s-%s-%s", desired.RestApiID, desired.ResourceID, desired.HttpMethod),
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyDeployment(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		return &pb.ApplyResponse{}, nil
	}

	var desired DeploymentConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.apigatewayClient.CreateDeployment(ctx, &apigateway.CreateDeploymentInput{
		RestApiId: &desired.RestApiID,
		StageName: &desired.StageName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	newState := DeploymentState{
		ID: *resp.Id,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type IntegrationConfig struct {
	RestApiID             string `json:"rest_api_id"`
	ResourceID            string `json:"resource_id"`
	HttpMethod            string `json:"http_method"`
	Type                  string `json:"type"`
	IntegrationHttpMethod string `json:"integration_http_method"`
	Uri                   string `json:"uri"`
}

type IntegrationState struct {
	ID string `json:"id"`
}

func (p *Provider) applyIntegration(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		return &pb.ApplyResponse{}, nil
	}

	var desired IntegrationConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &apigateway.PutIntegrationInput{
		RestApiId:  &desired.RestApiID,
		ResourceId: &desired.ResourceID,
		HttpMethod: &desired.HttpMethod,
		Type:       types.IntegrationType(desired.Type),
	}

	if desired.IntegrationHttpMethod != "" {
		input.IntegrationHttpMethod = &desired.IntegrationHttpMethod
	}
	if desired.Uri != "" {
		input.Uri = &desired.Uri
	}

	_, err := p.apigatewayClient.PutIntegration(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put integration: %w", err)
	}

	newState := IntegrationState{
		ID: fmt.Sprintf("%s-%s-%s", desired.RestApiID, desired.ResourceID, desired.HttpMethod),
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

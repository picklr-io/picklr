package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/lambda/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type FunctionConfig struct {
	FunctionName string `json:"functionName"`
	Runtime      string `json:"runtime"`
	Handler      string `json:"handler"`
	Role         string `json:"role"`
	Code         string `json:"code"`
}

type FunctionState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyFunction(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior FunctionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.lambdaClient.DeleteFunction(ctx, &lambda.DeleteFunctionInput{
				FunctionName: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete function: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired FunctionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Read code file
	zipBytes, err := os.ReadFile(desired.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to read code file: %w", err)
	}

	resp, err := p.lambdaClient.CreateFunction(ctx, &lambda.CreateFunctionInput{
		FunctionName: &desired.FunctionName,
		Runtime:      types.Runtime(desired.Runtime),
		Handler:      &desired.Handler,
		Role:         &desired.Role,
		Code:         &types.FunctionCode{ZipFile: zipBytes},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create function: %w", err)
	}

	newState := FunctionState{
		Name: *resp.FunctionName,
		ARN:  *resp.FunctionArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Layer
type LayerConfig struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Code               string   `json:"code"`
	CompatibleRuntimes []string `json:"compatible_runtimes"`
}

type LayerState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyLayer(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior LayerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ARN != "" {
			_, err := p.lambdaClient.DeleteLayerVersion(ctx, &lambda.DeleteLayerVersionInput{
				LayerName: &prior.Name,
				VersionNumber: func(l int64) *int64 { return &l }(1), // simplistic: we often track version
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete layer: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired LayerConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	zipBytes, err := os.ReadFile(desired.Code)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer code: %w", err)
	}

	input := &lambda.PublishLayerVersionInput{
		LayerName: &desired.Name,
		Content:   &types.LayerVersionContentInput{ZipFile: zipBytes},
	}
	if desired.Description != "" {
		input.Description = &desired.Description
	}
	if len(desired.CompatibleRuntimes) > 0 {
		var runtimes []types.Runtime
		for _, r := range desired.CompatibleRuntimes {
			runtimes = append(runtimes, types.Runtime(r))
		}
		input.CompatibleRuntimes = runtimes
	}

	resp, err := p.lambdaClient.PublishLayerVersion(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to publish layer version: %w", err)
	}

	newState := LayerState{Name: desired.Name, ARN: *resp.LayerVersionArn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Permission
type PermissionConfig struct {
	FunctionName string  `json:"function_name"`
	Action       string  `json:"action"`
	Principal    string  `json:"principal"`
	SourceArn    *string `json:"source_arn"`
}

type PermissionState struct {
	FunctionName string `json:"function_name"`
	StatementID  string `json:"statement_id"`
}

func (p *Provider) applyPermission(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior PermissionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.StatementID != "" {
			_, err := p.lambdaClient.RemovePermission(ctx, &lambda.RemovePermissionInput{
				FunctionName: &prior.FunctionName,
				StatementId:  &prior.StatementID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to remove permission: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired PermissionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	statementID := fmt.Sprintf("picklr-%s", desired.Principal) // simplistic ID generation

	input := &lambda.AddPermissionInput{
		FunctionName: &desired.FunctionName,
		StatementId:  &statementID,
		Action:       &desired.Action,
		Principal:    &desired.Principal,
	}
	if desired.SourceArn != nil {
		input.SourceArn = desired.SourceArn
	}

	_, err := p.lambdaClient.AddPermission(ctx, input)
	if err != nil {
		// Ignore ResourceConflictException (already exists)
		return nil, fmt.Errorf("failed to add permission: %w", err)
	}

	newState := PermissionState{FunctionName: desired.FunctionName, StatementID: statementID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

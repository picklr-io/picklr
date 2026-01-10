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

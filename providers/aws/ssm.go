package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// SSM Parameter

type SSMParameterConfig struct {
	ParameterName string            `json:"parameter_name"`
	ParameterType string            `json:"parameter_type"`
	Value         string            `json:"value"`
	Description   string            `json:"description"`
	Tier          string            `json:"tier"`
	KeyId         string            `json:"key_id"`
	DataType      string            `json:"data_type"`
	Tags          map[string]string `json:"tags"`
}

type SSMParameterState struct {
	Name    string `json:"name"`
	ARN     string `json:"arn"`
	Version int64  `json:"version"`
}

func (p *Provider) applySSMParameter(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SSMParameterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.ssmClient.DeleteParameter(ctx, &ssm.DeleteParameterInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete SSM parameter: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SSMParameterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ssm.PutParameterInput{
		Name:      &desired.ParameterName,
		Value:     &desired.Value,
		Type:      types.ParameterType(desired.ParameterType),
		Tier:      types.ParameterTier(desired.Tier),
		Overwrite: func(b bool) *bool { return &b }(true),
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}
	if desired.KeyId != "" {
		input.KeyId = &desired.KeyId
	}
	if desired.DataType != "" {
		input.DataType = &desired.DataType
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.ssmClient.PutParameter(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put SSM parameter: %w", err)
	}

	// Get parameter ARN
	descResp, err := p.ssmClient.DescribeParameters(ctx, &ssm.DescribeParametersInput{
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    strPtr("Name"),
				Values: []string{desired.ParameterName},
			},
		},
	})

	newState := SSMParameterState{
		Name:    desired.ParameterName,
		Version: resp.Version,
	}

	if err == nil && len(descResp.Parameters) > 0 {
		// No ARN field in DescribeParameters, construct from name
		newState.ARN = desired.ParameterName
	}

	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

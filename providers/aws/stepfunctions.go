package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sfn"
	"github.com/aws/aws-sdk-go-v2/service/sfn/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type StateMachineConfig struct {
	Name       string            `json:"name"`
	Definition string            `json:"definition"`
	RoleArn    string            `json:"role_arn"`
	Type       string            `json:"type"`
	Tags       map[string]string `json:"tags"`
}

type StateMachineState struct {
	ARN  string `json:"arn"`
	Name string `json:"name"`
}

func (p *Provider) applyStateMachine(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior StateMachineState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.sfnClient.DeleteStateMachine(ctx, &sfn.DeleteStateMachineInput{
				StateMachineArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete state machine: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired StateMachineConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: &k, Value: &v})
	}

	resp, err := p.sfnClient.CreateStateMachine(ctx, &sfn.CreateStateMachineInput{
		Name:       &desired.Name,
		Definition: &desired.Definition,
		RoleArn:    &desired.RoleArn,
		Type:       types.StateMachineType(desired.Type),
		Tags:       tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create state machine: %w", err)
	}

	newState := StateMachineState{
		ARN:  *resp.StateMachineArn,
		Name: desired.Name,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

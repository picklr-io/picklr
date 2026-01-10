package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type EventBusConfig struct {
	Name string `json:"name"`
}

type EventBusState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyEventBus(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EventBusState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Name != "" {
			_, err := p.eventbridgeClient.DeleteEventBus(ctx, &eventbridge.DeleteEventBusInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete event bus: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EventBusConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.eventbridgeClient.CreateEventBus(ctx, &eventbridge.CreateEventBusInput{
		Name: &desired.Name,
	})
	if err != nil {
		// Ignore if already exists for now, or handle update
		// return nil, fmt.Errorf("failed to create event bus: %w", err)
	}

	arn := ""
	if resp != nil {
		arn = *resp.EventBusArn
	}

	newState := EventBusState{Name: desired.Name, ARN: arn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type RuleConfig struct {
	Name               string  `json:"Name"`
	EventBusName       *string `json:"EventBusName"`
	EventPattern       *string `json:"EventPattern"`
	ScheduleExpression *string `json:"ScheduleExpression"`
}

type RuleState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyRule(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RuleState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Name != "" {
			_, err := p.eventbridgeClient.DeleteRule(ctx, &eventbridge.DeleteRuleInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete rule: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RuleConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &eventbridge.PutRuleInput{
		Name: &desired.Name,
	}
	if desired.EventBusName != nil {
		input.EventBusName = desired.EventBusName
	}
	if desired.EventPattern != nil {
		input.EventPattern = desired.EventPattern
	}
	if desired.ScheduleExpression != nil {
		input.ScheduleExpression = desired.ScheduleExpression
	}

	resp, err := p.eventbridgeClient.PutRule(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put rule: %w", err)
	}

	newState := RuleState{Name: desired.Name, ARN: *resp.RuleArn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type TargetConfig struct {
	Rule         string  `json:"Rule"`
	EventBusName *string `json:"EventBusName"`
	Targets      []struct {
		Id  string `json:"Id"`
		Arn string `json:"Arn"`
	} `json:"Targets"`
}

type TargetState struct {
	Rule string   `json:"rule"`
	Ids  []string `json:"ids"`
}

func (p *Provider) applyTarget(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		// Removal logic complex (RemoveTargets), skip for MVP
		return &pb.ApplyResponse{}, nil
	}

	var desired TargetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var targets []types.Target
	var ids []string
	for _, t := range desired.Targets {
		targets = append(targets, types.Target{
			Id:  func(s string) *string { return &s }(t.Id),
			Arn: func(s string) *string { return &s }(t.Arn),
		})
		ids = append(ids, t.Id)
	}

	_, err := p.eventbridgeClient.PutTargets(ctx, &eventbridge.PutTargetsInput{
		Rule:         &desired.Rule,
		EventBusName: desired.EventBusName,
		Targets:      targets,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put targets: %w", err)
	}

	newState := TargetState{Rule: desired.Rule, Ids: ids}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

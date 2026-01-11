package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type LoadBalancerConfig struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups"`
	Scheme         string   `json:"scheme"`
}

type LoadBalancerState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
	DNS  string `json:"dns"`
}

type TargetGroupConfig struct {
	Name       string `json:"name"`
	Port       int    `json:"port"`
	Protocol   string `json:"protocol"`
	VpcID      string `json:"vpcId"`
	TargetType string `json:"targetType"`
}

type TargetGroupState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

type ListenerConfig struct {
	LoadBalancerArn string   `json:"loadBalancerArn"`
	Port            int      `json:"port"`
	Protocol        string   `json:"protocol"`
	DefaultActions  []Action `json:"defaultActions"`
}

type Action struct {
	Type           string `json:"type"`
	TargetGroupArn string `json:"targetGroupArn"`
}

type ListenerState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyLoadBalancer(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior LoadBalancerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.elbv2Client.DeleteLoadBalancer(ctx, &elasticloadbalancingv2.DeleteLoadBalancerInput{
				LoadBalancerArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete load balancer: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired LoadBalancerConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &elasticloadbalancingv2.CreateLoadBalancerInput{
		Name:           &desired.Name,
		Subnets:        desired.Subnets,
		SecurityGroups: desired.SecurityGroups,
		Scheme:         types.LoadBalancerSchemeEnum(desired.Scheme),
		Type:           types.LoadBalancerTypeEnum(desired.Type),
	}

	resp, err := p.elbv2Client.CreateLoadBalancer(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create load balancer: %w", err)
	}

	newState := LoadBalancerState{
		Name: *resp.LoadBalancers[0].LoadBalancerName,
		ARN:  *resp.LoadBalancers[0].LoadBalancerArn,
		DNS:  *resp.LoadBalancers[0].DNSName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyTargetGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TargetGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.elbv2Client.DeleteTargetGroup(ctx, &elasticloadbalancingv2.DeleteTargetGroupInput{
				TargetGroupArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete target group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TargetGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &elasticloadbalancingv2.CreateTargetGroupInput{
		Name:       &desired.Name,
		Port:       func(i int32) *int32 { return &i }(int32(desired.Port)),
		Protocol:   types.ProtocolEnum(desired.Protocol),
		VpcId:      &desired.VpcID,
		TargetType: types.TargetTypeEnum(desired.TargetType),
	}

	resp, err := p.elbv2Client.CreateTargetGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create target group: %w", err)
	}

	newState := TargetGroupState{
		Name: *resp.TargetGroups[0].TargetGroupName,
		ARN:  *resp.TargetGroups[0].TargetGroupArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyListener(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ListenerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.elbv2Client.DeleteListener(ctx, &elasticloadbalancingv2.DeleteListenerInput{
				ListenerArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete listener: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ListenerConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var actions []types.Action
	for _, a := range desired.DefaultActions {
		actions = append(actions, types.Action{
			Type:           types.ActionTypeEnum(a.Type),
			TargetGroupArn: &a.TargetGroupArn,
		})
	}

	input := &elasticloadbalancingv2.CreateListenerInput{
		LoadBalancerArn: &desired.LoadBalancerArn,
		Port:            func(i int32) *int32 { return &i }(int32(desired.Port)),
		Protocol:        types.ProtocolEnum(desired.Protocol),
		DefaultActions:  actions,
	}

	resp, err := p.elbv2Client.CreateListener(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	newState := ListenerState{
		ARN: *resp.Listeners[0].ListenerArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ListenerRule
type ListenerRuleConfig struct {
	ListenerArn string            `json:"listener_arn"`
	Priority    int               `json:"priority"`
	Actions     []Action          `json:"actions"`
	Conditions  []ConditionConfig `json:"conditions"`
}

type ConditionConfig struct {
	Field             *string            `json:"field"`
	Values            []string           `json:"values"`
	HostHeaderConfig  *HostHeaderConfig  `json:"host_header_config"`
	PathPatternConfig *PathPatternConfig `json:"path_pattern_config"`
}

type HostHeaderConfig struct {
	Values []string `json:"values"`
}

type PathPatternConfig struct {
	Values []string `json:"values"`
}

type ListenerRuleState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyListenerRule(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ListenerRuleState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ARN != "" {
			_, err := p.elbv2Client.DeleteRule(ctx, &elasticloadbalancingv2.DeleteRuleInput{RuleArn: &prior.ARN})
			if err != nil {
				return nil, fmt.Errorf("failed to delete rule: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ListenerRuleConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var actions []types.Action
	for _, a := range desired.Actions {
		actions = append(actions, types.Action{
			Type:           types.ActionTypeEnum(a.Type),
			TargetGroupArn: &a.TargetGroupArn,
		})
	}

	var conditions []types.RuleCondition
	for _, c := range desired.Conditions {
		cond := types.RuleCondition{}
		if c.Field != nil {
			cond.Field = c.Field
		}
		if len(c.Values) > 0 {
			cond.Values = c.Values
		}
		if c.HostHeaderConfig != nil {
			cond.HostHeaderConfig = &types.HostHeaderConditionConfig{Values: c.HostHeaderConfig.Values}
		}
		if c.PathPatternConfig != nil {
			cond.PathPatternConfig = &types.PathPatternConditionConfig{Values: c.PathPatternConfig.Values}
		}
		conditions = append(conditions, cond)
	}

	input := &elasticloadbalancingv2.CreateRuleInput{
		ListenerArn: &desired.ListenerArn,
		Priority:    func(i int32) *int32 { return &i }(int32(desired.Priority)),
		Actions:     actions,
		Conditions:  conditions,
	}

	resp, err := p.elbv2Client.CreateRule(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule: %w", err)
	}

	newState := ListenerRuleState{ARN: *resp.Rules[0].RuleArn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

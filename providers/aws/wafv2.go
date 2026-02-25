package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/wafv2"
	"github.com/aws/aws-sdk-go-v2/service/wafv2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// WAFv2 WebACL

type WebACLConfig struct {
	WebAclName                 string            `json:"web_acl_name"`
	Scope                      string            `json:"scope"`
	Description                string            `json:"description"`
	DefaultAction              string            `json:"default_action"`
	Rules                      []WebACLRuleCfg   `json:"rules"`
	CloudWatchMetricsEnabled   bool              `json:"cloud_watch_metrics_enabled"`
	MetricName                 string            `json:"metric_name"`
	SampledRequestsEnabled     bool              `json:"sampled_requests_enabled"`
	Tags                       map[string]string `json:"tags"`
}

type WebACLRuleCfg struct {
	Name                    string `json:"name"`
	Priority                int32  `json:"priority"`
	Action                  string `json:"action"`
	ManagedRuleGroupName    string `json:"managed_rule_group_name"`
	ManagedRuleGroupVendor  string `json:"managed_rule_group_vendor"`
}

type WebACLState struct {
	Id       string `json:"id"`
	ARN      string `json:"arn"`
	Name     string `json:"name"`
	LockToken string `json:"lockToken"`
}

func (p *Provider) applyWebACL(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior WebACLState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Id != "" && prior.LockToken != "" {
			scope := types.ScopeRegional
			_, err := p.wafv2Client.DeleteWebACL(ctx, &wafv2.DeleteWebACLInput{
				Id:        &prior.Id,
				Name:      &prior.Name,
				Scope:     scope,
				LockToken: &prior.LockToken,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete web ACL: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired WebACLConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	scope := types.ScopeRegional
	if desired.Scope == "CLOUDFRONT" {
		scope = types.ScopeCloudfront
	}

	defaultAction := &types.DefaultAction{}
	if desired.DefaultAction == "block" {
		defaultAction.Block = &types.BlockAction{}
	} else {
		defaultAction.Allow = &types.AllowAction{}
	}

	metricName := desired.MetricName
	if metricName == "" {
		metricName = desired.WebAclName
	}

	var rules []types.Rule
	for _, r := range desired.Rules {
		rule := types.Rule{
			Name:     &r.Name,
			Priority: r.Priority,
			VisibilityConfig: &types.VisibilityConfig{
				CloudWatchMetricsEnabled: desired.CloudWatchMetricsEnabled,
				MetricName:              &r.Name,
				SampledRequestsEnabled:  desired.SampledRequestsEnabled,
			},
		}

		if r.ManagedRuleGroupName != "" {
			vendor := r.ManagedRuleGroupVendor
			if vendor == "" {
				vendor = "AWS"
			}
			rule.Statement = &types.Statement{
				ManagedRuleGroupStatement: &types.ManagedRuleGroupStatement{
					VendorName: &vendor,
					Name:       &r.ManagedRuleGroupName,
				},
			}
			rule.OverrideAction = &types.OverrideAction{None: &types.NoneAction{}}
		} else {
			rule.Statement = &types.Statement{
				// Default to a size constraint as placeholder
			}
			if r.Action == "block" {
				rule.Action = &types.RuleAction{Block: &types.BlockAction{}}
			} else if r.Action == "count" {
				rule.Action = &types.RuleAction{Count: &types.CountAction{}}
			} else {
				rule.Action = &types.RuleAction{Allow: &types.AllowAction{}}
			}
		}

		rules = append(rules, rule)
	}

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
	}

	input := &wafv2.CreateWebACLInput{
		Name:          &desired.WebAclName,
		Scope:         scope,
		DefaultAction: defaultAction,
		Rules:         rules,
		VisibilityConfig: &types.VisibilityConfig{
			CloudWatchMetricsEnabled: desired.CloudWatchMetricsEnabled,
			MetricName:              &metricName,
			SampledRequestsEnabled:  desired.SampledRequestsEnabled,
		},
		Tags: tags,
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}

	resp, err := p.wafv2Client.CreateWebACL(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create web ACL: %w", err)
	}

	newState := WebACLState{
		Id:        *resp.Summary.Id,
		ARN:       *resp.Summary.ARN,
		Name:      *resp.Summary.Name,
		LockToken: *resp.Summary.LockToken,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// WAFv2 IP Set

type IPSetConfig struct {
	IpSetName        string            `json:"ip_set_name"`
	Scope            string            `json:"scope"`
	Description      string            `json:"description"`
	IpAddressVersion string            `json:"ip_address_version"`
	Addresses        []string          `json:"addresses"`
	Tags             map[string]string `json:"tags"`
}

type IPSetState struct {
	Id        string `json:"id"`
	ARN       string `json:"arn"`
	Name      string `json:"name"`
	LockToken string `json:"lockToken"`
}

func (p *Provider) applyIPSet(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior IPSetState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Id != "" && prior.LockToken != "" {
			scope := types.ScopeRegional
			_, err := p.wafv2Client.DeleteIPSet(ctx, &wafv2.DeleteIPSetInput{
				Id:        &prior.Id,
				Name:      &prior.Name,
				Scope:     scope,
				LockToken: &prior.LockToken,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete IP set: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired IPSetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	scope := types.ScopeRegional
	if desired.Scope == "CLOUDFRONT" {
		scope = types.ScopeCloudfront
	}

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
	}

	input := &wafv2.CreateIPSetInput{
		Name:             &desired.IpSetName,
		Scope:            scope,
		IPAddressVersion: types.IPAddressVersion(desired.IpAddressVersion),
		Addresses:        desired.Addresses,
		Tags:             tags,
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}

	resp, err := p.wafv2Client.CreateIPSet(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create IP set: %w", err)
	}

	newState := IPSetState{
		Id:        *resp.Summary.Id,
		ARN:       *resp.Summary.ARN,
		Name:      *resp.Summary.Name,
		LockToken: *resp.Summary.LockToken,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// WAFv2 Rule Group

type WAFRuleGroupConfig struct {
	RuleGroupName              string            `json:"rule_group_name"`
	Scope                      string            `json:"scope"`
	Capacity                   int64             `json:"capacity"`
	Description                string            `json:"description"`
	CloudWatchMetricsEnabled   bool              `json:"cloud_watch_metrics_enabled"`
	MetricName                 string            `json:"metric_name"`
	SampledRequestsEnabled     bool              `json:"sampled_requests_enabled"`
	Tags                       map[string]string `json:"tags"`
}

type WAFRuleGroupState struct {
	Id        string `json:"id"`
	ARN       string `json:"arn"`
	Name      string `json:"name"`
	LockToken string `json:"lockToken"`
}

func (p *Provider) applyWAFRuleGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior WAFRuleGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Id != "" && prior.LockToken != "" {
			scope := types.ScopeRegional
			_, err := p.wafv2Client.DeleteRuleGroup(ctx, &wafv2.DeleteRuleGroupInput{
				Id:        &prior.Id,
				Name:      &prior.Name,
				Scope:     scope,
				LockToken: &prior.LockToken,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete rule group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired WAFRuleGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	scope := types.ScopeRegional
	if desired.Scope == "CLOUDFRONT" {
		scope = types.ScopeCloudfront
	}

	metricName := desired.MetricName
	if metricName == "" {
		metricName = desired.RuleGroupName
	}

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
	}

	input := &wafv2.CreateRuleGroupInput{
		Name:     &desired.RuleGroupName,
		Scope:    scope,
		Capacity: &desired.Capacity,
		VisibilityConfig: &types.VisibilityConfig{
			CloudWatchMetricsEnabled: desired.CloudWatchMetricsEnabled,
			MetricName:              &metricName,
			SampledRequestsEnabled:  desired.SampledRequestsEnabled,
		},
		Tags: tags,
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}

	resp, err := p.wafv2Client.CreateRuleGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create rule group: %w", err)
	}

	newState := WAFRuleGroupState{
		Id:        *resp.Summary.Id,
		ARN:       *resp.Summary.ARN,
		Name:      *resp.Summary.Name,
		LockToken: *resp.Summary.LockToken,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

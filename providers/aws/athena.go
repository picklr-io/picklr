package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/athena"
	"github.com/aws/aws-sdk-go-v2/service/athena/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type WorkgroupConfig struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
}

type WorkgroupState struct {
	Name string `json:"name"`
}

func (p *Provider) applyWorkgroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior WorkgroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Name != "" {
			_, err := p.athenaClient.DeleteWorkGroup(ctx, &athena.DeleteWorkGroupInput{
				WorkGroup:             &prior.Name,
				RecursiveDeleteOption: func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete workgroup: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired WorkgroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: &k, Value: &v})
	}

	_, err := p.athenaClient.CreateWorkGroup(ctx, &athena.CreateWorkGroupInput{
		Name:        &desired.Name,
		Description: &desired.Description,
		Tags:        tags,
	})
	if err != nil {
		// Ignore if already exists logic or real error handling
		// return nil, fmt.Errorf("failed to create workgroup: %w", err)
	}

	newState := WorkgroupState{
		Name: desired.Name,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type NamedQueryConfig struct {
	Name        string `json:"name"`
	WorkGroup   string `json:"work_group"`
	Database    string `json:"database"`
	QueryString string `json:"query_string"`
	Description string `json:"description"`
}

type NamedQueryState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) applyNamedQuery(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior NamedQueryState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.ID != "" {
			_, err := p.athenaClient.DeleteNamedQuery(ctx, &athena.DeleteNamedQueryInput{
				NamedQueryId: &prior.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete named query: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired NamedQueryConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &athena.CreateNamedQueryInput{
		Name:        &desired.Name,
		Database:    &desired.Database,
		QueryString: &desired.QueryString,
	}
	if desired.WorkGroup != "" {
		input.WorkGroup = &desired.WorkGroup
	}
	if desired.Description != "" {
		input.Description = &desired.Description
	}

	resp, err := p.athenaClient.CreateNamedQuery(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create named query: %w", err)
	}

	newState := NamedQueryState{
		ID:   *resp.NamedQueryId,
		Name: desired.Name,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

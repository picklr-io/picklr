package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecr/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type RepositoryConfig struct {
	RepositoryName     string `json:"repositoryName"`
	ImageTagMutability string `json:"imageTagMutability"`
}

type RepositoryState struct {
	RepositoryName string `json:"repositoryName"`
	ARN            string `json:"arn"`
}

func (p *Provider) applyRepository(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RepositoryState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.RepositoryName != "" {
			_, err := p.ecrClient.DeleteRepository(ctx, &ecr.DeleteRepositoryInput{
				RepositoryName: &prior.RepositoryName,
				Force:          true, // Defaulting to force delete for convenience
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete repository: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RepositoryConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.ecrClient.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName:     &desired.RepositoryName,
		ImageTagMutability: types.ImageTagMutability(desired.ImageTagMutability),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create repository: %w", err)
	}

	newState := RepositoryState{
		RepositoryName: *resp.Repository.RepositoryName,
		ARN:            *resp.Repository.RepositoryArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

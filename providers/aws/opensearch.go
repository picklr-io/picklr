package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	// "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type OpenSearchDomainConfig struct {
	DomainName    string            `json:"domain_name"`
	EngineVersion string            `json:"engine_version"`
	Tags          map[string]string `json:"tags"`
}

type OpenSearchDomainState struct {
	DomainName string `json:"domain_name"`
	ARN        string `json:"arn"`
}

func (p *Provider) applyOpenSearchDomain(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior OpenSearchDomainState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.DomainName != "" {
			_, err := p.opensearchClient.DeleteDomain(ctx, &opensearch.DeleteDomainInput{
				DomainName: &prior.DomainName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete domain: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired OpenSearchDomainConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.opensearchClient.CreateDomain(ctx, &opensearch.CreateDomainInput{
		DomainName:    &desired.DomainName,
		EngineVersion: &desired.EngineVersion,
	})
	if err != nil {
		// return nil, fmt.Errorf("failed to create domain: %w", err)
	}

	newState := OpenSearchDomainState{DomainName: desired.DomainName}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

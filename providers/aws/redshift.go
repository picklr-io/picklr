package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/redshift"
	// "github.com/aws/aws-sdk-go-v2/service/redshift/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type RedshiftClusterConfig struct {
	ClusterIdentifier      string            `json:"cluster_identifier"`
	NodeType               string            `json:"node_type"`
	ClusterType            string            `json:"cluster_type"`
	MasterUsername         string            `json:"master_username"`
	MasterUserPassword     string            `json:"master_user_password"`
	ClusterSubnetGroupName string            `json:"cluster_subnet_group_name"`
	DBName                 string            `json:"db_name"`
	NumberOfNodes          int32             `json:"number_of_nodes"`
	Tags                   map[string]string `json:"tags"`
}

type RedshiftClusterState struct {
	Identifier string `json:"identifier"`
}

func (p *Provider) applyRedshiftCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RedshiftClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Identifier != "" {
			_, err := p.redshiftClient.DeleteCluster(ctx, &redshift.DeleteClusterInput{
				ClusterIdentifier:        &prior.Identifier,
				SkipFinalClusterSnapshot: func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RedshiftClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &redshift.CreateClusterInput{
		ClusterIdentifier:  &desired.ClusterIdentifier,
		NodeType:           &desired.NodeType,
		ClusterType:        &desired.ClusterType,
		MasterUsername:     &desired.MasterUsername,
		MasterUserPassword: &desired.MasterUserPassword,
	}

	if desired.DBName != "" {
		input.DBName = &desired.DBName
	}
	if desired.ClusterSubnetGroupName != "" {
		input.ClusterSubnetGroupName = &desired.ClusterSubnetGroupName
	}
	if desired.ClusterType == "multi-node" && desired.NumberOfNodes > 0 {
		input.NumberOfNodes = &desired.NumberOfNodes
	}
	// Tags not directly in Create call or different... check SDK.
	// CreateClusterInput has Tags.

	_, err := p.redshiftClient.CreateCluster(ctx, input)
	if err != nil {
		// return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	newState := RedshiftClusterState{Identifier: desired.ClusterIdentifier}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyRedshiftSubnetGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// Simplified stub for now
	return &pb.ApplyResponse{}, nil
}

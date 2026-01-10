package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type DBInstanceConfig struct {
	Identifier         string `json:"identifier"`
	Engine             string `json:"engine"`
	InstanceClass      string `json:"instanceClass"`
	AllocatedStorage   int    `json:"allocatedStorage"`
	MasterUsername     string `json:"masterUsername"`
	MasterUserPassword string `json:"masterUserPassword"`
}

type DBInstanceState struct {
	Identifier string `json:"identifier"`
	ARN        string `json:"arn"`
}

func (p *Provider) applyDBInstance(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior DBInstanceState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Identifier != "" {
			_, err := p.rdsClient.DeleteDBInstance(ctx, &rds.DeleteDBInstanceInput{
				DBInstanceIdentifier: &prior.Identifier,
				SkipFinalSnapshot:    func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete DB instance: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DBInstanceConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// CREATE
	input := &rds.CreateDBInstanceInput{
		DBInstanceIdentifier: &desired.Identifier,
		Engine:               &desired.Engine,
		DBInstanceClass:      &desired.InstanceClass,
		AllocatedStorage:     func(i int32) *int32 { return &i }(int32(desired.AllocatedStorage)),
		MasterUsername:       &desired.MasterUsername,
		MasterUserPassword:   &desired.MasterUserPassword,
	}

	// Wait for creation to complete? RDS takes a long time.
	// For MVP, we might just fire Create and return the ID.
	// But Terraform usually waits.

	resp, err := p.rdsClient.CreateDBInstance(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create db instance: %w", err)
	}

	// Wait for available
	waiter := rds.NewDBInstanceAvailableWaiter(p.rdsClient)
	if err := waiter.Wait(ctx, &rds.DescribeDBInstancesInput{
		DBInstanceIdentifier: resp.DBInstance.DBInstanceIdentifier,
	}, 20*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to wait for db instance available: %w", err)
	}

	newState := DBInstanceState{
		Identifier: *resp.DBInstance.DBInstanceIdentifier,
		ARN:        *resp.DBInstance.DBInstanceArn,
	}
	stateJSON, _ := json.Marshal(newState)

	// In a real provider, we'd wait for availability.
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// DBSubnetGroup
type DBSubnetGroupConfig struct {
	Name      string            `json:"name"`
	SubnetIds []string          `json:"subnet_ids"`
	Tags      map[string]string `json:"tags"`
}

type DBSubnetGroupState struct {
	Name string `json:"name"`
}

func (p *Provider) applyDBSubnetGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DBSubnetGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Name != "" {
			_, err := p.rdsClient.DeleteDBSubnetGroup(ctx, &rds.DeleteDBSubnetGroupInput{DBSubnetGroupName: &prior.Name})
			if err != nil {
				return nil, fmt.Errorf("failed to delete db subnet group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DBSubnetGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Create
	input := &rds.CreateDBSubnetGroupInput{
		DBSubnetGroupName:        &desired.Name,
		DBSubnetGroupDescription: &desired.Name, // Use name as description
		SubnetIds:                desired.SubnetIds,
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.rdsClient.CreateDBSubnetGroup(ctx, input)
	if err != nil {
		// handle already exists?
		return nil, fmt.Errorf("failed to create db subnet group: %w", err)
	}

	newState := DBSubnetGroupState{Name: *resp.DBSubnetGroup.DBSubnetGroupName}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// DBCluster
type DBClusterConfig struct {
	Identifier         string            `json:"identifier"`
	Engine             string            `json:"engine"`
	MasterUsername     string            `json:"master_username"`
	MasterUserPassword string            `json:"master_user_password"`
	DatabaseName       string            `json:"database_name"`
	DBSubnetGroupName  string            `json:"db_subnet_group_name"`
	Tags               map[string]string `json:"tags"`
}

type DBClusterState struct {
	Identifier string `json:"identifier"`
	Endpoint   string `json:"endpoint"`
}

func (p *Provider) applyDBCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DBClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Identifier != "" {
			_, err := p.rdsClient.DeleteDBCluster(ctx, &rds.DeleteDBClusterInput{
				DBClusterIdentifier: &prior.Identifier,
				SkipFinalSnapshot:   func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete db cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DBClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &rds.CreateDBClusterInput{
		DBClusterIdentifier: &desired.Identifier,
		Engine:              &desired.Engine,
		MasterUsername:      &desired.MasterUsername,
		MasterUserPassword:  &desired.MasterUserPassword,
	}

	if desired.DatabaseName != "" {
		input.DatabaseName = &desired.DatabaseName
	}
	if desired.DBSubnetGroupName != "" {
		input.DBSubnetGroupName = &desired.DBSubnetGroupName
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.rdsClient.CreateDBCluster(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create db cluster: %w", err)
	}

	// Wait logic could be added here similar to Instance

	newState := DBClusterState{
		Identifier: *resp.DBCluster.DBClusterIdentifier,
		// Endpoint might not be available yet
	}
	if resp.DBCluster.Endpoint != nil {
		newState.Endpoint = *resp.DBCluster.Endpoint
	}

	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/rds"
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

package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/route53/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type HostedZoneConfig struct {
	Name string `json:"name"`
}

type HostedZoneState struct {
	Name   string `json:"name"`
	ID     string `json:"id"`
	ZoneID string `json:"zone_id"` // ID without prefix
}

type RecordSetConfig struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	TTL     int      `json:"ttl"`
	ZoneID  string   `json:"zone_id"`
	Records []string `json:"records"`
}

type RecordSetState struct {
	Name string `json:"name"`
	ID   string `json:"id"` // Unique identifier (ZoneID + Name + Type)
}

func (p *Provider) applyHostedZone(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior HostedZoneState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.route53Client.DeleteHostedZone(ctx, &route53.DeleteHostedZoneInput{
				Id: &prior.ID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete hosted zone: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired HostedZoneConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	callerRef := fmt.Sprintf("picklr-%d", time.Now().UnixNano())
	resp, err := p.route53Client.CreateHostedZone(ctx, &route53.CreateHostedZoneInput{
		Name:            &desired.Name,
		CallerReference: &callerRef,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create hosted zone: %w", err)
	}

	newState := HostedZoneState{
		Name:   *resp.HostedZone.Name,
		ID:     *resp.HostedZone.Id,
		ZoneID: *resp.HostedZone.Id, // Usually /hostedzone/ID, but often treated as ID
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyRecordSet(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RecordSetState
		// Deletion logic would go here, requiring reconstructing the input from state
		// For basic MVP, we might skip complex deletion reconstruction if state is minimal
		// But optimally: DELETE action in ChangeBatch
		_ = prior // Suppress unused error
		return &pb.ApplyResponse{}, nil
	}

	var desired RecordSetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var resourceRecords []types.ResourceRecord
	for _, r := range desired.Records {
		resourceRecords = append(resourceRecords, types.ResourceRecord{
			Value: func(s string) *string { return &s }(r),
		})
	}

	input := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: &desired.ZoneID,
		ChangeBatch: &types.ChangeBatch{
			Changes: []types.Change{
				{
					Action: types.ChangeActionUpsert,
					ResourceRecordSet: &types.ResourceRecordSet{
						Name:            &desired.Name,
						Type:            types.RRType(desired.Type),
						TTL:             func(i int64) *int64 { return &i }(int64(desired.TTL)),
						ResourceRecords: resourceRecords,
					},
				},
			},
		},
	}

	_, err := p.route53Client.ChangeResourceRecordSets(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert record set: %w", err)
	}

	newState := RecordSetState{
		Name: desired.Name,
		ID:   fmt.Sprintf("%s:%s:%s", desired.ZoneID, desired.Name, desired.Type),
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

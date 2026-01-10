package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type KeyConfig struct {
	Description string `json:"description"`
	KeyUsage    string `json:"key_usage"`
	Enabled     bool   `json:"enabled"`
}

type KeyState struct {
	KeyID string `json:"key_id"`
	ARN   string `json:"arn"`
}

type AliasConfig struct {
	AliasName   string `json:"alias_name"`
	TargetKeyID string `json:"target_key_id"`
}

type AliasState struct {
	AliasName string `json:"alias_name"`
	ARN       string `json:"arn"`
}

func (p *Provider) applyKey(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior KeyState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.KeyID != "" {
			_, err := p.kmsClient.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
				KeyId:               &prior.KeyID,
				PendingWindowInDays: func(i int32) *int32 { return &i }(7),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to schedule key deletion: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired KeyConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.kmsClient.CreateKey(ctx, &kms.CreateKeyInput{
		Description: &desired.Description,
		KeyUsage:    types.KeyUsageType(desired.KeyUsage),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create key: %w", err)
	}

	if !desired.Enabled {
		_, err = p.kmsClient.DisableKey(ctx, &kms.DisableKeyInput{
			KeyId: resp.KeyMetadata.KeyId,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to disable key: %w", err)
		}
	}

	newState := KeyState{
		KeyID: *resp.KeyMetadata.KeyId,
		ARN:   *resp.KeyMetadata.Arn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyAlias(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AliasState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.AliasName != "" {
			_, err := p.kmsClient.DeleteAlias(ctx, &kms.DeleteAliasInput{
				AliasName: &prior.AliasName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete alias: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AliasConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.kmsClient.CreateAlias(ctx, &kms.CreateAliasInput{
		AliasName:   &desired.AliasName,
		TargetKeyId: &desired.TargetKeyID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create alias: %w", err)
	}

	newState := AliasState{
		AliasName: desired.AliasName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

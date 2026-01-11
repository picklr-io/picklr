package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type SecretConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	KmsKeyID    string `json:"kmsKeyId"`
}

type SecretState struct {
	ARN  string `json:"arn"`
	Name string `json:"name"`
}

type SecretVersionConfig struct {
	SecretID     string `json:"secretId"`
	SecretString string `json:"secretString"`
}

type SecretVersionState struct {
	VersionID string `json:"versionId"`
}

func (p *Provider) applySecret(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SecretState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			// Secrets Manager ForceDeleteWithoutRecovery is safer for dev
			_, err := p.secretsmanagerClient.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
				SecretId:                   &prior.ARN,
				ForceDeleteWithoutRecovery: func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete secret: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SecretConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &secretsmanager.CreateSecretInput{
		Name: &desired.Name,
	}
	if desired.Description != "" {
		input.Description = &desired.Description
	}
	if desired.KmsKeyID != "" {
		input.KmsKeyId = &desired.KmsKeyID
	}

	resp, err := p.secretsmanagerClient.CreateSecret(ctx, input)
	if err != nil {
		// handle existing secret
		// (omitted for brevity, assume new for now or explicit error)
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}

	newState := SecretState{
		ARN:  *resp.ARN,
		Name: *resp.Name,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applySecretVersion(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		// Version deletion logic is tricky, usually we let the secret deletion handle it.
		// Or we can delete version specifically if needed.
		// For now, no-op or assume secret deletion cleans up.
		return &pb.ApplyResponse{}, nil
	}

	var desired SecretVersionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// PutSecretValue
	// Note: SecretID can be Name or ARN.
	input := &secretsmanager.PutSecretValueInput{
		SecretId:     &desired.SecretID,
		SecretString: &desired.SecretString,
	}

	resp, err := p.secretsmanagerClient.PutSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to put secret value: %w", err)
	}

	newState := SecretVersionState{
		VersionID: *resp.VersionId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// SecretPolicy
type SecretPolicyConfig struct {
	SecretID          string `json:"secretId"`
	ResourcePolicy    string `json:"resourcePolicy"`
	BlockPublicPolicy bool   `json:"blockPublicPolicy"`
}

type SecretPolicyState struct {
	SecretID string `json:"secretId"`
}

func (p *Provider) applySecretPolicy(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SecretPolicyState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.SecretID != "" {
			_, err := p.secretsmanagerClient.DeleteResourcePolicy(ctx, &secretsmanager.DeleteResourcePolicyInput{
				SecretId: &prior.SecretID,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete secret policy: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SecretPolicyConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.secretsmanagerClient.PutResourcePolicy(ctx, &secretsmanager.PutResourcePolicyInput{
		SecretId:          &desired.SecretID,
		ResourcePolicy:    &desired.ResourcePolicy,
		BlockPublicPolicy: &desired.BlockPublicPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put resource policy: %w", err)
	}

	newState := SecretPolicyState{SecretID: desired.SecretID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// SES Email Identity

type EmailIdentityConfig struct {
	EmailIdentity        string               `json:"email_identity"`
	DkimSigningAttributes *DkimSigningAttrCfg `json:"dkim_signing_attributes"`
	Tags                 map[string]string    `json:"tags"`
}

type DkimSigningAttrCfg struct {
	DomainSigningSelector   string `json:"domain_signing_selector"`
	DomainSigningPrivateKey string `json:"domain_signing_private_key"`
	NextSigningKeyLength    string `json:"next_signing_key_length"`
}

type EmailIdentityState struct {
	EmailIdentity        string `json:"emailIdentity"`
	IdentityType         string `json:"identityType"`
	VerifiedForSendingStatus bool `json:"verifiedForSendingStatus"`
}

func (p *Provider) applyEmailIdentity(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EmailIdentityState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.EmailIdentity != "" {
			_, err := p.sesv2Client.DeleteEmailIdentity(ctx, &sesv2.DeleteEmailIdentityInput{
				EmailIdentity: &prior.EmailIdentity,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete email identity: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EmailIdentityConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &sesv2.CreateEmailIdentityInput{
		EmailIdentity: &desired.EmailIdentity,
	}

	if desired.DkimSigningAttributes != nil {
		input.DkimSigningAttributes = &types.DkimSigningAttributes{}
		if desired.DkimSigningAttributes.DomainSigningSelector != "" {
			input.DkimSigningAttributes.DomainSigningSelector = &desired.DkimSigningAttributes.DomainSigningSelector
		}
		if desired.DkimSigningAttributes.DomainSigningPrivateKey != "" {
			input.DkimSigningAttributes.DomainSigningPrivateKey = &desired.DkimSigningAttributes.DomainSigningPrivateKey
		}
		if desired.DkimSigningAttributes.NextSigningKeyLength != "" {
			input.DkimSigningAttributes.NextSigningKeyLength = types.DkimSigningKeyLength(desired.DkimSigningAttributes.NextSigningKeyLength)
		}
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.sesv2Client.CreateEmailIdentity(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create email identity: %w", err)
	}

	newState := EmailIdentityState{
		EmailIdentity:        desired.EmailIdentity,
		IdentityType:         string(resp.IdentityType),
		VerifiedForSendingStatus: resp.VerifiedForSendingStatus,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// SES Configuration Set

type SESConfigSetConfig struct {
	ConfigurationSetName    string            `json:"configuration_set_name"`
	SendingEnabled          bool              `json:"sending_enabled"`
	ReputationMetricsEnabled bool             `json:"reputation_metrics_enabled"`
	TlsPolicy              string            `json:"tls_policy"`
	Tags                   map[string]string  `json:"tags"`
}

type SESConfigSetState struct {
	ConfigurationSetName string `json:"configurationSetName"`
}

func (p *Provider) applyConfigurationSet(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SESConfigSetState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ConfigurationSetName != "" {
			_, err := p.sesv2Client.DeleteConfigurationSet(ctx, &sesv2.DeleteConfigurationSetInput{
				ConfigurationSetName: &prior.ConfigurationSetName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete configuration set: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SESConfigSetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &sesv2.CreateConfigurationSetInput{
		ConfigurationSetName: &desired.ConfigurationSetName,
		SendingOptions: &types.SendingOptions{
			SendingEnabled: desired.SendingEnabled,
		},
		ReputationOptions: &types.ReputationOptions{
			ReputationMetricsEnabled: desired.ReputationMetricsEnabled,
		},
		DeliveryOptions: &types.DeliveryOptions{
			TlsPolicy: types.TlsPolicy(desired.TlsPolicy),
		},
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	_, err := p.sesv2Client.CreateConfigurationSet(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration set: %w", err)
	}

	newState := SESConfigSetState{
		ConfigurationSetName: desired.ConfigurationSetName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

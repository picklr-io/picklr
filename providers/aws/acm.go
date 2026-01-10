package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/acm/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type CertificateConfig struct {
	DomainName       string            `json:"domainName"`
	ValidationMethod string            `json:"validationMethod"`
	Tags             map[string]string `json:"tags"`
}

type CertificateState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyCertificate(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CertificateState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.acmClient.DeleteCertificate(ctx, &acm.DeleteCertificateInput{
				CertificateArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete certificate: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CertificateConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Check if key exists in prior state
	if req.PriorStateJson != nil {
		// Update logic could go here (e.g. tags). For now, we assume immutable domain name.
		return &pb.ApplyResponse{NewStateJson: req.PriorStateJson}, nil
	}

	input := &acm.RequestCertificateInput{
		DomainName:       &desired.DomainName,
		ValidationMethod: types.ValidationMethod(desired.ValidationMethod),
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.acmClient.RequestCertificate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to request certificate: %w", err)
	}

	newState := CertificateState{ARN: *resp.CertificateArn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type CertificateValidationConfig struct {
	CertificateArn string `json:"certificateArn"`
}

type CertificateValidationState struct {
	CertificateArn string `json:"certificateArn"`
}

// applyCertificateValidation waits for the certificate to be issued.
func (p *Provider) applyCertificateValidation(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		// No-op for delete, as this is just a waiter resource
		return &pb.ApplyResponse{}, nil
	}

	var desired CertificateValidationConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	waiter := acm.NewCertificateValidatedWaiter(p.acmClient)
	// Wait up to 5 minutes
	if err := waiter.Wait(ctx, &acm.DescribeCertificateInput{
		CertificateArn: &desired.CertificateArn,
	}, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to wait for certificate validation: %w", err)
	}

	newState := CertificateValidationState{CertificateArn: desired.CertificateArn}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

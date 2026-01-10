package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type BucketConfig struct {
	Bucket       string `json:"bucket"`
	ACL          string `json:"acl"`
	ForceDestroy bool   `json:"force_destroy"`
}

type BucketState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) planBucket(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {
	if req.DesiredConfigJson == nil && req.PriorStateJson != nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_DELETE}, nil
	}

	if req.PriorStateJson == nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	var prior BucketState
	if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
	}

	var desired BucketConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// 1. Check Drift: Does it exist?
	_, err := p.s3Client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &prior.Name,
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() == "NotFound" {
				// It's gone. We need to create it again.
				return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
			}
		}
		// Some other error (e.g. Forbidden), assume we can't check, basic diff
		// or fail? Let's fail safe -> error
		return nil, fmt.Errorf("failed to check bucket existence: %w", err)
	}

	// 2. Compare State
	if prior.Name != desired.Bucket {
		// Renaming requires replacement
		return &pb.PlanResponse{Action: pb.PlanResponse_REPLACE}, nil
	}

	// 3. Compare Mutable Fields (e.g. tags, ACL) - ACL requires API call to check real state
	// For now, we only support basic existence + name check.
	// If configs differ (and name is same), it's an UPDATE.
	if string(req.DesiredConfigJson) != string(req.PriorStateJson) {
		// TODO: Deep compare to avoid noise
		return &pb.PlanResponse{Action: pb.PlanResponse_UPDATE}, nil
	}

	return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
}

func (p *Provider) applyBucket(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior BucketState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
				Bucket: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete bucket: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired BucketConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// CREATE / UPDATE
	// S3 buckets are global unique, so 'create' is idempotent if we own it.

	// If it's an update (e.g. ACL changed), we should handle it.
	// Since CreateBucket doesn't update ACLs, we might need PutBucketAcl here if ACL changed.
	// For now, CreateBucket is safe for existence check/creation.

	_, err := p.s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: &desired.Bucket,
	})
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			// If already exists and owned by us, it's fine (idempotent for Create).
			// Code: BucketAlreadyOwnedByYou
			if ae.ErrorCode() != "BucketAlreadyOwnedByYou" {
				return nil, fmt.Errorf("failed to create bucket: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	// TODO: Apply updates (ACL, Tags) here

	newState := BucketState{
		Name: desired.Bucket,
		ARN:  fmt.Sprintf("arn:aws:s3:::%s", desired.Bucket),
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

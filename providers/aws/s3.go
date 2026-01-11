package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
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

type BucketPolicyConfig struct {
	Bucket string `json:"bucket"`
	Policy string `json:"policy"`
}

type BucketPolicyState struct {
	Bucket string `json:"bucket"`
}

func (p *Provider) applyBucketPolicy(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior BucketPolicyState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Bucket != "" {
			_, err := p.s3Client.DeleteBucketPolicy(ctx, &s3.DeleteBucketPolicyInput{
				Bucket: &prior.Bucket,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete bucket policy: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired BucketPolicyConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.s3Client.PutBucketPolicy(ctx, &s3.PutBucketPolicyInput{
		Bucket: &desired.Bucket,
		Policy: &desired.Policy,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put bucket policy: %w", err)
	}

	newState := BucketPolicyState{Bucket: desired.Bucket}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// BucketLifecycle
type LifecycleRuleConfig struct {
	ID         string            `json:"id"`
	Status     string            `json:"status"`
	Expiration *ExpirationConfig `json:"expiration"`
}

type ExpirationConfig struct {
	Days int `json:"days"`
}

type BucketLifecycleConfig struct {
	Bucket string                `json:"bucket"`
	Rules  []LifecycleRuleConfig `json:"rules"`
}

type BucketLifecycleState struct {
	Bucket string `json:"bucket"`
}

func (p *Provider) applyBucketLifecycle(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior BucketLifecycleState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Bucket != "" {
			_, err := p.s3Client.DeleteBucketLifecycle(ctx, &s3.DeleteBucketLifecycleInput{Bucket: &prior.Bucket})
			if err != nil {
				return nil, fmt.Errorf("failed to delete bucket lifecycle: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired BucketLifecycleConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var rules []types.LifecycleRule
	for _, r := range desired.Rules {
		rule := types.LifecycleRule{
			ID:     &r.ID,
			Status: types.ExpirationStatus(r.Status),
			Filter: &types.LifecycleRuleFilter{Prefix: func(s string) *string { return &s }("")}, // Applies to all if not specified, simplistic
		}
		if r.Expiration != nil {
			rule.Expiration = &types.LifecycleExpiration{
				Days: func(i int32) *int32 { return &i }(int32(r.Expiration.Days)),
			}
		}
		rules = append(rules, rule)
	}

	_, err := p.s3Client.PutBucketLifecycleConfiguration(ctx, &s3.PutBucketLifecycleConfigurationInput{
		Bucket:                 &desired.Bucket,
		LifecycleConfiguration: &types.BucketLifecycleConfiguration{Rules: rules},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put bucket lifecycle: %w", err)
	}

	newState := BucketLifecycleState{Bucket: desired.Bucket}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// BucketNotification
type LambdaFunctionConfigurationConfig struct {
	LambdaFunctionArn string   `json:"lambdaFunctionArn"`
	Events            []string `json:"events"`
}

type BucketNotificationConfig struct {
	Bucket                       string                              `json:"bucket"`
	LambdaFunctionConfigurations []LambdaFunctionConfigurationConfig `json:"lambda_function_configurations"`
}

type BucketNotificationState struct {
	Bucket string `json:"bucket"`
}

func (p *Provider) applyBucketNotification(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		// S3 Notification configuration can be cleared by putting empty config.
		var prior BucketNotificationState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.Bucket != "" {
			_, err := p.s3Client.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
				Bucket:                    &prior.Bucket,
				NotificationConfiguration: &types.NotificationConfiguration{},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to clear bucket notification: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired BucketNotificationConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	nc := &types.NotificationConfiguration{}

	for _, l := range desired.LambdaFunctionConfigurations {
		var events []types.Event
		for _, e := range l.Events {
			events = append(events, types.Event(e))
		}
		nc.LambdaFunctionConfigurations = append(nc.LambdaFunctionConfigurations, types.LambdaFunctionConfiguration{
			LambdaFunctionArn: &l.LambdaFunctionArn,
			Events:            events,
		})
	}

	_, err := p.s3Client.PutBucketNotificationConfiguration(ctx, &s3.PutBucketNotificationConfigurationInput{
		Bucket:                    &desired.Bucket,
		NotificationConfiguration: nc,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put bucket notification: %w", err)
	}

	newState := BucketNotificationState{Bucket: desired.Bucket}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

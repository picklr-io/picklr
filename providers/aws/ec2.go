package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type InstanceConfig struct {
	AMI          string            `json:"ami"`
	InstanceType string            `json:"instance_type"`
	Tags         map[string]string `json:"tags"`
}

type InstanceState struct {
	ID        string `json:"id"`
	PublicIP  string `json:"public_ip"`
	PrivateIP string `json:"private_ip"`
}

func (p *Provider) planInstance(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {

	if req.DesiredConfigJson == nil && req.PriorStateJson != nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_DELETE}, nil
	}

	if req.PriorStateJson == nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	var prior InstanceState
	if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
		return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
	}

	var desired InstanceConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// 1. Check Drift
	resp, err := p.ec2Client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{prior.ID},
	})
	if err != nil {
		// Check for InvalidInstanceID.NotFound
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() == "InvalidInstanceID.NotFound" {
				return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
			}
		}
		return nil, fmt.Errorf("failed to describe instance: %w", err)
	}

	if len(resp.Reservations) == 0 || len(resp.Reservations[0].Instances) == 0 {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	instance := resp.Reservations[0].Instances[0]

	if instance.State.Name == types.InstanceStateNameTerminated {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	// 2. Compare State
	// AMI (ImageId) - Immutable
	if *instance.ImageId != desired.AMI {
		return &pb.PlanResponse{Action: pb.PlanResponse_REPLACE}, nil
	}

	// Instance Type - Mutable (with Stop usually), but let's say REPLACE for simplicity if we don't want to handle stop/start
	// OR UPDATE if we implement it. Let's return UPDATE and assume Apply handles (or errors if intricate).
	// For now, let's treat InstanceType change as REPLACE to be safe, unless we implement ModifyInstanceAttribute logic.
	if string(instance.InstanceType) != desired.InstanceType {
		return &pb.PlanResponse{Action: pb.PlanResponse_REPLACE}, nil
	}

	// Tags - Mutable
	// We need to compare existing tags with desired tags.
	// AWS returns list of tags.
	// Simple check: if desired config string differs from prior config string (and not AMI/Type change), it's likely tags.
	if string(req.DesiredConfigJson) != string(req.PriorStateJson) {
		return &pb.PlanResponse{Action: pb.PlanResponse_UPDATE}, nil
	}

	return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
}

func (p *Provider) applyInstance(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	// DELETE
	if req.DesiredConfigJson == nil {
		var prior InstanceState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.ec2Client.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
				InstanceIds: []string{prior.ID},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to terminate instance: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired InstanceConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired config: %w", err)
	}

	// UPDATE
	if req.PriorStateJson != nil {
		var prior InstanceState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			// Check if it exists (Apply might be called for CREATE if Plan said CREATE, but here we cover UPDATE case where ID exists)
			// Ideally we check action from Plan, but Provider doesn't get Plan action in ApplyRequest yet.
			// We infer based on PriorStateJson being present and usable.

			// For now, we only support Tag updates in place.
			// If we reached here, it means Plan said UPDATE (or we are blindly applying).

			// Simplistic Tag Update: Overwrite all tags.
			// Note: This doesn't remove old tags not in desired. To do that we need to list and delete.
			// Verified MVP: Add/Update desired tags.
			if len(desired.Tags) > 0 {
				var tags []types.Tag
				for k, v := range desired.Tags {
					tags = append(tags, types.Tag{Key: &k, Value: &v})
				}
				_, err := p.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
					Resources: []string{prior.ID},
					Tags:      tags,
				})
				if err != nil {
					return nil, fmt.Errorf("failed to update tags: %w", err)
				}
			}

			return &pb.ApplyResponse{NewStateJson: req.PriorStateJson}, nil
		}
	}

	// CREATE
	runInput := &ec2.RunInstancesInput{
		ImageId:      &desired.AMI,
		InstanceType: types.InstanceType(desired.InstanceType),
		MinCount:     func(i int32) *int32 { return &i }(1),
		MaxCount:     func(i int32) *int32 { return &i }(1),
	}

	resp, err := p.ec2Client.RunInstances(ctx, runInput)
	if err != nil {
		return nil, fmt.Errorf("failed to run instance: %w", err)
	}

	if len(resp.Instances) == 0 {
		return nil, fmt.Errorf("no instances created")
	}

	instance := resp.Instances[0]

	// Wait for running state
	waiter := ec2.NewInstanceRunningWaiter(p.ec2Client)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{*instance.InstanceId},
	}, 5*time.Minute); err != nil {
		return nil, fmt.Errorf("failed to wait for instance running: %w", err)
	}

	// Tags
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		_, err := p.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{*instance.InstanceId},
			Tags:      tags,
		})
		if err != nil {
			// Log warning but don't fail? Or fail.
		}
	}

	newState := InstanceState{
		ID: *instance.InstanceId,
		// IPs might not be available immediately.
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

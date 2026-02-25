package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// CloudTrail Trail

type TrailConfig struct {
	TrailName                   string            `json:"trail_name"`
	S3BucketName                string            `json:"s3_bucket_name"`
	S3KeyPrefix                 string            `json:"s3_key_prefix"`
	IncludeGlobalServiceEvents  bool              `json:"include_global_service_events"`
	IsMultiRegionTrail          bool              `json:"is_multi_region_trail"`
	EnableLogFileValidation     bool              `json:"enable_log_file_validation"`
	CloudWatchLogsLogGroupArn   string            `json:"cloud_watch_logs_log_group_arn"`
	CloudWatchLogsRoleArn       string            `json:"cloud_watch_logs_role_arn"`
	KmsKeyId                    string            `json:"kms_key_id"`
	IsOrganizationTrail         bool              `json:"is_organization_trail"`
	Tags                        map[string]string `json:"tags"`
}

type TrailState struct {
	Name     string `json:"name"`
	ARN      string `json:"arn"`
}

func (p *Provider) applyTrail(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TrailState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.cloudtrailClient.DeleteTrail(ctx, &cloudtrail.DeleteTrailInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete trail: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TrailConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &cloudtrail.CreateTrailInput{
		Name:                       &desired.TrailName,
		S3BucketName:               &desired.S3BucketName,
		IncludeGlobalServiceEvents: &desired.IncludeGlobalServiceEvents,
		IsMultiRegionTrail:         &desired.IsMultiRegionTrail,
		EnableLogFileValidation:    &desired.EnableLogFileValidation,
		IsOrganizationTrail:        &desired.IsOrganizationTrail,
	}

	if desired.S3KeyPrefix != "" {
		input.S3KeyPrefix = &desired.S3KeyPrefix
	}
	if desired.CloudWatchLogsLogGroupArn != "" {
		input.CloudWatchLogsLogGroupArn = &desired.CloudWatchLogsLogGroupArn
	}
	if desired.CloudWatchLogsRoleArn != "" {
		input.CloudWatchLogsRoleArn = &desired.CloudWatchLogsRoleArn
	}
	if desired.KmsKeyId != "" {
		input.KmsKeyId = &desired.KmsKeyId
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.TagsList = tags
	}

	resp, err := p.cloudtrailClient.CreateTrail(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create trail: %w", err)
	}

	newState := TrailState{
		Name: *resp.Name,
	}
	if resp.TrailARN != nil {
		newState.ARN = *resp.TrailARN
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// FileSystem
type FileSystemConfig struct {
	CreationToken   *string           `json:"creation_token"`
	PerformanceMode string            `json:"performance_mode"`
	Encrypted       bool              `json:"encrypted"`
	KmsKeyId        *string           `json:"kms_key_id"`
	Tags            map[string]string `json:"tags"`
}

type FileSystemState struct {
	ID   string `json:"id"`
	Name string `json:"name"` // Not strictly a name, but we might want to track token or tags
}

func (p *Provider) applyFileSystem(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior FileSystemState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.efsClient.DeleteFileSystem(ctx, &efs.DeleteFileSystemInput{FileSystemId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete file system: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired FileSystemConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &efs.CreateFileSystemInput{
		PerformanceMode: types.PerformanceMode(desired.PerformanceMode),
		Encrypted:       func(b bool) *bool { return &b }(desired.Encrypted),
	}
	if desired.CreationToken != nil {
		input.CreationToken = desired.CreationToken
	}
	if desired.KmsKeyId != nil {
		input.KmsKeyId = desired.KmsKeyId
	}
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.Tags = tags
	}

	resp, err := p.efsClient.CreateFileSystem(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create file system: %w", err)
	}

	newState := FileSystemState{ID: *resp.FileSystemId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// MountTarget
type MountTargetConfig struct {
	FileSystemId   string   `json:"file_system_id"`
	SubnetId       string   `json:"subnet_id"`
	SecurityGroups []string `json:"security_groups"`
	IpAddress      *string  `json:"ip_address"`
}

type MountTargetState struct {
	ID string `json:"id"`
}

func (p *Provider) applyMountTarget(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior MountTargetState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.efsClient.DeleteMountTarget(ctx, &efs.DeleteMountTargetInput{MountTargetId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete mount target: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired MountTargetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &efs.CreateMountTargetInput{
		FileSystemId: &desired.FileSystemId,
		SubnetId:     &desired.SubnetId,
	}
	if len(desired.SecurityGroups) > 0 {
		input.SecurityGroups = desired.SecurityGroups
	}
	if desired.IpAddress != nil {
		input.IpAddress = desired.IpAddress
	}

	resp, err := p.efsClient.CreateMountTarget(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create mount target: %w", err)
	}

	newState := MountTargetState{ID: *resp.MountTargetId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

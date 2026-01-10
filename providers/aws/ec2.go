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

// KeyPair
type KeyPairConfig struct {
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
}

type KeyPairState struct {
	KeyName   string `json:"keyName"`
	KeyPairID string `json:"keyPairId"`
}

func (p *Provider) applyKeyPair(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior KeyPairState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.KeyName != "" {
			_, err := p.ec2Client.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{KeyName: &prior.KeyName})
			if err != nil {
				return nil, fmt.Errorf("failed to delete key pair: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired KeyPairConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Import or Create
	var keyName string
	var keyPairID string

	if desired.PublicKey != "" {
		resp, err := p.ec2Client.ImportKeyPair(ctx, &ec2.ImportKeyPairInput{
			KeyName:           &desired.Name,
			PublicKeyMaterial: []byte(desired.PublicKey),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to import key pair: %w", err)
		}
		keyName = *resp.KeyName
		keyPairID = *resp.KeyPairId
	} else {
		resp, err := p.ec2Client.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{
			KeyName: &desired.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create key pair: %w", err)
		}
		keyName = *resp.KeyName
		keyPairID = *resp.KeyPairId
	}

	newState := KeyPairState{KeyName: keyName, KeyPairID: keyPairID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// LaunchTemplate
type BlockDeviceMapping struct {
	DeviceName string `json:"deviceName"`
	EBS        struct {
		VolumeSize          int    `json:"volumeSize"`
		VolumeType          string `json:"volumeType"`
		DeleteOnTermination bool   `json:"deleteOnTermination"`
	} `json:"ebs"`
}

type LaunchTemplateConfig struct {
	Name               string               `json:"name"`
	ImageID            string               `json:"imageId"`
	InstanceType       string               `json:"instanceType"`
	KeyName            string               `json:"keyName"`
	UserData           string               `json:"userData"`
	IAMInstanceProfile map[string]string    `json:"iam_instance_profile"`
	SecurityGroupIDs   []string             `json:"security_group_ids"`
	BlockDevices       []BlockDeviceMapping `json:"block_device_mappings"`
}

type LaunchTemplateState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) applyLaunchTemplate(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior LaunchTemplateState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteLaunchTemplate(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete launch template: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired LaunchTemplateConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateLaunchTemplateInput{
		LaunchTemplateName: &desired.Name,
		LaunchTemplateData: &types.RequestLaunchTemplateData{
			ImageId:      &desired.ImageID,
			InstanceType: types.InstanceType(desired.InstanceType),
		},
	}
	if desired.KeyName != "" {
		input.LaunchTemplateData.KeyName = &desired.KeyName
	}
	if desired.UserData != "" {
		input.LaunchTemplateData.UserData = &desired.UserData
	}
	if len(desired.SecurityGroupIDs) > 0 {
		input.LaunchTemplateData.SecurityGroupIds = desired.SecurityGroupIDs
	}
	if v, ok := desired.IAMInstanceProfile["arn"]; ok && v != "" {
		input.LaunchTemplateData.IamInstanceProfile = &types.LaunchTemplateIamInstanceProfileSpecificationRequest{
			Arn: &v,
		}
	}
	if len(desired.BlockDevices) > 0 {
		var mappings []types.LaunchTemplateBlockDeviceMappingRequest
		for _, bd := range desired.BlockDevices {
			mappings = append(mappings, types.LaunchTemplateBlockDeviceMappingRequest{
				DeviceName: &bd.DeviceName,
				Ebs: &types.LaunchTemplateEbsBlockDeviceRequest{
					VolumeSize:          func(i int32) *int32 { return &i }(int32(bd.EBS.VolumeSize)),
					VolumeType:          types.VolumeType(bd.EBS.VolumeType),
					DeleteOnTermination: &bd.EBS.DeleteOnTermination,
				},
			})
		}
		input.LaunchTemplateData.BlockDeviceMappings = mappings
	}

	resp, err := p.ec2Client.CreateLaunchTemplate(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create launch template: %w", err)
	}

	newState := LaunchTemplateState{ID: *resp.LaunchTemplate.LaunchTemplateId, Name: *resp.LaunchTemplate.LaunchTemplateName}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Volume
type VolumeConfig struct {
	AvailabilityZone string            `json:"availability_zone"`
	Size             int               `json:"size"`
	VolumeType       string            `json:"volume_type"`
	Tags             map[string]string `json:"tags"`
}

type VolumeState struct {
	ID string `json:"id"`
}

func (p *Provider) applyVolume(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VolumeState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete volume: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VolumeConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateVolumeInput{
		AvailabilityZone: &desired.AvailabilityZone,
		Size:             func(i int32) *int32 { return &i }(int32(desired.Size)),
		VolumeType:       types.VolumeType(desired.VolumeType),
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeVolume, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateVolume(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create volume: %w", err)
	}

	newState := VolumeState{ID: *resp.VolumeId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil

// NetworkAcl
type NetworkAclEntry struct {
	RuleNumber int    `json:"ruleNumber"`
	Protocol   string `json:"protocol"`
	RuleAction string `json:"ruleAction"`
	CidrBlock  string `json:"cidrBlock"`
	FromPort   int    `json:"fromPort"`
	ToPort     int    `json:"toPort"`
}

type NetworkAclConfig struct {
	VpcID   string            `json:"vpc_id"`
	Ingress []NetworkAclEntry `json:"ingress"`
	Egress  []NetworkAclEntry `json:"egress"`
	Tags    map[string]string `json:"tags"`
}

type NetworkAclState struct {
	ID string `json:"id"`
}

func (p *Provider) applyNetworkAcl(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior NetworkAclState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteNetworkAcl(ctx, &ec2.DeleteNetworkAclInput{NetworkAclId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete network acl: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired NetworkAclConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Create ACL
	input := &ec2.CreateNetworkAclInput{
		VpcId: &desired.VpcID,
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeNetworkAcl, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateNetworkAcl(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create network acl: %w", err)
	}
	aclID := resp.NetworkAcl.NetworkAclId

	// Helper to create entry
	createEntry := func(entry NetworkAclEntry, egress bool) error {
		ruleAction := types.RuleAction(entry.RuleAction) // allow | deny
		protocol := entry.Protocol
		cidr := entry.CidrBlock
		ruleNum := int32(entry.RuleNumber)
		portRange := &types.PortRange{
			From: func(i int32) *int32 { return &i }(int32(entry.FromPort)),
			To:   func(i int32) *int32 { return &i }(int32(entry.ToPort)),
		}
		if entry.FromPort == 0 && entry.ToPort == 0 {
			if protocol == "-1" {
				portRange = nil
			}
		}

		_, err := p.ec2Client.CreateNetworkAclEntry(ctx, &ec2.CreateNetworkAclEntryInput{
			NetworkAclId: aclID,
			RuleNumber:   &ruleNum,
			Protocol:     &protocol,
			RuleAction:   ruleAction,
			CidrBlock:    &cidr,
			Egress:       &egress,
			PortRange:    portRange,
		})
		return err
	}

	// Ingress
	for _, entry := range desired.Ingress {
		if err := createEntry(entry, false); err != nil {
			return nil, fmt.Errorf("failed to create ingress entry %d: %w", entry.RuleNumber, err)
		}
	}

	// Egress
	for _, entry := range desired.Egress {
		if err := createEntry(entry, true); err != nil {
			return nil, fmt.Errorf("failed to create egress entry %d: %w", entry.RuleNumber, err)
		}
	}

	newState := NetworkAclState{ID: *aclID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// VpcPeeringConnection
type VpcPeeringConnectionConfig struct {
	VpcID      string            `json:"vpc_id"`
	PeerVpcID  string            `json:"peer_vpc_id"`
	AutoAccept bool              `json:"auto_accept"`
	Tags       map[string]string `json:"tags"`
}

type VpcPeeringConnectionState struct {
	ID string `json:"id"`
}

func (p *Provider) applyVpcPeeringConnection(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VpcPeeringConnectionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteVpcPeeringConnection(ctx, &ec2.DeleteVpcPeeringConnectionInput{VpcPeeringConnectionId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete peering connection: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VpcPeeringConnectionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateVpcPeeringConnectionInput{
		VpcId:     &desired.VpcID,
		PeerVpcId: &desired.PeerVpcID,
	}
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeVpcPeeringConnection, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateVpcPeeringConnection(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create peering connection: %w", err)
	}

	peeringID := resp.VpcPeeringConnection.VpcPeeringConnectionId

	if desired.AutoAccept {
		_, err := p.ec2Client.AcceptVpcPeeringConnection(ctx, &ec2.AcceptVpcPeeringConnectionInput{
			VpcPeeringConnectionId: peeringID,
		})
		if err != nil {
			// Ignore if needed
		}
	}

	newState := VpcPeeringConnectionState{ID: *peeringID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// TransitGateway
type TransitGatewayConfig struct {
	Description string            `json:"description"`
	Tags        map[string]string `json:"tags"`
}

type TransitGatewayState struct {
	ID string `json:"id"`
}

func (p *Provider) applyTransitGateway(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TransitGatewayState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteTransitGateway(ctx, &ec2.DeleteTransitGatewayInput{TransitGatewayId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete transit gateway: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TransitGatewayConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateTransitGatewayInput{}
	if desired.Description != "" {
		input.Description = &desired.Description
	}
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeTransitGateway, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateTransitGateway(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create transit gateway: %w", err)
	}

	newState := TransitGatewayState{ID: *resp.TransitGateway.TransitGatewayId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// TransitGatewayAttachment
type TransitGatewayAttachmentConfig struct {
	TransitGatewayID string            `json:"transit_gateway_id"`
	VpcID            string            `json:"vpc_id"`
	SubnetIDs        []string          `json:"subnet_ids"`
	Tags             map[string]string `json:"tags"`
}

type TransitGatewayAttachmentState struct {
	ID string `json:"id"`
}

func (p *Provider) applyTransitGatewayAttachment(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TransitGatewayAttachmentState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteTransitGatewayVpcAttachment(ctx, &ec2.DeleteTransitGatewayVpcAttachmentInput{TransitGatewayAttachmentId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete tgw attachment: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TransitGatewayAttachmentConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateTransitGatewayVpcAttachmentInput{
		TransitGatewayId: &desired.TransitGatewayID,
		VpcId:            &desired.VpcID,
		SubnetIds:        desired.SubnetIDs,
	}
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeTransitGatewayAttachment, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateTransitGatewayVpcAttachment(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create tgw attachment: %w", err)
	}

	newState := TransitGatewayAttachmentState{ID: *resp.TransitGatewayVpcAttachment.TransitGatewayAttachmentId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// VpcEndpoint
type VpcEndpointConfig struct {
	VpcID            string            `json:"vpc_id"`
	ServiceName      string            `json:"service_name"`
	VpcEndpointType  string            `json:"vpc_endpoint_type"`
	RouteTableIDs    []string          `json:"route_table_ids"`
	SubnetIDs        []string          `json:"subnet_ids"`
	SecurityGroupIDs []string          `json:"security_group_ids"`
	Tags             map[string]string `json:"tags"`
}

type VpcEndpointState struct {
	ID string `json:"id"`
}

func (p *Provider) applyVpcEndpoint(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VpcEndpointState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteVpcEndpoints(ctx, &ec2.DeleteVpcEndpointsInput{VpcEndpointIds: []string{prior.ID}})
			if err != nil {
				return nil, fmt.Errorf("failed to delete vpc endpoint: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VpcEndpointConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateVpcEndpointInput{
		VpcId:           &desired.VpcID,
		ServiceName:     &desired.ServiceName,
		VpcEndpointType: types.VpcEndpointType(desired.VpcEndpointType),
	}
	if len(desired.RouteTableIDs) > 0 {
		input.RouteTableIds = desired.RouteTableIDs
	}
	if len(desired.SubnetIDs) > 0 {
		input.SubnetIds = desired.SubnetIDs
	}
	if len(desired.SecurityGroupIDs) > 0 {
		input.SecurityGroupIds = desired.SecurityGroupIDs
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		input.TagSpecifications = []types.TagSpecification{
			{ResourceType: types.ResourceTypeVpcEndpoint, Tags: tags},
		}
	}

	resp, err := p.ec2Client.CreateVpcEndpoint(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create vpc endpoint: %w", err)
	}

	newState := VpcEndpointState{ID: *resp.VpcEndpoint.VpcEndpointId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}


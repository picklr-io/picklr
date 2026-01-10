package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type VpcConfig struct {
	CidrBlock string            `json:"cidrBlock"`
	Tags      map[string]string `json:"tags"`
}

type VpcState struct {
	ID        string `json:"id"`
	CidrBlock string `json:"cidrBlock"`
}

type SubnetConfig struct {
	VpcID               string `json:"vpcId"`
	CidrBlock           string `json:"cidrBlock"`
	AvailabilityZone    string `json:"availabilityZone"`
	MapPublicIpOnLaunch bool   `json:"mapPublicIpOnLaunch"`
}

type SubnetState struct {
	ID    string `json:"id"`
	VpcID string `json:"vpcId"`
}

type SecurityGroupConfig struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	VpcID       string              `json:"vpcId"`
	Ingress     []SecurityGroupRule `json:"ingress"`
	Egress      []SecurityGroupRule `json:"egress"`
}

type SecurityGroupRule struct {
	FromPort   int      `json:"fromPort"`
	ToPort     int      `json:"toPort"`
	Protocol   string   `json:"protocol"`
	CidrBlocks []string `json:"cidrBlocks"`
}

type SecurityGroupState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) applyVpc(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VpcState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.ec2Client.DeleteVpc(ctx, &ec2.DeleteVpcInput{VpcId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete VPC: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VpcConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.ec2Client.CreateVpc(ctx, &ec2.CreateVpcInput{
		CidrBlock: &desired.CidrBlock,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create VPC: %w", err)
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		_, _ = p.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{*resp.Vpc.VpcId},
			Tags:      tags,
		})
	}

	newState := VpcState{
		ID:        *resp.Vpc.VpcId,
		CidrBlock: *resp.Vpc.CidrBlock,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applySubnet(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SubnetState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.ec2Client.DeleteSubnet(ctx, &ec2.DeleteSubnetInput{SubnetId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete subnet: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SubnetConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateSubnetInput{
		VpcId:     &desired.VpcID,
		CidrBlock: &desired.CidrBlock,
	}
	if desired.AvailabilityZone != "" {
		input.AvailabilityZone = &desired.AvailabilityZone
	}

	resp, err := p.ec2Client.CreateSubnet(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create subnet: %w", err)
	}

	if desired.MapPublicIpOnLaunch {
		_, _ = p.ec2Client.ModifySubnetAttribute(ctx, &ec2.ModifySubnetAttributeInput{
			SubnetId:            resp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &types.AttributeBooleanValue{Value: func(b bool) *bool { return &b }(true)},
		})
	}

	newState := SubnetState{
		ID:    *resp.Subnet.SubnetId,
		VpcID: *resp.Subnet.VpcId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applySecurityGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SecurityGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			_, err := p.ec2Client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete SG: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SecurityGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateSecurityGroupInput{
		GroupName:   &desired.Name,
		Description: &desired.Description,
	}
	if desired.VpcID != "" {
		input.VpcId = &desired.VpcID
	}

	resp, err := p.ec2Client.CreateSecurityGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create SG: %w", err)
	}
	groupID := *resp.GroupId

	// Ingress
	if len(desired.Ingress) > 0 {
		var perms []types.IpPermission
		for _, rule := range desired.Ingress {
			ipRanges := []types.IpRange{}
			for _, cidr := range rule.CidrBlocks {
				ipRanges = append(ipRanges, types.IpRange{CidrIp: &cidr})
			}
			perms = append(perms, types.IpPermission{
				IpProtocol: &rule.Protocol,
				FromPort:   func(i int32) *int32 { return &i }(int32(rule.FromPort)),
				ToPort:     func(i int32) *int32 { return &i }(int32(rule.ToPort)),
				IpRanges:   ipRanges,
			})
		}
		_, err := p.ec2Client.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId:       &groupID,
			IpPermissions: perms,
		})
		if err != nil {
			// Log error
		}
	}

	newState := SecurityGroupState{
		ID:   groupID,
		Name: desired.Name,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

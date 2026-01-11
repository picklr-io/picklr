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
	VpcID               string            `json:"vpcId"`
	CidrBlock           string            `json:"cidrBlock"`
	AvailabilityZone    string            `json:"availabilityZone"`
	MapPublicIpOnLaunch bool              `json:"mapPublicIpOnLaunch"`
	Tags                map[string]string `json:"tags"`
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

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		_, _ = p.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{
			Resources: []string{*resp.Subnet.SubnetId},
			Tags:      tags,
		})
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

// InternetGateway
type InternetGatewayConfig struct {
	VpcID string            `json:"vpcId"`
	Tags  map[string]string `json:"tags"`
}

type InternetGatewayState struct {
	ID    string `json:"id"`
	VpcID string `json:"vpcId"`
}

func (p *Provider) applyInternetGateway(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior InternetGatewayState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.ID != "" {
			// Detach first
			if prior.VpcID != "" {
				_, _ = p.ec2Client.DetachInternetGateway(ctx, &ec2.DetachInternetGatewayInput{
					InternetGatewayId: &prior.ID,
					VpcId:             &prior.VpcID,
				})
			}
			_, err := p.ec2Client.DeleteInternetGateway(ctx, &ec2.DeleteInternetGatewayInput{InternetGatewayId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete IGW: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired InternetGatewayConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Create
	resp, err := p.ec2Client.CreateInternetGateway(ctx, &ec2.CreateInternetGatewayInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to create IGW: %w", err)
	}
	igwID := *resp.InternetGateway.InternetGatewayId

	// Attach
	if desired.VpcID != "" {
		_, err := p.ec2Client.AttachInternetGateway(ctx, &ec2.AttachInternetGatewayInput{
			InternetGatewayId: &igwID,
			VpcId:             &desired.VpcID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to attach IGW: %w", err)
		}
	}

	// Tags
	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: &k, Value: &v})
		}
		_, _ = p.ec2Client.CreateTags(ctx, &ec2.CreateTagsInput{Resources: []string{igwID}, Tags: tags})
	}

	newState := InternetGatewayState{ID: igwID, VpcID: desired.VpcID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ElasticIP
type ElasticIPConfig struct {
	Tags map[string]string `json:"tags"`
}

type ElasticIPState struct {
	AllocationID string `json:"allocationId"`
	PublicIP     string `json:"publicIp"`
}

func (p *Provider) applyElasticIP(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ElasticIPState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.AllocationID != "" {
			_, err := p.ec2Client.ReleaseAddress(ctx, &ec2.ReleaseAddressInput{AllocationId: &prior.AllocationID})
			if err != nil {
				return nil, fmt.Errorf("failed to release EIP: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ElasticIPConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Check if already allocated (from Prior)
	if req.PriorStateJson != nil {
		// Just tagging? Or NoOp. For simplicity, assume immutable allocation.
		return &pb.ApplyResponse{NewStateJson: req.PriorStateJson}, nil
	}

	resp, err := p.ec2Client.AllocateAddress(ctx, &ec2.AllocateAddressInput{Domain: types.DomainTypeVpc})
	if err != nil {
		return nil, fmt.Errorf("failed to allocate address: %w", err)
	}

	newState := ElasticIPState{AllocationID: *resp.AllocationId, PublicIP: *resp.PublicIp}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// NatGateway
type NatGatewayConfig struct {
	SubnetID     string            `json:"subnetId"`
	AllocationID string            `json:"allocationId"`
	Tags         map[string]string `json:"tags"`
}

type NatGatewayState struct {
	ID string `json:"id"`
}

func (p *Provider) applyNatGateway(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior NatGatewayState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteNatGateway(ctx, &ec2.DeleteNatGatewayInput{NatGatewayId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete NAT GW: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired NatGatewayConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.ec2Client.CreateNatGateway(ctx, &ec2.CreateNatGatewayInput{
		SubnetId:     &desired.SubnetID,
		AllocationId: &desired.AllocationID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create NAT GW: %w", err)
	}

	// WAITER REQUIRED for NAT Gateway to be available? Usually not strictly for creation ID return,
	// but for Routes to reference it, it should exist.
	// We'll return ID immediately.

	newState := NatGatewayState{ID: *resp.NatGateway.NatGatewayId}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// RouteTable
type RouteConfig struct {
	DestinationCidrBlock string  `json:"destinationCidrBlock"`
	GatewayID            *string `json:"gatewayId"`
	NatGatewayID         *string `json:"natGatewayId"`
}

type RouteTableConfig struct {
	VpcID  string            `json:"vpcId"`
	Routes []RouteConfig     `json:"routes"`
	Tags   map[string]string `json:"tags"`
}

type RouteTableState struct {
	ID string `json:"id"`
}

func (p *Provider) applyRouteTable(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RouteTableState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.ID != "" {
			_, err := p.ec2Client.DeleteRouteTable(ctx, &ec2.DeleteRouteTableInput{RouteTableId: &prior.ID})
			if err != nil {
				return nil, fmt.Errorf("failed to delete RT: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RouteTableConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Create
	resp, err := p.ec2Client.CreateRouteTable(ctx, &ec2.CreateRouteTableInput{VpcId: &desired.VpcID})
	if err != nil {
		return nil, fmt.Errorf("failed to create RT: %w", err)
	}
	rtID := *resp.RouteTable.RouteTableId

	// Routes
	for _, route := range desired.Routes {
		input := &ec2.CreateRouteInput{
			RouteTableId:         &rtID,
			DestinationCidrBlock: &route.DestinationCidrBlock,
		}
		if route.GatewayID != nil {
			input.GatewayId = route.GatewayID
		}
		if route.NatGatewayID != nil {
			input.NatGatewayId = route.NatGatewayID
		}
		_, err := p.ec2Client.CreateRoute(ctx, input)
		if err != nil {
			// Log error or fail?
			// Often fails if route already exists (local).
			// Ideally we check if destination is local.
			if route.DestinationCidrBlock != "10.0.0.0/16" { // naive check
				// Just try
			}
		}
	}

	newState := RouteTableState{ID: rtID}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

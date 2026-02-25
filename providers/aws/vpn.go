package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// VPN Gateway

type VpnGatewayConfig struct {
	VpcId            string            `json:"vpc_id"`
	AvailabilityZone string            `json:"availability_zone"`
	VpnType          string            `json:"vpn_type"`
	AmazonSideAsn    *int64            `json:"amazon_side_asn"`
	Tags             map[string]string `json:"tags"`
}

type VpnGatewayState struct {
	VpnGatewayId string `json:"vpnGatewayId"`
}

func (p *Provider) applyVpnGateway(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VpnGatewayState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.VpnGatewayId != "" {
			_, err := p.ec2Client.DeleteVpnGateway(ctx, &ec2.DeleteVpnGatewayInput{
				VpnGatewayId: &prior.VpnGatewayId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete VPN gateway: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VpnGatewayConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateVpnGatewayInput{
		Type: ec2types.GatewayType(desired.VpnType),
	}

	if desired.AvailabilityZone != "" {
		input.AvailabilityZone = &desired.AvailabilityZone
	}
	if desired.AmazonSideAsn != nil {
		input.AmazonSideAsn = desired.AmazonSideAsn
	}

	if len(desired.Tags) > 0 {
		var tags []ec2types.TagSpecification
		var ec2Tags []ec2types.Tag
		for k, v := range desired.Tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		tags = append(tags, ec2types.TagSpecification{
			ResourceType: ec2types.ResourceTypeVpnGateway,
			Tags:         ec2Tags,
		})
		input.TagSpecifications = tags
	}

	resp, err := p.ec2Client.CreateVpnGateway(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN gateway: %w", err)
	}

	vpnGwId := *resp.VpnGateway.VpnGatewayId

	// Attach to VPC if specified
	if desired.VpcId != "" {
		_, err := p.ec2Client.AttachVpnGateway(ctx, &ec2.AttachVpnGatewayInput{
			VpnGatewayId: &vpnGwId,
			VpcId:        &desired.VpcId,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to attach VPN gateway to VPC: %w", err)
		}
	}

	newState := VpnGatewayState{
		VpnGatewayId: vpnGwId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Customer Gateway

type CustomerGatewayConfig struct {
	BgpAsn         int32             `json:"bgp_asn"`
	IpAddress      string            `json:"ip_address"`
	VpnType        string            `json:"vpn_type"`
	DeviceName     string            `json:"device_name"`
	CertificateArn string            `json:"certificate_arn"`
	Tags           map[string]string `json:"tags"`
}

type CustomerGatewayState struct {
	CustomerGatewayId string `json:"customerGatewayId"`
}

func (p *Provider) applyCustomerGateway(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CustomerGatewayState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.CustomerGatewayId != "" {
			_, err := p.ec2Client.DeleteCustomerGateway(ctx, &ec2.DeleteCustomerGatewayInput{
				CustomerGatewayId: &prior.CustomerGatewayId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete customer gateway: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CustomerGatewayConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	bgpAsn := int32(desired.BgpAsn)

	input := &ec2.CreateCustomerGatewayInput{
		BgpAsn:    &bgpAsn,
		IpAddress: &desired.IpAddress,
		Type:      ec2types.GatewayType(desired.VpnType),
	}

	if desired.DeviceName != "" {
		input.DeviceName = &desired.DeviceName
	}
	if desired.CertificateArn != "" {
		input.CertificateArn = &desired.CertificateArn
	}

	if len(desired.Tags) > 0 {
		var ec2Tags []ec2types.Tag
		for k, v := range desired.Tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.TagSpecifications = []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeCustomerGateway,
				Tags:         ec2Tags,
			},
		}
	}

	resp, err := p.ec2Client.CreateCustomerGateway(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create customer gateway: %w", err)
	}

	newState := CustomerGatewayState{
		CustomerGatewayId: *resp.CustomerGateway.CustomerGatewayId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// VPN Connection

type VpnConnectionConfig struct {
	CustomerGatewayId    string            `json:"customer_gateway_id"`
	VpnType              string            `json:"vpn_type"`
	VpnGatewayId         string            `json:"vpn_gateway_id"`
	TransitGatewayId     string            `json:"transit_gateway_id"`
	EnableAcceleration   bool              `json:"enable_acceleration"`
	StaticRoutesOnly     bool              `json:"static_routes_only"`
	LocalIpv4NetworkCidr string            `json:"local_ipv4_network_cidr"`
	RemoteIpv4NetworkCidr string           `json:"remote_ipv4_network_cidr"`
	Tags                 map[string]string `json:"tags"`
}

type VpnConnectionState struct {
	VpnConnectionId string `json:"vpnConnectionId"`
}

func (p *Provider) applyVpnConnection(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior VpnConnectionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.VpnConnectionId != "" {
			_, err := p.ec2Client.DeleteVpnConnection(ctx, &ec2.DeleteVpnConnectionInput{
				VpnConnectionId: &prior.VpnConnectionId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete VPN connection: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired VpnConnectionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ec2.CreateVpnConnectionInput{
		CustomerGatewayId: &desired.CustomerGatewayId,
		Type:              &desired.VpnType,
	}

	if desired.VpnGatewayId != "" {
		input.VpnGatewayId = &desired.VpnGatewayId
	}
	if desired.TransitGatewayId != "" {
		input.TransitGatewayId = &desired.TransitGatewayId
	}

	options := &ec2types.VpnConnectionOptionsSpecification{
		EnableAcceleration: &desired.EnableAcceleration,
		StaticRoutesOnly:   &desired.StaticRoutesOnly,
	}
	if desired.LocalIpv4NetworkCidr != "" {
		options.LocalIpv4NetworkCidr = &desired.LocalIpv4NetworkCidr
	}
	if desired.RemoteIpv4NetworkCidr != "" {
		options.RemoteIpv4NetworkCidr = &desired.RemoteIpv4NetworkCidr
	}
	input.Options = options

	if len(desired.Tags) > 0 {
		var ec2Tags []ec2types.Tag
		for k, v := range desired.Tags {
			ec2Tags = append(ec2Tags, ec2types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.TagSpecifications = []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVpnConnection,
				Tags:         ec2Tags,
			},
		}
	}

	resp, err := p.ec2Client.CreateVpnConnection(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPN connection: %w", err)
	}

	newState := VpnConnectionState{
		VpnConnectionId: *resp.VpnConnection.VpnConnectionId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

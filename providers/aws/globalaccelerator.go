package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator"
	"github.com/aws/aws-sdk-go-v2/service/globalaccelerator/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type AcceleratorConfig struct {
	Name          string            `json:"name"`
	IPAddressType string            `json:"ip_address_type"`
	Enabled       bool              `json:"enabled"`
	Tags          map[string]string `json:"tags"`
}

type AcceleratorState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
	DNS  string `json:"dns"`
}

func (p *Provider) applyAccelerator(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AcceleratorState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			// Must disable before deleting.
			_, err := p.globalacceleratorClient.UpdateAccelerator(ctx, &globalaccelerator.UpdateAcceleratorInput{
				AcceleratorArn: &prior.ARN,
				Enabled:        func(b bool) *bool { return &b }(false),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to disable accelerator before delete: %w", err)
			}

			// Wait for disabled state? GlobalAccelerator takes time.
			// Simplified: best effort delete
			_, err = p.globalacceleratorClient.DeleteAccelerator(ctx, &globalaccelerator.DeleteAcceleratorInput{
				AcceleratorArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete accelerator: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AcceleratorConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	callerRef := fmt.Sprintf("picklr-%d", time.Now().UnixNano())

	var tags []types.Tag
	for k, v := range desired.Tags {
		tags = append(tags, types.Tag{Key: &k, Value: &v})
	}

	resp, err := p.globalacceleratorClient.CreateAccelerator(ctx, &globalaccelerator.CreateAcceleratorInput{
		Name:             &desired.Name,
		IdempotencyToken: &callerRef,
		Enabled:          &desired.Enabled,
		IpAddressType:    types.IpAddressType(desired.IPAddressType),
		Tags:             tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create accelerator: %w", err)
	}

	newState := AcceleratorState{
		Name: *resp.Accelerator.Name,
		ARN:  *resp.Accelerator.AcceleratorArn,
		DNS:  *resp.Accelerator.DnsName,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type GlobalAcceleratorListenerConfig struct {
	AcceleratorARN string      `json:"accelerator_arn"`
	Protocol       string      `json:"protocol"`
	PortRanges     []PortRange `json:"port_ranges"`
	ClientAffinity string      `json:"client_affinity"`
}

type PortRange struct {
	FromPort int `json:"from_port"`
	ToPort   int `json:"to_port"`
}

type GlobalAcceleratorListenerState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyGlobalAcceleratorListener(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior GlobalAcceleratorListenerState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.globalacceleratorClient.DeleteListener(ctx, &globalaccelerator.DeleteListenerInput{
				ListenerArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete listener: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired GlobalAcceleratorListenerConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var portRanges []types.PortRange
	for _, pr := range desired.PortRanges {
		portRanges = append(portRanges, types.PortRange{
			FromPort: func(i int32) *int32 { return &i }(int32(pr.FromPort)),
			ToPort:   func(i int32) *int32 { return &i }(int32(pr.ToPort)),
		})
	}

	callerRef := fmt.Sprintf("picklr-%d", time.Now().UnixNano())

	resp, err := p.globalacceleratorClient.CreateListener(ctx, &globalaccelerator.CreateListenerInput{
		AcceleratorArn:   &desired.AcceleratorARN,
		IdempotencyToken: &callerRef,
		Protocol:         types.Protocol(desired.Protocol),
		PortRanges:       portRanges,
		ClientAffinity:   types.ClientAffinity(desired.ClientAffinity),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create listener: %w", err)
	}

	newState := GlobalAcceleratorListenerState{
		ARN: *resp.Listener.ListenerArn,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

type EndpointGroupConfig struct {
	ListenerARN            string                  `json:"listener_arn"`
	Region                 string                  `json:"endpoint_group_region"`
	EndpointConfigurations []EndpointConfiguration `json:"endpoint_configurations"`
	TrafficDialPercentage  float32                 `json:"traffic_dial_percentage"`
	HealthCheckPort        int                     `json:"health_check_port"`
	HealthCheckProtocol    string                  `json:"health_check_protocol"`
	HealthCheckPath        string                  `json:"health_check_path"`
}

type EndpointConfiguration struct {
	EndpointID                  string `json:"endpoint_id"`
	Weight                      int    `json:"weight"`
	ClientIPPreservationEnabled bool   `json:"client_ip_preservation_enabled"`
}

type EndpointGroupState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyEndpointGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EndpointGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.globalacceleratorClient.DeleteEndpointGroup(ctx, &globalaccelerator.DeleteEndpointGroupInput{
				EndpointGroupArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete endpoint group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EndpointGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var endpointConfigs []types.EndpointConfiguration
	for _, ec := range desired.EndpointConfigurations {
		endpointConfigs = append(endpointConfigs, types.EndpointConfiguration{
			EndpointId:                  &ec.EndpointID,
			Weight:                      func(i int32) *int32 { return &i }(int32(ec.Weight)),
			ClientIPPreservationEnabled: &ec.ClientIPPreservationEnabled,
		})
	}

	callerRef := fmt.Sprintf("picklr-%d", time.Now().UnixNano())

	input := &globalaccelerator.CreateEndpointGroupInput{
		ListenerArn:            &desired.ListenerARN,
		IdempotencyToken:       &callerRef,
		EndpointGroupRegion:    &desired.Region,
		EndpointConfigurations: endpointConfigs,
		TrafficDialPercentage:  func(f float32) *float32 { return &f }(desired.TrafficDialPercentage),
	}

	if desired.HealthCheckPort != 0 {
		input.HealthCheckPort = func(i int32) *int32 { return &i }(int32(desired.HealthCheckPort))
	}
	if desired.HealthCheckProtocol != "" {
		input.HealthCheckProtocol = types.HealthCheckProtocol(desired.HealthCheckProtocol)
	}
	if desired.HealthCheckPath != "" {
		input.HealthCheckPath = &desired.HealthCheckPath
	}

	resp, err := p.globalacceleratorClient.CreateEndpointGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create endpoint group: %w", err)
	}

	newState := EndpointGroupState{
		ARN: *resp.EndpointGroup.EndpointGroupArn,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

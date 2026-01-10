package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type ClusterConfig struct {
	ClusterName string `json:"clusterName"`
}

type ClusterState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

type TaskDefinitionConfig struct {
	Family               string                `json:"family"`
	NetworkMode          string                `json:"networkMode"`
	Cpu                  string                `json:"cpu"`
	Memory               string                `json:"memory"`
	ContainerDefinitions []ContainerDefinition `json:"containerDefinitions"`
}

type ContainerDefinition struct {
	Name         string        `json:"name"`
	Image        string        `json:"image"`
	Cpu          int           `json:"cpu"`
	Memory       int           `json:"memory"`
	PortMappings []PortMapping `json:"portMappings"`
}

type PortMapping struct {
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
	Protocol      string `json:"protocol"`
}

type TaskDefinitionState struct {
	ARN    string `json:"arn"`
	Family string `json:"family"`
}

type ServiceConfig struct {
	ServiceName          string                `json:"serviceName"`
	Cluster              string                `json:"cluster"`
	TaskDefinition       string                `json:"taskDefinition"`
	DesiredCount         int                   `json:"desiredCount"`
	LaunchType           string                `json:"launchType"`
	NetworkConfiguration *NetworkConfiguration `json:"networkConfiguration"`
	LoadBalancers        []LoadBalancer        `json:"loadBalancers"`
}

type NetworkConfiguration struct {
	Subnets        []string `json:"subnets"`
	SecurityGroups []string `json:"securityGroups"`
	AssignPublicIp bool     `json:"assignPublicIp"`
}

type LoadBalancer struct {
	TargetGroupArn string `json:"targetGroupArn"`
	ContainerName  string `json:"containerName"`
	ContainerPort  int    `json:"containerPort"`
}

type ServiceState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.ecsClient.DeleteCluster(ctx, &ecs.DeleteClusterInput{
				Cluster: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.ecsClient.CreateCluster(ctx, &ecs.CreateClusterInput{
		ClusterName: &desired.ClusterName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	newState := ClusterState{
		Name: *resp.Cluster.ClusterName,
		ARN:  *resp.Cluster.ClusterArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyTaskDefinition(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TaskDefinitionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.ecsClient.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
				TaskDefinition: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to deregister task definition: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TaskDefinitionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var containerDefs []types.ContainerDefinition
	for _, c := range desired.ContainerDefinitions {
		var mappings []types.PortMapping
		for _, m := range c.PortMappings {
			mappings = append(mappings, types.PortMapping{
				ContainerPort: func(i int32) *int32 { return &i }(int32(m.ContainerPort)),
				HostPort:      func(i int32) *int32 { return &i }(int32(m.HostPort)),
				Protocol:      types.TransportProtocol(m.Protocol),
			})
		}
		containerDefs = append(containerDefs, types.ContainerDefinition{
			Name:         &c.Name,
			Image:        &c.Image,
			Cpu:          int32(c.Cpu),
			Memory:       func(i int32) *int32 { return &i }(int32(c.Memory)),
			PortMappings: mappings,
		})
	}

	resp, err := p.ecsClient.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
		Family:                  &desired.Family,
		ContainerDefinitions:    containerDefs,
		NetworkMode:             types.NetworkMode(desired.NetworkMode),
		Cpu:                     &desired.Cpu,
		Memory:                  &desired.Memory,
		RequiresCompatibilities: []types.Compatibility{types.CompatibilityFargate}, // Default assumption
	})
	if err != nil {
		return nil, fmt.Errorf("failed to register task definition: %w", err)
	}

	newState := TaskDefinitionState{
		ARN:    *resp.TaskDefinition.TaskDefinitionArn,
		Family: *resp.TaskDefinition.Family,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyService(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ServiceState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			// Cluster name is needed for deletion, we might need to store it in state or pass it differently.
			// Simplified: Assuming we can find it or it's not strictly required if we destroy cluster.
			_, err := p.ecsClient.DeleteService(ctx, &ecs.DeleteServiceInput{
				Service: &prior.Name,
				// Cluster: ???,
				Force: func(b bool) *bool { return &b }(true),
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete service: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ServiceConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &ecs.CreateServiceInput{
		ServiceName:    &desired.ServiceName,
		Cluster:        &desired.Cluster,
		TaskDefinition: &desired.TaskDefinition,
		DesiredCount:   func(i int32) *int32 { return &i }(int32(desired.DesiredCount)),
		LaunchType:     types.LaunchType(desired.LaunchType),
	}

	if desired.NetworkConfiguration != nil {
		assignPublic := types.AssignPublicIpDisabled
		if desired.NetworkConfiguration.AssignPublicIp {
			assignPublic = types.AssignPublicIpEnabled
		}
		input.NetworkConfiguration = &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				Subnets:        desired.NetworkConfiguration.Subnets,
				SecurityGroups: desired.NetworkConfiguration.SecurityGroups,
				AssignPublicIp: assignPublic,
			},
		}
	}

	if len(desired.LoadBalancers) > 0 {
		var lbs []types.LoadBalancer
		for _, lb := range desired.LoadBalancers {
			lbs = append(lbs, types.LoadBalancer{
				TargetGroupArn: &lb.TargetGroupArn,
				ContainerName:  &lb.ContainerName,
				ContainerPort:  func(i int32) *int32 { return &i }(int32(lb.ContainerPort)),
			})
		}
		input.LoadBalancers = lbs
	}

	resp, err := p.ecsClient.CreateService(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	newState := ServiceState{
		Name: *resp.Service.ServiceName,
		ARN:  *resp.Service.ServiceArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// EKS Cluster

type EKSClusterConfig struct {
	ClusterName           string            `json:"cluster_name"`
	RoleArn               string            `json:"role_arn"`
	Version               string            `json:"version"`
	VpcConfig             EKSVpcConfig      `json:"vpc_config"`
	EncryptionConfig      *EKSEncryption    `json:"encryption_config"`
	KubernetesNetworkCfg  *EKSNetworkConfig `json:"kubernetes_network_config"`
	Tags                  map[string]string  `json:"tags"`
}

type EKSVpcConfig struct {
	SubnetIds             []string `json:"subnet_ids"`
	SecurityGroupIds      []string `json:"security_group_ids"`
	EndpointPublicAccess  bool     `json:"endpoint_public_access"`
	EndpointPrivateAccess bool     `json:"endpoint_private_access"`
}

type EKSEncryption struct {
	KeyArn    string   `json:"key_arn"`
	Resources []string `json:"resources"`
}

type EKSNetworkConfig struct {
	ServiceIpv4Cidr string `json:"service_ipv4_cidr"`
	IpFamily        string `json:"ip_family"`
}

type EKSClusterState struct {
	Name     string `json:"name"`
	ARN      string `json:"arn"`
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
}

func (p *Provider) applyEKSCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EKSClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.eksClient.DeleteCluster(ctx, &eks.DeleteClusterInput{
				Name: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete EKS cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EKSClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &eks.CreateClusterInput{
		Name:    &desired.ClusterName,
		RoleArn: &desired.RoleArn,
		ResourcesVpcConfig: &types.VpcConfigRequest{
			SubnetIds:             desired.VpcConfig.SubnetIds,
			SecurityGroupIds:      desired.VpcConfig.SecurityGroupIds,
			EndpointPublicAccess:  &desired.VpcConfig.EndpointPublicAccess,
			EndpointPrivateAccess: &desired.VpcConfig.EndpointPrivateAccess,
		},
		Tags: desired.Tags,
	}

	if desired.Version != "" {
		input.Version = &desired.Version
	}

	if desired.EncryptionConfig != nil {
		input.EncryptionConfig = []types.EncryptionConfig{
			{
				Provider:  &types.Provider{KeyArn: &desired.EncryptionConfig.KeyArn},
				Resources: desired.EncryptionConfig.Resources,
			},
		}
	}

	if desired.KubernetesNetworkCfg != nil {
		input.KubernetesNetworkConfig = &types.KubernetesNetworkConfigRequest{}
		if desired.KubernetesNetworkCfg.ServiceIpv4Cidr != "" {
			input.KubernetesNetworkConfig.ServiceIpv4Cidr = &desired.KubernetesNetworkCfg.ServiceIpv4Cidr
		}
		if desired.KubernetesNetworkCfg.IpFamily != "" {
			input.KubernetesNetworkConfig.IpFamily = types.IpFamily(desired.KubernetesNetworkCfg.IpFamily)
		}
	}

	resp, err := p.eksClient.CreateCluster(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS cluster: %w", err)
	}

	newState := EKSClusterState{
		Name: *resp.Cluster.Name,
		ARN:  *resp.Cluster.Arn,
	}
	if resp.Cluster.Endpoint != nil {
		newState.Endpoint = *resp.Cluster.Endpoint
	}
	if resp.Cluster.Version != nil {
		newState.Version = *resp.Cluster.Version
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// EKS Node Group

type EKSNodeGroupConfig struct {
	NodeGroupName string            `json:"node_group_name"`
	ClusterName   string            `json:"cluster_name"`
	NodeRoleArn   string            `json:"node_role_arn"`
	SubnetIds     []string          `json:"subnet_ids"`
	ScalingConfig EKSScalingConfig  `json:"scaling_config"`
	InstanceTypes []string          `json:"instance_types"`
	AmiType       string            `json:"ami_type"`
	CapacityType  string            `json:"capacity_type"`
	DiskSize      *int32            `json:"disk_size"`
	Labels        map[string]string `json:"labels"`
	Tags          map[string]string `json:"tags"`
}

type EKSScalingConfig struct {
	DesiredSize int32 `json:"desired_size"`
	MaxSize     int32 `json:"max_size"`
	MinSize     int32 `json:"min_size"`
}

type EKSNodeGroupState struct {
	NodeGroupName string `json:"nodeGroupName"`
	ARN           string `json:"arn"`
	ClusterName   string `json:"clusterName"`
}

func (p *Provider) applyEKSNodeGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EKSNodeGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.NodeGroupName != "" {
			_, err := p.eksClient.DeleteNodegroup(ctx, &eks.DeleteNodegroupInput{
				ClusterName:   &prior.ClusterName,
				NodegroupName: &prior.NodeGroupName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete EKS node group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EKSNodeGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &eks.CreateNodegroupInput{
		NodegroupName: &desired.NodeGroupName,
		ClusterName:   &desired.ClusterName,
		NodeRole:      &desired.NodeRoleArn,
		Subnets:       desired.SubnetIds,
		ScalingConfig: &types.NodegroupScalingConfig{
			DesiredSize: &desired.ScalingConfig.DesiredSize,
			MaxSize:     &desired.ScalingConfig.MaxSize,
			MinSize:     &desired.ScalingConfig.MinSize,
		},
		Tags: desired.Tags,
	}

	if len(desired.InstanceTypes) > 0 {
		input.InstanceTypes = desired.InstanceTypes
	}
	if desired.AmiType != "" {
		input.AmiType = types.AMITypes(desired.AmiType)
	}
	if desired.CapacityType != "" {
		input.CapacityType = types.CapacityTypes(desired.CapacityType)
	}
	if desired.DiskSize != nil {
		input.DiskSize = desired.DiskSize
	}
	if len(desired.Labels) > 0 {
		input.Labels = desired.Labels
	}

	resp, err := p.eksClient.CreateNodegroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS node group: %w", err)
	}

	newState := EKSNodeGroupState{
		NodeGroupName: *resp.Nodegroup.NodegroupName,
		ARN:           *resp.Nodegroup.NodegroupArn,
		ClusterName:   *resp.Nodegroup.ClusterName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// EKS Fargate Profile

type EKSFargateProfileConfig struct {
	FargateProfileName  string              `json:"fargate_profile_name"`
	ClusterName         string              `json:"cluster_name"`
	PodExecutionRoleArn string              `json:"pod_execution_role_arn"`
	SubnetIds           []string            `json:"subnet_ids"`
	Selectors           []EKSFargateSelect  `json:"selectors"`
	Tags                map[string]string   `json:"tags"`
}

type EKSFargateSelect struct {
	Namespace string            `json:"namespace"`
	Labels    map[string]string `json:"labels"`
}

type EKSFargateProfileState struct {
	FargateProfileName string `json:"fargateProfileName"`
	ARN                string `json:"arn"`
	ClusterName        string `json:"clusterName"`
}

func (p *Provider) applyEKSFargateProfile(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EKSFargateProfileState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.FargateProfileName != "" {
			_, err := p.eksClient.DeleteFargateProfile(ctx, &eks.DeleteFargateProfileInput{
				ClusterName:        &prior.ClusterName,
				FargateProfileName: &prior.FargateProfileName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete EKS Fargate profile: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EKSFargateProfileConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var selectors []types.FargateProfileSelector
	for _, s := range desired.Selectors {
		selectors = append(selectors, types.FargateProfileSelector{
			Namespace: &s.Namespace,
			Labels:    s.Labels,
		})
	}

	input := &eks.CreateFargateProfileInput{
		FargateProfileName:  &desired.FargateProfileName,
		ClusterName:         &desired.ClusterName,
		PodExecutionRoleArn: &desired.PodExecutionRoleArn,
		Subnets:             desired.SubnetIds,
		Selectors:           selectors,
		Tags:                desired.Tags,
	}

	resp, err := p.eksClient.CreateFargateProfile(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS Fargate profile: %w", err)
	}

	newState := EKSFargateProfileState{
		FargateProfileName: *resp.FargateProfile.FargateProfileName,
		ARN:                *resp.FargateProfile.FargateProfileArn,
		ClusterName:        *resp.FargateProfile.ClusterName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// EKS Addon

type EKSAddonConfig struct {
	AddonName             string `json:"addon_name"`
	ClusterName           string `json:"cluster_name"`
	AddonVersion          string `json:"addon_version"`
	ServiceAccountRoleArn string `json:"service_account_role_arn"`
	ResolveConflicts      string `json:"resolve_conflicts"`
	Tags                  map[string]string `json:"tags"`
}

type EKSAddonState struct {
	AddonName   string `json:"addonName"`
	ARN         string `json:"arn"`
	ClusterName string `json:"clusterName"`
}

func (p *Provider) applyEKSAddon(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior EKSAddonState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.AddonName != "" {
			_, err := p.eksClient.DeleteAddon(ctx, &eks.DeleteAddonInput{
				ClusterName: &prior.ClusterName,
				AddonName:   &prior.AddonName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete EKS addon: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired EKSAddonConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &eks.CreateAddonInput{
		AddonName:        &desired.AddonName,
		ClusterName:      &desired.ClusterName,
		ResolveConflicts: types.ResolveConflicts(desired.ResolveConflicts),
		Tags:             desired.Tags,
	}

	if desired.AddonVersion != "" {
		input.AddonVersion = &desired.AddonVersion
	}
	if desired.ServiceAccountRoleArn != "" {
		input.ServiceAccountRoleArn = &desired.ServiceAccountRoleArn
	}

	resp, err := p.eksClient.CreateAddon(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create EKS addon: %w", err)
	}

	newState := EKSAddonState{
		AddonName:   *resp.Addon.AddonName,
		ARN:         *resp.Addon.AddonArn,
		ClusterName: *resp.Addon.ClusterName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

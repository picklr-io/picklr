package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/elasticache/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// ElastiCache Replication Group

type ReplicationGroupConfig struct {
	ReplicationGroupId          string   `json:"replication_group_id"`
	ReplicationGroupDescription string   `json:"replication_group_description"`
	NodeType                    string   `json:"node_type"`
	NumCacheClusters            int32    `json:"num_cache_clusters"`
	Port                        int32    `json:"port"`
	ParameterGroupName          string   `json:"parameter_group_name"`
	SubnetGroupName             string   `json:"subnet_group_name"`
	SecurityGroupIds            []string `json:"security_group_ids"`
	AutomaticFailoverEnabled    bool     `json:"automatic_failover_enabled"`
	MultiAzEnabled              bool     `json:"multi_az_enabled"`
	AtRestEncryptionEnabled     bool     `json:"at_rest_encryption_enabled"`
	TransitEncryptionEnabled    bool     `json:"transit_encryption_enabled"`
	EngineVersion               string   `json:"engine_version"`
	SnapshotRetentionLimit      *int32   `json:"snapshot_retention_limit"`
	SnapshotWindow              string   `json:"snapshot_window"`
	Tags                        map[string]string `json:"tags"`
}

type ReplicationGroupState struct {
	ReplicationGroupId string `json:"replicationGroupId"`
	ARN                string `json:"arn"`
	PrimaryEndpoint    string `json:"primaryEndpoint"`
}

func (p *Provider) applyReplicationGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ReplicationGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ReplicationGroupId != "" {
			_, err := p.elasticacheClient.DeleteReplicationGroup(ctx, &elasticache.DeleteReplicationGroupInput{
				ReplicationGroupId: &prior.ReplicationGroupId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete replication group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ReplicationGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &elasticache.CreateReplicationGroupInput{
		ReplicationGroupId:          &desired.ReplicationGroupId,
		ReplicationGroupDescription: &desired.ReplicationGroupDescription,
		CacheNodeType:               &desired.NodeType,
		NumCacheClusters:            &desired.NumCacheClusters,
		Port:                        &desired.Port,
		AutomaticFailoverEnabled:    &desired.AutomaticFailoverEnabled,
		MultiAZEnabled:              &desired.MultiAzEnabled,
		AtRestEncryptionEnabled:     &desired.AtRestEncryptionEnabled,
		TransitEncryptionEnabled:    &desired.TransitEncryptionEnabled,
	}

	if desired.ParameterGroupName != "" {
		input.CacheParameterGroupName = &desired.ParameterGroupName
	}
	if desired.SubnetGroupName != "" {
		input.CacheSubnetGroupName = &desired.SubnetGroupName
	}
	if len(desired.SecurityGroupIds) > 0 {
		input.SecurityGroupIds = desired.SecurityGroupIds
	}
	if desired.EngineVersion != "" {
		input.EngineVersion = &desired.EngineVersion
	}
	if desired.SnapshotRetentionLimit != nil {
		input.SnapshotRetentionLimit = desired.SnapshotRetentionLimit
	}
	if desired.SnapshotWindow != "" {
		input.SnapshotWindow = &desired.SnapshotWindow
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.elasticacheClient.CreateReplicationGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create replication group: %w", err)
	}

	newState := ReplicationGroupState{
		ReplicationGroupId: *resp.ReplicationGroup.ReplicationGroupId,
	}
	if resp.ReplicationGroup.ARN != nil {
		newState.ARN = *resp.ReplicationGroup.ARN
	}
	if resp.ReplicationGroup.NodeGroups != nil && len(resp.ReplicationGroup.NodeGroups) > 0 {
		if resp.ReplicationGroup.NodeGroups[0].PrimaryEndpoint != nil {
			newState.PrimaryEndpoint = *resp.ReplicationGroup.NodeGroups[0].PrimaryEndpoint.Address
		}
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ElastiCache Cluster

type CacheClusterConfig struct {
	ClusterId          string   `json:"cluster_id"`
	Engine             string   `json:"engine"`
	NodeType           string   `json:"node_type"`
	NumCacheNodes      int32    `json:"num_cache_nodes"`
	Port               int32    `json:"port"`
	ParameterGroupName string   `json:"parameter_group_name"`
	SubnetGroupName    string   `json:"subnet_group_name"`
	SecurityGroupIds   []string `json:"security_group_ids"`
	EngineVersion      string   `json:"engine_version"`
	AvailabilityZone   string   `json:"availability_zone"`
	Tags               map[string]string `json:"tags"`
}

type CacheClusterState struct {
	ClusterId        string `json:"clusterId"`
	ARN              string `json:"arn"`
	Endpoint         string `json:"endpoint"`
	ConfigEndpoint   string `json:"configEndpoint"`
}

func (p *Provider) applyCacheCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CacheClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ClusterId != "" {
			_, err := p.elasticacheClient.DeleteCacheCluster(ctx, &elasticache.DeleteCacheClusterInput{
				CacheClusterId: &prior.ClusterId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cache cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CacheClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &elasticache.CreateCacheClusterInput{
		CacheClusterId: &desired.ClusterId,
		Engine:         &desired.Engine,
		CacheNodeType:  &desired.NodeType,
		NumCacheNodes:  &desired.NumCacheNodes,
		Port:           &desired.Port,
	}

	if desired.ParameterGroupName != "" {
		input.CacheParameterGroupName = &desired.ParameterGroupName
	}
	if desired.SubnetGroupName != "" {
		input.CacheSubnetGroupName = &desired.SubnetGroupName
	}
	if len(desired.SecurityGroupIds) > 0 {
		input.SecurityGroupIds = desired.SecurityGroupIds
	}
	if desired.EngineVersion != "" {
		input.EngineVersion = &desired.EngineVersion
	}
	if desired.AvailabilityZone != "" {
		input.PreferredAvailabilityZone = &desired.AvailabilityZone
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.elasticacheClient.CreateCacheCluster(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache cluster: %w", err)
	}

	newState := CacheClusterState{
		ClusterId: *resp.CacheCluster.CacheClusterId,
	}
	if resp.CacheCluster.ARN != nil {
		newState.ARN = *resp.CacheCluster.ARN
	}
	if resp.CacheCluster.ConfigurationEndpoint != nil {
		newState.ConfigEndpoint = *resp.CacheCluster.ConfigurationEndpoint.Address
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ElastiCache Subnet Group

type CacheSubnetGroupConfig struct {
	SubnetGroupName string   `json:"subnet_group_name"`
	Description     string   `json:"description"`
	SubnetIds       []string `json:"subnet_ids"`
	Tags            map[string]string `json:"tags"`
}

type CacheSubnetGroupState struct {
	SubnetGroupName string `json:"subnetGroupName"`
	ARN             string `json:"arn"`
}

func (p *Provider) applyCacheSubnetGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CacheSubnetGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.SubnetGroupName != "" {
			_, err := p.elasticacheClient.DeleteCacheSubnetGroup(ctx, &elasticache.DeleteCacheSubnetGroupInput{
				CacheSubnetGroupName: &prior.SubnetGroupName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cache subnet group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CacheSubnetGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &elasticache.CreateCacheSubnetGroupInput{
		CacheSubnetGroupName:        &desired.SubnetGroupName,
		CacheSubnetGroupDescription: &desired.Description,
		SubnetIds:                   desired.SubnetIds,
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.elasticacheClient.CreateCacheSubnetGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache subnet group: %w", err)
	}

	newState := CacheSubnetGroupState{
		SubnetGroupName: *resp.CacheSubnetGroup.CacheSubnetGroupName,
	}
	if resp.CacheSubnetGroup.ARN != nil {
		newState.ARN = *resp.CacheSubnetGroup.ARN
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// ElastiCache Parameter Group

type CacheParameterGroupConfig struct {
	ParameterGroupName string              `json:"parameter_group_name"`
	Family             string              `json:"family"`
	Description        string              `json:"description"`
	Parameters         []CacheParamEntry   `json:"parameters"`
	Tags               map[string]string   `json:"tags"`
}

type CacheParamEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type CacheParameterGroupState struct {
	ParameterGroupName string `json:"parameterGroupName"`
	ARN                string `json:"arn"`
}

func (p *Provider) applyCacheParameterGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior CacheParameterGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ParameterGroupName != "" {
			_, err := p.elasticacheClient.DeleteCacheParameterGroup(ctx, &elasticache.DeleteCacheParameterGroupInput{
				CacheParameterGroupName: &prior.ParameterGroupName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cache parameter group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired CacheParameterGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	desc := desired.Description
	if desc == "" {
		desc = desired.ParameterGroupName
	}

	input := &elasticache.CreateCacheParameterGroupInput{
		CacheParameterGroupName:   &desired.ParameterGroupName,
		CacheParameterGroupFamily: &desired.Family,
		Description:               &desc,
	}

	if len(desired.Tags) > 0 {
		var tags []types.Tag
		for k, v := range desired.Tags {
			tags = append(tags, types.Tag{Key: strPtr(k), Value: strPtr(v)})
		}
		input.Tags = tags
	}

	resp, err := p.elasticacheClient.CreateCacheParameterGroup(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache parameter group: %w", err)
	}

	// Apply parameters if any
	if len(desired.Parameters) > 0 {
		var params []types.ParameterNameValue
		for _, param := range desired.Parameters {
			params = append(params, types.ParameterNameValue{
				ParameterName:  strPtr(param.Name),
				ParameterValue: strPtr(param.Value),
			})
		}
		_, err := p.elasticacheClient.ModifyCacheParameterGroup(ctx, &elasticache.ModifyCacheParameterGroupInput{
			CacheParameterGroupName: &desired.ParameterGroupName,
			ParameterNameValues:     params,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to modify cache parameter group: %w", err)
		}
	}

	newState := CacheParameterGroupState{
		ParameterGroupName: *resp.CacheParameterGroup.CacheParameterGroupName,
	}
	if resp.CacheParameterGroup.ARN != nil {
		newState.ARN = *resp.CacheParameterGroup.ARN
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func strPtr(s string) *string {
	return &s
}

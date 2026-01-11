package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/kafka"
	"github.com/aws/aws-sdk-go-v2/service/kafka/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type MSKClusterConfig struct {
	ClusterName         string                 `json:"cluster_name"`
	NumberOfBrokerNodes int32                  `json:"number_of_broker_nodes"`
	KafkaVersion        string                 `json:"kafka_version"`
	BrokerNodeGroupInfo map[string]interface{} `json:"broker_node_group_info"`
	Tags                map[string]string      `json:"tags"`
}

type MSKClusterState struct {
	ARN  string `json:"arn"`
	Name string `json:"name"`
}

func (p *Provider) applyMSKCluster(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior MSKClusterState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.kafkaClient.DeleteCluster(ctx, &kafka.DeleteClusterInput{
				ClusterArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete cluster: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired MSKClusterConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	// Convert BrokerNodeGroupInfo map to struct
	// Simplified: Assuming simple structure or specific fields
	// Need types.BrokerNodeGroupInfo
	var brokerInfo types.BrokerNodeGroupInfo
	if clientSubnets, ok := desired.BrokerNodeGroupInfo["ClientSubnets"].([]interface{}); ok {
		for _, s := range clientSubnets {
			brokerInfo.ClientSubnets = append(brokerInfo.ClientSubnets, s.(string))
		}
	}
	if instanceType, ok := desired.BrokerNodeGroupInfo["InstanceType"].(string); ok {
		brokerInfo.InstanceType = &instanceType
	}
	if securityGroups, ok := desired.BrokerNodeGroupInfo["SecurityGroups"].([]interface{}); ok {
		for _, s := range securityGroups {
			brokerInfo.SecurityGroups = append(brokerInfo.SecurityGroups, s.(string))
		}
	}

	resp, err := p.kafkaClient.CreateCluster(ctx, &kafka.CreateClusterInput{
		ClusterName:         &desired.ClusterName,
		BrokerNodeGroupInfo: &brokerInfo,
		KafkaVersion:        &desired.KafkaVersion,
		NumberOfBrokerNodes: &desired.NumberOfBrokerNodes,
		Tags:                desired.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create msk cluster: %w", err)
	}

	newState := MSKClusterState{
		ARN:  *resp.ClusterArn,
		Name: *resp.ClusterName,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

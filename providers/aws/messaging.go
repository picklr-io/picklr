package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type QueueConfig struct {
	QueueName              string `json:"queueName"`
	VisibilityTimeout      int    `json:"visibilityTimeout"`
	MessageRetentionPeriod int    `json:"messageRetentionPeriod"`
}

type QueueState struct {
	URL string `json:"url"`
	ARN string `json:"arn"` // Need to fetch attributes to get ARN
}

type TopicConfig struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

type TopicState struct {
	ARN string `json:"arn"`
}

type SubscriptionConfig struct {
	TopicArn string `json:"topicArn"`
	Protocol string `json:"protocol"`
	Endpoint string `json:"endpoint"`
}

type SubscriptionState struct {
	ARN string `json:"arn"`
}

func (p *Provider) applyQueue(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior QueueState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.URL != "" {
			_, err := p.sqsClient.DeleteQueue(ctx, &sqs.DeleteQueueInput{
				QueueUrl: &prior.URL,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete queue: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired QueueConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	attrs := map[string]string{
		"VisibilityTimeout":      fmt.Sprintf("%d", desired.VisibilityTimeout),
		"MessageRetentionPeriod": fmt.Sprintf("%d", desired.MessageRetentionPeriod),
	}

	resp, err := p.sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  &desired.QueueName,
		Attributes: attrs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create queue: %w", err)
	}

	newState := QueueState{
		URL: *resp.QueueUrl,
		// Fetch ARN attribute if needed
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyTopic(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TopicState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.snsClient.DeleteTopic(ctx, &sns.DeleteTopicInput{
				TopicArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete topic: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TopicConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &sns.CreateTopicInput{
		Name:       &desired.Name,
		Attributes: make(map[string]string),
	}
	if desired.DisplayName != "" {
		input.Attributes["DisplayName"] = desired.DisplayName
	}

	resp, err := p.snsClient.CreateTopic(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create topic: %w", err)
	}

	newState := TopicState{
		ARN: *resp.TopicArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applySubscription(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior SubscriptionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ARN != "" {
			_, err := p.snsClient.Unsubscribe(ctx, &sns.UnsubscribeInput{
				SubscriptionArn: &prior.ARN,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to unsubscribe: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired SubscriptionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.snsClient.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: &desired.TopicArn,
		Protocol: &desired.Protocol,
		Endpoint: &desired.Endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	newState := SubscriptionState{
		ARN: *resp.SubscriptionArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

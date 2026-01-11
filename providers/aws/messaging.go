package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	typesSNS "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	typesSQS "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type QueueConfig struct {
	QueueName                     string            `json:"queue_name"`
	VisibilityTimeout             int               `json:"visibility_timeout"`
	MessageRetentionPeriod        int               `json:"message_retention_period"`
	DelaySeconds                  int               `json:"delay_seconds"`
	ReceiveMessageWaitTimeSeconds int               `json:"receive_message_wait_time_seconds"`
	FifoQueue                     bool              `json:"fifo_queue"`
	ContentBasedDeduplication     bool              `json:"content_based_deduplication"`
	RedrivePolicy                 string            `json:"redrive_policy"`
	Tags                          map[string]string `json:"tags"`
}

type QueueState struct {
	URL string `json:"url"`
	ARN string `json:"arn"`
}

type TopicConfig struct {
	Name                      string            `json:"name"`
	DisplayName               string            `json:"display_name"`
	FifoTopic                 bool              `json:"fifo_topic"`
	ContentBasedDeduplication bool              `json:"content_based_deduplication"`
	KmsMasterKeyId            string            `json:"kms_master_key_id"`
	Tags                      map[string]string `json:"tags"`
}

type TopicState struct {
	ARN string `json:"arn"`
}

type SubscriptionConfig struct {
	TopicArn string `json:"topic_arn"` // fix json tag to match pkl
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
		"VisibilityTimeout":             fmt.Sprintf("%d", desired.VisibilityTimeout),
		"MessageRetentionPeriod":        fmt.Sprintf("%d", desired.MessageRetentionPeriod),
		"DelaySeconds":                  fmt.Sprintf("%d", desired.DelaySeconds),
		"ReceiveMessageWaitTimeSeconds": fmt.Sprintf("%d", desired.ReceiveMessageWaitTimeSeconds),
	}
	if desired.FifoQueue {
		attrs["FifoQueue"] = "true"
	}
	if desired.ContentBasedDeduplication {
		attrs["ContentBasedDeduplication"] = "true"
	}
	if desired.RedrivePolicy != "" {
		attrs["RedrivePolicy"] = desired.RedrivePolicy
	}

	resp, err := p.sqsClient.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName:  &desired.QueueName,
		Attributes: attrs,
		Tags:       desired.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create queue: %w", err)
	}

	newState := QueueState{
		URL: *resp.QueueUrl,
	}
	// Fetch ARN attribute to populate ARN in state
	out, err := p.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: resp.QueueUrl,
		AttributeNames: []typesSQS.QueueAttributeName{
			typesSQS.QueueAttributeNameQueueArn,
		},
	})
	if err == nil {
		if arn, ok := out.Attributes[string(typesSQS.QueueAttributeNameQueueArn)]; ok {
			newState.ARN = arn
		}
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

	// Add attributes
	if desired.DisplayName != "" {
		input.Attributes["DisplayName"] = desired.DisplayName
	}
	if desired.FifoTopic {
		input.Attributes["FifoTopic"] = "true"
	}
	if desired.ContentBasedDeduplication {
		input.Attributes["ContentBasedDeduplication"] = "true"
	}
	if desired.KmsMasterKeyId != "" {
		input.Attributes["KmsMasterKeyId"] = desired.KmsMasterKeyId
	}

	// Add tags
	if len(desired.Tags) > 0 {
		for k, v := range desired.Tags {
			input.Tags = append(input.Tags, typesSNS.Tag{Key: &k, Value: &v})
		}
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

package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type LogGroupConfig struct {
	LogGroupName    string `json:"log_group_name"`
	RetentionInDays int    `json:"retention_in_days"`
}

type LogGroupState struct {
	LogGroupName string `json:"log_group_name"`
	ARN          string `json:"arn"`
}

type AlarmConfig struct {
	AlarmName          string  `json:"alarm_name"`
	MetricName         string  `json:"metric_name"`
	Namespace          string  `json:"namespace"`
	Threshold          float64 `json:"threshold"`
	ComparisonOperator string  `json:"comparison_operator"`
	EvaluationPeriods  int     `json:"evaluation_periods"`
	Period             int     `json:"period"`
	Statistic          string  `json:"statistic"`
}

type AlarmState struct {
	AlarmName string `json:"alarm_name"`
	ARN       string `json:"arn"`
}

func (p *Provider) applyLogGroup(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior LogGroupState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.LogGroupName != "" {
			_, err := p.cloudwatchlogsClient.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{
				LogGroupName: &prior.LogGroupName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete log group: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired LogGroupConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.cloudwatchlogsClient.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{
		LogGroupName: &desired.LogGroupName,
	})
	if err != nil {
		// Checking for resource already exists error would be good here
		return nil, fmt.Errorf("failed to create log group: %w", err)
	}

	if desired.RetentionInDays > 0 {
		_, err = p.cloudwatchlogsClient.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    &desired.LogGroupName,
			RetentionInDays: func(i int32) *int32 { return &i }(int32(desired.RetentionInDays)),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to put retention policy: %w", err)
		}
	}

	newState := LogGroupState{
		LogGroupName: desired.LogGroupName,
		ARN:          fmt.Sprintf("arn:aws:logs:us-east-1:*:log-group:%s", desired.LogGroupName), // Constructed ARN
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func (p *Provider) applyAlarm(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AlarmState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.AlarmName != "" {
			_, err := p.cloudwatchClient.DeleteAlarms(ctx, &cloudwatch.DeleteAlarmsInput{
				AlarmNames: []string{prior.AlarmName},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete alarm: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AlarmConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.cloudwatchClient.PutMetricAlarm(ctx, &cloudwatch.PutMetricAlarmInput{
		AlarmName:          &desired.AlarmName,
		MetricName:         &desired.MetricName,
		Namespace:          &desired.Namespace,
		Threshold:          &desired.Threshold,
		ComparisonOperator: types.ComparisonOperator(desired.ComparisonOperator),
		EvaluationPeriods:  func(i int32) *int32 { return &i }(int32(desired.EvaluationPeriods)),
		Period:             func(i int32) *int32 { return &i }(int32(desired.Period)),
		Statistic:          types.Statistic(desired.Statistic),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put metric alarm: %w", err)
	}

	newState := AlarmState{
		AlarmName: desired.AlarmName,
		ARN:       fmt.Sprintf("arn:aws:cloudwatch:us-east-1:*:alarm:%s", desired.AlarmName),
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Dashboard
type DashboardConfig struct {
	DashboardName string `json:"dashboard_name"`
	DashboardBody string `json:"dashboard_body"`
}

type DashboardState struct {
	DashboardName string `json:"dashboard_name"`
}

func (p *Provider) applyDashboard(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DashboardState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.DashboardName != "" {
			_, err := p.cloudwatchClient.DeleteDashboards(ctx, &cloudwatch.DeleteDashboardsInput{
				DashboardNames: []string{prior.DashboardName},
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete dashboard: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DashboardConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.cloudwatchClient.PutDashboard(ctx, &cloudwatch.PutDashboardInput{
		DashboardName: &desired.DashboardName,
		DashboardBody: &desired.DashboardBody,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to put dashboard: %w", err)
	}

	newState := DashboardState{DashboardName: desired.DashboardName}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// LogStream
type LogStreamConfig struct {
	LogGroupName  string `json:"log_group_name"`
	LogStreamName string `json:"log_stream_name"`
}

type LogStreamState struct {
	LogGroupName  string `json:"log_group_name"`
	LogStreamName string `json:"log_stream_name"`
	ARN           string `json:"arn"`
}

func (p *Provider) applyLogStream(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior LogStreamState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err == nil && prior.LogGroupName != "" {
			_, err := p.cloudwatchlogsClient.DeleteLogStream(ctx, &cloudwatchlogs.DeleteLogStreamInput{
				LogGroupName:  &prior.LogGroupName,
				LogStreamName: &prior.LogStreamName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete log stream: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired LogStreamConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	_, err := p.cloudwatchlogsClient.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{
		LogGroupName:  &desired.LogGroupName,
		LogStreamName: &desired.LogStreamName,
	})
	if err != nil {
		// handle already exists
		return nil, fmt.Errorf("failed to create log stream: %w", err)
	}

	newState := LogStreamState{
		LogGroupName:  desired.LogGroupName,
		LogStreamName: desired.LogStreamName,
		ARN:           fmt.Sprintf("arn:aws:logs:us-east-1:*:log-group:%s:log-stream:%s", desired.LogGroupName, desired.LogStreamName),
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

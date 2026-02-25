package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2"
	"github.com/aws/aws-sdk-go-v2/service/apigatewayv2/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// APIGatewayV2 Api

type ApiV2Config struct {
	ApiName                    string          `json:"api_name"`
	ProtocolType               string          `json:"protocol_type"`
	Description                string          `json:"description"`
	CorsConfiguration          *CorsConfig     `json:"cors_configuration"`
	DisableExecuteApiEndpoint  bool            `json:"disable_execute_api_endpoint"`
	Tags                       map[string]string `json:"tags"`
}

type CorsConfig struct {
	AllowOrigins     []string `json:"allow_origins"`
	AllowMethods     []string `json:"allow_methods"`
	AllowHeaders     []string `json:"allow_headers"`
	ExposeHeaders    []string `json:"expose_headers"`
	MaxAge           *int32   `json:"max_age"`
	AllowCredentials bool     `json:"allow_credentials"`
}

type ApiV2State struct {
	ApiId       string `json:"apiId"`
	ApiEndpoint string `json:"apiEndpoint"`
	Name        string `json:"name"`
}

func (p *Provider) applyApiV2(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior ApiV2State
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ApiId != "" {
			_, err := p.apigatewayv2Client.DeleteApi(ctx, &apigatewayv2.DeleteApiInput{
				ApiId: &prior.ApiId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete API: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired ApiV2Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &apigatewayv2.CreateApiInput{
		Name:                      &desired.ApiName,
		ProtocolType:              types.ProtocolType(desired.ProtocolType),
		DisableExecuteApiEndpoint: &desired.DisableExecuteApiEndpoint,
		Tags:                      desired.Tags,
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}

	if desired.CorsConfiguration != nil {
		input.CorsConfiguration = &types.Cors{
			AllowOrigins:     desired.CorsConfiguration.AllowOrigins,
			AllowMethods:     desired.CorsConfiguration.AllowMethods,
			AllowHeaders:     desired.CorsConfiguration.AllowHeaders,
			ExposeHeaders:    desired.CorsConfiguration.ExposeHeaders,
			AllowCredentials: &desired.CorsConfiguration.AllowCredentials,
		}
		if desired.CorsConfiguration.MaxAge != nil {
			input.CorsConfiguration.MaxAge = desired.CorsConfiguration.MaxAge
		}
	}

	resp, err := p.apigatewayv2Client.CreateApi(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create API: %w", err)
	}

	newState := ApiV2State{
		ApiId: *resp.ApiId,
		Name:  *resp.Name,
	}
	if resp.ApiEndpoint != nil {
		newState.ApiEndpoint = *resp.ApiEndpoint
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// APIGatewayV2 Stage

type StageV2Config struct {
	StageName            string            `json:"stage_name"`
	ApiId                string            `json:"api_id"`
	AutoDeploy           bool              `json:"auto_deploy"`
	Description          string            `json:"description"`
	DefaultRouteSettings *RouteSettingsCfg `json:"default_route_settings"`
	StageVariables       map[string]string `json:"stage_variables"`
	Tags                 map[string]string `json:"tags"`
}

type RouteSettingsCfg struct {
	DetailedMetricsEnabled bool    `json:"detailed_metrics_enabled"`
	LoggingLevel           string  `json:"logging_level"`
	ThrottlingBurstLimit   *int32  `json:"throttling_burst_limit"`
	ThrottlingRateLimit    *float64 `json:"throttling_rate_limit"`
}

type StageV2State struct {
	StageName string `json:"stageName"`
	ApiId     string `json:"apiId"`
}

func (p *Provider) applyStageV2(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior StageV2State
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.StageName != "" && prior.ApiId != "" {
			_, err := p.apigatewayv2Client.DeleteStage(ctx, &apigatewayv2.DeleteStageInput{
				ApiId:     &prior.ApiId,
				StageName: &prior.StageName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete stage: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired StageV2Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &apigatewayv2.CreateStageInput{
		StageName:      &desired.StageName,
		ApiId:          &desired.ApiId,
		AutoDeploy:     &desired.AutoDeploy,
		StageVariables: desired.StageVariables,
		Tags:           desired.Tags,
	}

	if desired.Description != "" {
		input.Description = &desired.Description
	}

	if desired.DefaultRouteSettings != nil {
		input.DefaultRouteSettings = &types.RouteSettings{
			DetailedMetricsEnabled: &desired.DefaultRouteSettings.DetailedMetricsEnabled,
		}
		if desired.DefaultRouteSettings.LoggingLevel != "" {
			input.DefaultRouteSettings.LoggingLevel = types.LoggingLevel(desired.DefaultRouteSettings.LoggingLevel)
		}
		if desired.DefaultRouteSettings.ThrottlingBurstLimit != nil {
			input.DefaultRouteSettings.ThrottlingBurstLimit = desired.DefaultRouteSettings.ThrottlingBurstLimit
		}
		if desired.DefaultRouteSettings.ThrottlingRateLimit != nil {
			input.DefaultRouteSettings.ThrottlingRateLimit = desired.DefaultRouteSettings.ThrottlingRateLimit
		}
	}

	resp, err := p.apigatewayv2Client.CreateStage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create stage: %w", err)
	}

	newState := StageV2State{
		StageName: *resp.StageName,
		ApiId:     desired.ApiId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// APIGatewayV2 Route

type RouteV2Config struct {
	ApiId             string `json:"api_id"`
	RouteKey          string `json:"route_key"`
	Target            string `json:"target"`
	AuthorizationType string `json:"authorization_type"`
	AuthorizerId      string `json:"authorizer_id"`
	OperationName     string `json:"operation_name"`
}

type RouteV2State struct {
	RouteId  string `json:"routeId"`
	RouteKey string `json:"routeKey"`
	ApiId    string `json:"apiId"`
}

func (p *Provider) applyRouteV2(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior RouteV2State
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.RouteId != "" && prior.ApiId != "" {
			_, err := p.apigatewayv2Client.DeleteRoute(ctx, &apigatewayv2.DeleteRouteInput{
				ApiId:   &prior.ApiId,
				RouteId: &prior.RouteId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete route: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired RouteV2Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &apigatewayv2.CreateRouteInput{
		ApiId:             &desired.ApiId,
		RouteKey:          &desired.RouteKey,
		AuthorizationType: types.AuthorizationType(desired.AuthorizationType),
	}

	if desired.Target != "" {
		input.Target = &desired.Target
	}
	if desired.AuthorizerId != "" {
		input.AuthorizerId = &desired.AuthorizerId
	}
	if desired.OperationName != "" {
		input.OperationName = &desired.OperationName
	}

	resp, err := p.apigatewayv2Client.CreateRoute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create route: %w", err)
	}

	newState := RouteV2State{
		RouteId:  *resp.RouteId,
		RouteKey: *resp.RouteKey,
		ApiId:    desired.ApiId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// APIGatewayV2 Integration

type IntegrationV2Config struct {
	ApiId                string `json:"api_id"`
	IntegrationType      string `json:"integration_type"`
	IntegrationUri       string `json:"integration_uri"`
	IntegrationMethod    string `json:"integration_method"`
	PassthroughBehavior  string `json:"passthrough_behavior"`
	ConnectionType       string `json:"connection_type"`
	ConnectionId         string `json:"connection_id"`
	PayloadFormatVersion string `json:"payload_format_version"`
	TimeoutInMillis      *int32 `json:"timeout_in_millis"`
}

type IntegrationV2State struct {
	IntegrationId string `json:"integrationId"`
	ApiId         string `json:"apiId"`
}

func (p *Provider) applyIntegrationV2(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior IntegrationV2State
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.IntegrationId != "" && prior.ApiId != "" {
			_, err := p.apigatewayv2Client.DeleteIntegration(ctx, &apigatewayv2.DeleteIntegrationInput{
				ApiId:         &prior.ApiId,
				IntegrationId: &prior.IntegrationId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete integration: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired IntegrationV2Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &apigatewayv2.CreateIntegrationInput{
		ApiId:           &desired.ApiId,
		IntegrationType: types.IntegrationType(desired.IntegrationType),
	}

	if desired.IntegrationUri != "" {
		input.IntegrationUri = &desired.IntegrationUri
	}
	if desired.IntegrationMethod != "" {
		input.IntegrationMethod = &desired.IntegrationMethod
	}
	if desired.PassthroughBehavior != "" {
		input.PassthroughBehavior = types.PassthroughBehavior(desired.PassthroughBehavior)
	}
	if desired.ConnectionType != "" {
		input.ConnectionType = types.ConnectionType(desired.ConnectionType)
	}
	if desired.ConnectionId != "" {
		input.ConnectionId = &desired.ConnectionId
	}
	if desired.PayloadFormatVersion != "" {
		input.PayloadFormatVersion = &desired.PayloadFormatVersion
	}
	if desired.TimeoutInMillis != nil {
		input.TimeoutInMillis = desired.TimeoutInMillis
	}

	resp, err := p.apigatewayv2Client.CreateIntegration(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create integration: %w", err)
	}

	newState := IntegrationV2State{
		IntegrationId: *resp.IntegrationId,
		ApiId:         desired.ApiId,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// APIGatewayV2 Domain Name

type DomainNameV2Config struct {
	DomainName               string                   `json:"domain_name"`
	DomainNameConfigurations []DomainNameConfigEntry   `json:"domain_name_configurations"`
	Tags                     map[string]string         `json:"tags"`
}

type DomainNameConfigEntry struct {
	CertificateArn string `json:"certificate_arn"`
	EndpointType   string `json:"endpoint_type"`
	SecurityPolicy string `json:"security_policy"`
}

type DomainNameV2State struct {
	DomainName        string `json:"domainName"`
	HostedZoneId      string `json:"hostedZoneId"`
	TargetDomainName  string `json:"targetDomainName"`
}

func (p *Provider) applyDomainNameV2(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DomainNameV2State
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.DomainName != "" {
			_, err := p.apigatewayv2Client.DeleteDomainName(ctx, &apigatewayv2.DeleteDomainNameInput{
				DomainName: &prior.DomainName,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete domain name: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DomainNameV2Config
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var configs []types.DomainNameConfiguration
	for _, c := range desired.DomainNameConfigurations {
		configs = append(configs, types.DomainNameConfiguration{
			CertificateArn: &c.CertificateArn,
			EndpointType:   types.EndpointType(c.EndpointType),
			SecurityPolicy: types.SecurityPolicy(c.SecurityPolicy),
		})
	}

	input := &apigatewayv2.CreateDomainNameInput{
		DomainName:               &desired.DomainName,
		DomainNameConfigurations: configs,
		Tags:                     desired.Tags,
	}

	resp, err := p.apigatewayv2Client.CreateDomainName(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create domain name: %w", err)
	}

	newState := DomainNameV2State{
		DomainName: *resp.DomainName,
	}
	if len(resp.DomainNameConfigurations) > 0 {
		if resp.DomainNameConfigurations[0].HostedZoneId != nil {
			newState.HostedZoneId = *resp.DomainNameConfigurations[0].HostedZoneId
		}
		if resp.DomainNameConfigurations[0].ApiGatewayDomainName != nil {
			newState.TargetDomainName = *resp.DomainNameConfigurations[0].ApiGatewayDomainName
		}
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

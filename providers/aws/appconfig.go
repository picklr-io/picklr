package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/appconfig"
	// "github.com/aws/aws-sdk-go-v2/service/appconfig/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// --- Application ---

type AppConfigApplicationConfig struct {
	Name string            `json:"name"`
	Tags map[string]string `json:"tags"`
}

type AppConfigApplicationState struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

func (p *Provider) applyAppConfigApplication(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AppConfigApplicationState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Id != "" {
			_, err := p.appconfigClient.DeleteApplication(ctx, &appconfig.DeleteApplicationInput{
				ApplicationId: &prior.Id,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete application: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AppConfigApplicationConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.appconfigClient.CreateApplication(ctx, &appconfig.CreateApplicationInput{
		Name: &desired.Name,
		Tags: desired.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create application: %w", err)
	}

	newState := AppConfigApplicationState{
		Id:   *resp.Id,
		Name: *resp.Name,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// --- Environment ---

type AppConfigEnvironmentConfig struct {
	ApplicationId string            `json:"application_id"`
	Name          string            `json:"name"`
	Tags          map[string]string `json:"tags"`
}

type AppConfigEnvironmentState struct {
	Id            string `json:"id"`
	ApplicationId string `json:"application_id"`
}

func (p *Provider) applyAppConfigEnvironment(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AppConfigEnvironmentState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Id != "" {
			_, err := p.appconfigClient.DeleteEnvironment(ctx, &appconfig.DeleteEnvironmentInput{
				ApplicationId: &prior.ApplicationId,
				EnvironmentId: &prior.Id,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete environment: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AppConfigEnvironmentConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.appconfigClient.CreateEnvironment(ctx, &appconfig.CreateEnvironmentInput{
		ApplicationId: &desired.ApplicationId,
		Name:          &desired.Name,
		Tags:          desired.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	newState := AppConfigEnvironmentState{
		Id:            *resp.Id,
		ApplicationId: desired.ApplicationId,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// --- Configuration Profile ---

type AppConfigProfileConfig struct {
	ApplicationId string            `json:"application_id"`
	Name          string            `json:"name"`
	LocationUri   string            `json:"location_uri"`
	Tags          map[string]string `json:"tags"`
}

type AppConfigProfileState struct {
	Id            string `json:"id"`
	ApplicationId string `json:"application_id"`
}

func (p *Provider) applyAppConfigProfile(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior AppConfigProfileState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior: %w", err)
		}
		if prior.Id != "" {
			_, err := p.appconfigClient.DeleteConfigurationProfile(ctx, &appconfig.DeleteConfigurationProfileInput{
				ApplicationId:          &prior.ApplicationId,
				ConfigurationProfileId: &prior.Id,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete configuration profile: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired AppConfigProfileConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	resp, err := p.appconfigClient.CreateConfigurationProfile(ctx, &appconfig.CreateConfigurationProfileInput{
		ApplicationId: &desired.ApplicationId,
		Name:          &desired.Name,
		LocationUri:   &desired.LocationUri,
		Tags:          desired.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create configuration profile: %w", err)
	}

	newState := AppConfigProfileState{
		Id:            *resp.Id,
		ApplicationId: desired.ApplicationId,
	}
	stateJSON, _ := json.Marshal(newState)
	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

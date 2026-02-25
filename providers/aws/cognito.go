package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/cognitoidentity"
	cognitoidptypes "github.com/aws/aws-sdk-go-v2/service/cognitoidentity/types"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

// Cognito User Pool

type UserPoolConfig struct {
	PoolName               string              `json:"pool_name"`
	PasswordPolicy         *PasswordPolicyCfg  `json:"password_policy"`
	AllowUserSelfSignUp    bool                `json:"allow_user_self_sign_up"`
	AutoVerifiedAttributes []string            `json:"auto_verified_attributes"`
	MfaConfiguration       string              `json:"mfa_configuration"`
	EmailConfiguration     *EmailConfigCfg     `json:"email_configuration"`
	SchemaAttributes       []SchemaAttrCfg     `json:"schema_attributes"`
	Tags                   map[string]string   `json:"tags"`
}

type PasswordPolicyCfg struct {
	MinimumLength                 int  `json:"minimum_length"`
	RequireUppercase              bool `json:"require_uppercase"`
	RequireLowercase              bool `json:"require_lowercase"`
	RequireNumbers                bool `json:"require_numbers"`
	RequireSymbols                bool `json:"require_symbols"`
	TemporaryPasswordValidityDays int  `json:"temporary_password_validity_days"`
}

type EmailConfigCfg struct {
	EmailSendingAccount string `json:"email_sending_account"`
	SourceArn           string `json:"source_arn"`
	ReplyToEmailAddress string `json:"reply_to_email_address"`
}

type SchemaAttrCfg struct {
	Name              string `json:"name"`
	AttributeDataType string `json:"attribute_data_type"`
	Required          bool   `json:"required"`
	Mutable           bool   `json:"mutable"`
}

type UserPoolState struct {
	UserPoolId string `json:"userPoolId"`
	ARN        string `json:"arn"`
	Name       string `json:"name"`
}

func (p *Provider) applyUserPool(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior UserPoolState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.UserPoolId != "" {
			_, err := p.cognitoIdpClient.DeleteUserPool(ctx, &cognitoidentityprovider.DeleteUserPoolInput{
				UserPoolId: &prior.UserPoolId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete user pool: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired UserPoolConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &cognitoidentityprovider.CreateUserPoolInput{
		PoolName:               &desired.PoolName,
		AutoVerifiedAttributes: toVerifiedAttrs(desired.AutoVerifiedAttributes),
		MfaConfiguration:       types.UserPoolMfaType(desired.MfaConfiguration),
		UserPoolTags:           desired.Tags,
	}

	if desired.PasswordPolicy != nil {
		minLen := int32(desired.PasswordPolicy.MinimumLength)
		tempDays := int32(desired.PasswordPolicy.TemporaryPasswordValidityDays)
		input.Policies = &types.UserPoolPolicyType{
			PasswordPolicy: &types.PasswordPolicyType{
				MinimumLength:                 &minLen,
				RequireUppercase:              desired.PasswordPolicy.RequireUppercase,
				RequireLowercase:              desired.PasswordPolicy.RequireLowercase,
				RequireNumbers:                desired.PasswordPolicy.RequireNumbers,
				RequireSymbols:                desired.PasswordPolicy.RequireSymbols,
				TemporaryPasswordValidityDays: tempDays,
			},
		}
	}

	if desired.EmailConfiguration != nil {
		input.EmailConfiguration = &types.EmailConfigurationType{
			EmailSendingAccount: types.EmailSendingAccountType(desired.EmailConfiguration.EmailSendingAccount),
		}
		if desired.EmailConfiguration.SourceArn != "" {
			input.EmailConfiguration.SourceArn = &desired.EmailConfiguration.SourceArn
		}
		if desired.EmailConfiguration.ReplyToEmailAddress != "" {
			input.EmailConfiguration.ReplyToEmailAddress = &desired.EmailConfiguration.ReplyToEmailAddress
		}
	}

	if len(desired.SchemaAttributes) > 0 {
		var schema []types.SchemaAttributeType
		for _, a := range desired.SchemaAttributes {
			schema = append(schema, types.SchemaAttributeType{
				Name:              strPtr(a.Name),
				AttributeDataType: types.AttributeDataType(a.AttributeDataType),
				Required:          &a.Required,
				Mutable:           &a.Mutable,
			})
		}
		input.Schema = schema
	}

	if !desired.AllowUserSelfSignUp {
		input.AdminCreateUserConfig = &types.AdminCreateUserConfigType{
			AllowAdminCreateUserOnly: true,
		}
	}

	resp, err := p.cognitoIdpClient.CreateUserPool(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create user pool: %w", err)
	}

	newState := UserPoolState{
		UserPoolId: *resp.UserPool.Id,
		ARN:        *resp.UserPool.Arn,
		Name:       *resp.UserPool.Name,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

func toVerifiedAttrs(attrs []string) []types.VerifiedAttributeType {
	var result []types.VerifiedAttributeType
	for _, a := range attrs {
		result = append(result, types.VerifiedAttributeType(a))
	}
	return result
}

// Cognito User Pool Client

type UserPoolClientConfig struct {
	ClientName                      string   `json:"client_name"`
	UserPoolId                      string   `json:"user_pool_id"`
	GenerateSecret                  bool     `json:"generate_secret"`
	AllowedOauthFlows               []string `json:"allowed_oauth_flows"`
	AllowedOauthScopes              []string `json:"allowed_oauth_scopes"`
	AllowedOauthFlowsUserPoolClient bool     `json:"allowed_oauth_flows_user_pool_client"`
	CallbackUrls                    []string `json:"callback_urls"`
	LogoutUrls                      []string `json:"logout_urls"`
	SupportedIdentityProviders      []string `json:"supported_identity_providers"`
	AccessTokenValidity             *int32   `json:"access_token_validity"`
	IdTokenValidity                 *int32   `json:"id_token_validity"`
	RefreshTokenValidity            int32    `json:"refresh_token_validity"`
	ExplicitAuthFlows               []string `json:"explicit_auth_flows"`
}

type UserPoolClientState struct {
	ClientId   string `json:"clientId"`
	UserPoolId string `json:"userPoolId"`
	Name       string `json:"name"`
}

func (p *Provider) applyUserPoolClient(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior UserPoolClientState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ClientId != "" {
			_, err := p.cognitoIdpClient.DeleteUserPoolClient(ctx, &cognitoidentityprovider.DeleteUserPoolClientInput{
				UserPoolId: &prior.UserPoolId,
				ClientId:   &prior.ClientId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete user pool client: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired UserPoolClientConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &cognitoidentityprovider.CreateUserPoolClientInput{
		ClientName:                      &desired.ClientName,
		UserPoolId:                      &desired.UserPoolId,
		GenerateSecret:                  desired.GenerateSecret,
		AllowedOAuthFlowsUserPoolClient: desired.AllowedOauthFlowsUserPoolClient,
	}

	if len(desired.AllowedOauthFlows) > 0 {
		var flows []types.OAuthFlowType
		for _, f := range desired.AllowedOauthFlows {
			flows = append(flows, types.OAuthFlowType(f))
		}
		input.AllowedOAuthFlows = flows
	}
	if len(desired.AllowedOauthScopes) > 0 {
		input.AllowedOAuthScopes = desired.AllowedOauthScopes
	}
	if len(desired.CallbackUrls) > 0 {
		input.CallbackURLs = desired.CallbackUrls
	}
	if len(desired.LogoutUrls) > 0 {
		input.LogoutURLs = desired.LogoutUrls
	}
	if len(desired.SupportedIdentityProviders) > 0 {
		input.SupportedIdentityProviders = desired.SupportedIdentityProviders
	}
	if desired.AccessTokenValidity != nil {
		input.AccessTokenValidity = desired.AccessTokenValidity
	}
	if desired.IdTokenValidity != nil {
		input.IdTokenValidity = desired.IdTokenValidity
	}
	if desired.RefreshTokenValidity > 0 {
		input.RefreshTokenValidity = desired.RefreshTokenValidity
	}
	if len(desired.ExplicitAuthFlows) > 0 {
		var flows []types.ExplicitAuthFlowsType
		for _, f := range desired.ExplicitAuthFlows {
			flows = append(flows, types.ExplicitAuthFlowsType(f))
		}
		input.ExplicitAuthFlows = flows
	}

	resp, err := p.cognitoIdpClient.CreateUserPoolClient(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create user pool client: %w", err)
	}

	newState := UserPoolClientState{
		ClientId:   *resp.UserPoolClient.ClientId,
		UserPoolId: *resp.UserPoolClient.UserPoolId,
		Name:       *resp.UserPoolClient.ClientName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

// Cognito Identity Pool

type IdentityPoolConfig struct {
	IdentityPoolName                 string                   `json:"identity_pool_name"`
	AllowUnauthenticatedIdentities   bool                     `json:"allow_unauthenticated_identities"`
	CognitoIdentityProviders         []CognitoIdProviderCfg   `json:"cognito_identity_providers"`
	Tags                             map[string]string        `json:"tags"`
}

type CognitoIdProviderCfg struct {
	ClientId             string `json:"client_id"`
	ProviderName         string `json:"provider_name"`
	ServerSideTokenCheck bool   `json:"server_side_token_check"`
}

type IdentityPoolState struct {
	IdentityPoolId   string `json:"identityPoolId"`
	IdentityPoolName string `json:"identityPoolName"`
}

func (p *Provider) applyIdentityPool(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior IdentityPoolState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.IdentityPoolId != "" {
			_, err := p.cognitoIdentityClient.DeleteIdentityPool(ctx, &cognitoidentity.DeleteIdentityPoolInput{
				IdentityPoolId: &prior.IdentityPoolId,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete identity pool: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired IdentityPoolConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	input := &cognitoidentity.CreateIdentityPoolInput{
		IdentityPoolName:               &desired.IdentityPoolName,
		AllowUnauthenticatedIdentities: desired.AllowUnauthenticatedIdentities,
		IdentityPoolTags:               desired.Tags,
	}

	if len(desired.CognitoIdentityProviders) > 0 {
		var providers []cognitoidptypes.CognitoIdentityProvider
		for _, p := range desired.CognitoIdentityProviders {
			providers = append(providers, cognitoidptypes.CognitoIdentityProvider{
				ClientId:             &p.ClientId,
				ProviderName:         &p.ProviderName,
				ServerSideTokenCheck: &p.ServerSideTokenCheck,
			})
		}
		input.CognitoIdentityProviders = providers
	}

	resp, err := p.cognitoIdentityClient.CreateIdentityPool(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create identity pool: %w", err)
	}

	newState := IdentityPoolState{
		IdentityPoolId:   *resp.IdentityPoolId,
		IdentityPoolName: *resp.IdentityPoolName,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

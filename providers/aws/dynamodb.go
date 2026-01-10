package aws

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type TableConfig struct {
	TableName   string                `json:"tableName"`
	Attributes  []AttributeDefinition `json:"attributes"`
	KeySchema   []KeySchemaElement    `json:"keySchema"`
	BillingMode string                `json:"billingMode"`
}

type AttributeDefinition struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type KeySchemaElement struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type TableState struct {
	Name string `json:"name"`
	ARN  string `json:"arn"`
}

func (p *Provider) applyTable(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior TableState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.Name != "" {
			_, err := p.dynamodbClient.DeleteTable(ctx, &dynamodb.DeleteTableInput{
				TableName: &prior.Name,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete table: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired TableConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var attrs []types.AttributeDefinition
	for _, a := range desired.Attributes {
		attrs = append(attrs, types.AttributeDefinition{
			AttributeName: &a.Name,
			AttributeType: types.ScalarAttributeType(a.Type),
		})
	}

	var keySchema []types.KeySchemaElement
	for _, k := range desired.KeySchema {
		keySchema = append(keySchema, types.KeySchemaElement{
			AttributeName: &k.Name,
			KeyType:       types.KeyType(k.Type),
		})
	}

	resp, err := p.dynamodbClient.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            &desired.TableName,
		AttributeDefinitions: attrs,
		KeySchema:            keySchema,
		BillingMode:          types.BillingMode(desired.BillingMode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create table: %w", err)
	}

	newState := TableState{
		Name: *resp.TableDescription.TableName,
		ARN:  *resp.TableDescription.TableArn,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type DistributionConfig struct {
	Enabled              bool                 `json:"enabled"`
	PriceClass           string               `json:"price_class"`
	DefaultCacheBehavior DefaultCacheBehavior `json:"default_cache_behavior"`
	Origins              []Origin             `json:"origins"`
}

type DefaultCacheBehavior struct {
	TargetOriginID       string   `json:"target_origin_id"`
	ViewerProtocolPolicy string   `json:"viewer_protocol_policy"`
	AllowedMethods       []string `json:"allowed_methods"`
}

type Origin struct {
	DomainName string `json:"domain_name"`
	OriginID   string `json:"origin_id"`
}

type DistributionState struct {
	ID     string `json:"id"`
	ARN    string `json:"arn"`
	Domain string `json:"domain"`
	ETag   string `json:"etag"`
}

func (p *Provider) applyDistribution(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if req.DesiredConfigJson == nil {
		var prior DistributionState
		if err := json.Unmarshal(req.PriorStateJson, &prior); err != nil {
			return nil, fmt.Errorf("failed to unmarshal prior state: %w", err)
		}
		if prior.ID != "" {
			// CloudFront requires disabling before deletion and uses ETags.
			// Simplified: Force Delete implies Disable -> Wait -> Delete, which is complex.
			// For MVP, we might error or try a best-effort disable + delete.

			// Get current config to get ETag if not stored
			// etag := prior.ETag

			_, err := p.cloudfrontClient.DeleteDistribution(ctx, &cloudfront.DeleteDistributionInput{
				Id:      &prior.ID,
				IfMatch: &prior.ETag,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to delete distribution: %w", err)
			}
		}
		return &pb.ApplyResponse{}, nil
	}

	var desired DistributionConfig
	if err := json.Unmarshal(req.DesiredConfigJson, &desired); err != nil {
		return nil, fmt.Errorf("failed to unmarshal desired: %w", err)
	}

	var items []types.Origin
	for _, o := range desired.Origins {
		items = append(items, types.Origin{
			Id:         &o.OriginID,
			DomainName: &o.DomainName,
			CustomOriginConfig: &types.CustomOriginConfig{
				HTTPPort:             func(i int32) *int32 { return &i }(80),
				HTTPSPort:            func(i int32) *int32 { return &i }(443),
				OriginProtocolPolicy: types.OriginProtocolPolicyHttpOnly, // Defaulting for simplicity
			},
		})
	}

	// Prepare AllowedMethods slice
	var methods []types.Method
	for _, m := range desired.DefaultCacheBehavior.AllowedMethods {
		methods = append(methods, types.Method(m))
	}

	callerRef := fmt.Sprintf("picklr-%d", time.Now().UnixNano())

	input := &cloudfront.CreateDistributionInput{
		DistributionConfig: &types.DistributionConfig{
			CallerReference: &callerRef,
			Enabled:         &desired.Enabled,
			Origins: &types.Origins{
				Quantity: func(i int32) *int32 { return &i }(int32(len(items))),
				Items:    items,
			},
			DefaultCacheBehavior: &types.DefaultCacheBehavior{
				TargetOriginId:       &desired.DefaultCacheBehavior.TargetOriginID,
				ViewerProtocolPolicy: types.ViewerProtocolPolicy(desired.DefaultCacheBehavior.ViewerProtocolPolicy),
				AllowedMethods: &types.AllowedMethods{
					Quantity: func(i int32) *int32 { return &i }(int32(len(methods))),
					Items:    methods,
					CachedMethods: &types.CachedMethods{
						Quantity: func(i int32) *int32 { return &i }(int32(2)),
						Items:    []types.Method{types.MethodGet, types.MethodHead},
					},
				},
				MinTTL: func(i int64) *int64 { return &i }(0),
				ForwardedValues: &types.ForwardedValues{
					Cookies: &types.CookiePreference{
						Forward: types.ItemSelectionNone,
					},
					QueryString: func(b bool) *bool { return &b }(false),
				},
			},
			Comment: func(s string) *string { return &s }("Created by Picklr"),
		},
	}

	resp, err := p.cloudfrontClient.CreateDistribution(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create distribution: %w", err)
	}

	newState := DistributionState{
		ID:     *resp.Distribution.Id,
		ARN:    *resp.Distribution.ARN,
		Domain: *resp.Distribution.DomainName,
		ETag:   *resp.ETag,
	}
	stateJSON, _ := json.Marshal(newState)

	return &pb.ApplyResponse{NewStateJson: stateJSON}, nil
}

package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/acm"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"

	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/sns"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	pb "github.com/picklr-io/picklr/pkg/proto/provider"
)

type Provider struct {
	pb.UnimplementedProviderServer
	s3Client          *s3.Client
	ec2Client         *ec2.Client
	iamClient         *iam.Client
	lambdaClient      *lambda.Client
	dynamodbClient    *dynamodb.Client
	rdsClient         *rds.Client
	sqsClient         *sqs.Client
	snsClient         *sns.Client
	ecrClient         *ecr.Client
	ecsClient         *ecs.Client
	elbv2Client       *elasticloadbalancingv2.Client
	route53Client     *route53.Client
	apigatewayClient  *apigateway.Client
	autoscalingClient *autoscaling.Client
	acmClient         *acm.Client
	cloudfrontClient  *cloudfront.Client
	eventbridgeClient *eventbridge.Client
	efsClient         *efs.Client

	cloudwatchClient     *cloudwatch.Client
	cloudwatchlogsClient *cloudwatchlogs.Client
	kmsClient            *kms.Client
	secretsmanagerClient *secretsmanager.Client
}

func New() *Provider {
	return &Provider{}
}

func (p *Provider) ensureClient(ctx context.Context, region string) error {
	if p.s3Client != nil && p.ec2Client != nil && p.iamClient != nil && p.lambdaClient != nil && p.dynamodbClient != nil && p.rdsClient != nil && p.sqsClient != nil && p.snsClient != nil && p.ecrClient != nil && p.ecsClient != nil && p.elbv2Client != nil && p.route53Client != nil && p.apigatewayClient != nil && p.cloudfrontClient != nil && p.cloudwatchClient != nil && p.cloudwatchlogsClient != nil && p.kmsClient != nil && p.secretsmanagerClient != nil {
		return nil
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("unable to load SDK config, %v", err)
	}

	p.s3Client = s3.NewFromConfig(cfg)
	p.ec2Client = ec2.NewFromConfig(cfg)
	p.iamClient = iam.NewFromConfig(cfg)
	p.lambdaClient = lambda.NewFromConfig(cfg)
	p.dynamodbClient = dynamodb.NewFromConfig(cfg)
	p.rdsClient = rds.NewFromConfig(cfg)
	p.sqsClient = sqs.NewFromConfig(cfg)
	p.snsClient = sns.NewFromConfig(cfg)
	p.ecrClient = ecr.NewFromConfig(cfg)
	p.ecsClient = ecs.NewFromConfig(cfg)
	p.elbv2Client = elasticloadbalancingv2.NewFromConfig(cfg)
	p.route53Client = route53.NewFromConfig(cfg)
	p.apigatewayClient = apigateway.NewFromConfig(cfg)
	p.autoscalingClient = autoscaling.NewFromConfig(cfg)
	p.acmClient = acm.NewFromConfig(cfg)
	p.cloudfrontClient = cloudfront.NewFromConfig(cfg)
	p.eventbridgeClient = eventbridge.NewFromConfig(cfg)
	p.efsClient = efs.NewFromConfig(cfg)

	p.cloudwatchClient = cloudwatch.NewFromConfig(cfg)
	p.cloudwatchlogsClient = cloudwatchlogs.NewFromConfig(cfg)
	p.kmsClient = kms.NewFromConfig(cfg)
	p.secretsmanagerClient = secretsmanager.NewFromConfig(cfg)

	return nil
}

func (p *Provider) Configure(ctx context.Context, req *pb.ConfigureRequest) (*pb.ConfigureResponse, error) {
	// For simplicity, we'll just initialize with default config for now,
	// or maybe pick up region from the request if we decide to pass it in Configure logic later.
	// Ideally, the ConfigureRequest should contain the provider configuration (region, etc).
	// Here we just ensure we can load the default config.
	if err := p.ensureClient(ctx, "us-east-1"); err != nil { // Default region or extract from config
		return &pb.ConfigureResponse{
			Diagnostics: []*pb.Diagnostic{
				{
					Severity: pb.Diagnostic_ERROR,
					Summary:  "Failed to load AWS config",
					Detail:   err.Error(),
				},
			},
		}, nil
	}
	return &pb.ConfigureResponse{}, nil
}

func (p *Provider) Plan(ctx context.Context, req *pb.PlanRequest) (*pb.PlanResponse, error) {
	if err := p.ensureClient(ctx, "us-east-1"); err != nil {
		return nil, err
	}

	switch req.Type {
	case "aws:S3.Bucket":
		return p.planBucket(ctx, req)
	case "aws:EC2.Instance":
		return p.planInstance(ctx, req)
	}

	// Fallback for other resources (Naive logic)
	if req.DesiredConfigJson == nil && req.PriorStateJson != nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_DELETE}, nil
	}

	if req.PriorStateJson == nil {
		return &pb.PlanResponse{Action: pb.PlanResponse_CREATE}, nil
	}

	if string(req.DesiredConfigJson) != string(req.PriorStateJson) {
		return &pb.PlanResponse{Action: pb.PlanResponse_REPLACE}, nil
	}

	return &pb.PlanResponse{Action: pb.PlanResponse_NOOP}, nil
}

func (p *Provider) Apply(ctx context.Context, req *pb.ApplyRequest) (*pb.ApplyResponse, error) {
	if err := p.ensureClient(ctx, "us-east-1"); err != nil {
		return nil, err
	}

	switch req.Type {
	case "aws:S3.Bucket": // Mapping from Pkl type name
		return p.applyBucket(ctx, req)
	case "aws:EC2.Instance":
		return p.applyInstance(ctx, req)
	case "aws:EC2.KeyPair":
		return p.applyKeyPair(ctx, req)
	case "aws:EC2.LaunchTemplate":
		return p.applyLaunchTemplate(ctx, req)
	case "aws:AutoScaling.AutoScalingGroup":
		return p.applyAutoScalingGroup(ctx, req)

	case "aws:EC2.Vpc":
		return p.applyVpc(ctx, req)
	case "aws:EC2.Subnet":
		return p.applySubnet(ctx, req)
	case "aws:EC2.SecurityGroup":
		return p.applySecurityGroup(ctx, req)
	case "aws:EC2.InternetGateway":
		return p.applyInternetGateway(ctx, req)
	case "aws:EC2.ElasticIP":
		return p.applyElasticIP(ctx, req)
	case "aws:EC2.NatGateway":
		return p.applyNatGateway(ctx, req)
	case "aws:EC2.RouteTable":
		return p.applyRouteTable(ctx, req)

	case "aws:IAM.Role":
		return p.applyRole(ctx, req)
	case "aws:IAM.Policy":
		return p.applyPolicy(ctx, req)
	case "aws:Lambda.Function":
		return p.applyFunction(ctx, req)
	case "aws:DynamoDB.Table":
		return p.applyTable(ctx, req)
	case "aws:RDS.Instance":
		return p.applyDBInstance(ctx, req)
	case "aws:SQS.Queue":
		return p.applyQueue(ctx, req)
	case "aws:SNS.Topic":
		return p.applyTopic(ctx, req)
	case "aws:SNS.Subscription":
		return p.applySubscription(ctx, req)
	case "aws:ECR.Repository":
		return p.applyRepository(ctx, req)
	case "aws:ECS.Cluster":
		return p.applyCluster(ctx, req)
	case "aws:ECS.TaskDefinition":
		return p.applyTaskDefinition(ctx, req)
	case "aws:ECS.Service":
		return p.applyService(ctx, req)
	case "aws:ELBv2.LoadBalancer":
		return p.applyLoadBalancer(ctx, req)
	case "aws:ELBv2.TargetGroup":
		return p.applyTargetGroup(ctx, req)
	case "aws:ELBv2.Listener":
		return p.applyListener(ctx, req)
	case "aws:Route53.HostedZone":
		return p.applyHostedZone(ctx, req)
	case "aws:Route53.RecordSet":
		return p.applyRecordSet(ctx, req)
	case "aws:APIGateway.RestApi":
		return p.applyRestApi(ctx, req)
	case "aws:APIGateway.ApiResource":
		return p.applyApiResource(ctx, req)
	case "aws:APIGateway.Method":

		return p.applyMethod(ctx, req)
	case "aws:APIGateway.Deployment":
		return p.applyDeployment(ctx, req)
	case "aws:CloudFront.Distribution":
		return p.applyDistribution(ctx, req)
	case "aws:CloudWatch.LogGroup":
		return p.applyLogGroup(ctx, req)
	case "aws:CloudWatch.Alarm":
		return p.applyAlarm(ctx, req)
	case "aws:KMS.Key":
		return p.applyKey(ctx, req)
	case "aws:KMS.Alias":
		return p.applyAlias(ctx, req)
	case "aws:SecretsManager.Secret":
		return p.applySecret(ctx, req)
	case "aws:SecretsManager.SecretVersion":
		return p.applySecretVersion(ctx, req)
	case "aws:ACM.Certificate":
		return p.applyCertificate(ctx, req)
	case "aws:ACM.CertificateValidation":
		return p.applyCertificateValidation(ctx, req)
	case "aws:EventBridge.EventBus":
		return p.applyEventBus(ctx, req)
	case "aws:EventBridge.Rule":
		return p.applyRule(ctx, req)
	case "aws:EventBridge.Target":
		return p.applyTarget(ctx, req)
	case "aws:IAM.InstanceProfile":
		return p.applyInstanceProfile(ctx, req)
	case "aws:S3.BucketPolicy":
		return p.applyBucketPolicy(ctx, req)
	case "aws:EC2.Volume":
		return p.applyVolume(ctx, req)
	case "aws:RDS.DBSubnetGroup":
		return p.applyDBSubnetGroup(ctx, req)
	case "aws:RDS.DBParameterGroup":
		return p.applyDBParameterGroup(ctx, req)
	case "aws:RDS.DBCluster":
		return p.applyDBCluster(ctx, req)
	case "aws:EC2.NetworkAcl":
		return p.applyNetworkAcl(ctx, req)
	case "aws:EC2.VpcPeeringConnection":
		return p.applyVpcPeeringConnection(ctx, req)
	case "aws:EC2.TransitGateway":
		return p.applyTransitGateway(ctx, req)
	case "aws:EC2.TransitGatewayAttachment":
		return p.applyTransitGatewayAttachment(ctx, req)
	case "aws:EC2.VpcEndpoint":
		return p.applyVpcEndpoint(ctx, req)
	case "aws:EC2.PlacementGroup":
		return p.applyPlacementGroup(ctx, req)

	case "aws:IAM.User":
		return p.applyUser(ctx, req)
	case "aws:IAM.Group":
		return p.applyGroup(ctx, req)
	case "aws:IAM.PolicyAttachment":
		return p.applyPolicyAttachment(ctx, req)
	case "aws:IAM.ServiceLinkedRole":
		return p.applyServiceLinkedRole(ctx, req)

	case "aws:Lambda.Layer":
		return p.applyLayer(ctx, req)
	case "aws:Lambda.Permission":
		return p.applyPermission(ctx, req)

	case "aws:EFS.FileSystem":
		return p.applyFileSystem(ctx, req)
	case "aws:EFS.MountTarget":
		return p.applyMountTarget(ctx, req)

	case "aws:S3.BucketLifecycle":
		return p.applyBucketLifecycle(ctx, req)
	case "aws:S3.BucketNotification":
		return p.applyBucketNotification(ctx, req)
	}

	return nil, fmt.Errorf("unknown resource type: %s", req.Type)
}

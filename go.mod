module github.com/picklr-io/picklr

go 1.24.1

require (
	github.com/apple/pkl-go v0.12.1
	github.com/aws/aws-sdk-go-v2 v1.41.2
	github.com/aws/aws-sdk-go-v2/config v1.32.6
	github.com/aws/aws-sdk-go-v2/service/acm v1.37.19
	github.com/aws/aws-sdk-go-v2/service/apigateway v1.38.4
	github.com/aws/aws-sdk-go-v2/service/apigatewayv2 v1.33.6
	github.com/aws/aws-sdk-go-v2/service/appconfig v1.43.9
	github.com/aws/aws-sdk-go-v2/service/athena v1.56.5
	github.com/aws/aws-sdk-go-v2/service/autoscaling v1.62.5
	github.com/aws/aws-sdk-go-v2/service/cloudfront v1.59.0
	github.com/aws/aws-sdk-go-v2/service/cloudtrail v1.55.6
	github.com/aws/aws-sdk-go-v2/service/cloudwatch v1.53.1
	github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs v1.63.1
	github.com/aws/aws-sdk-go-v2/service/codebuild v1.68.9
	github.com/aws/aws-sdk-go-v2/service/codecommit v1.33.8
	github.com/aws/aws-sdk-go-v2/service/codedeploy v1.35.9
	github.com/aws/aws-sdk-go-v2/service/codepipeline v1.46.17
	github.com/aws/aws-sdk-go-v2/service/cognitoidentity v1.33.19
	github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider v1.58.1
	github.com/aws/aws-sdk-go-v2/service/dynamodb v1.53.5
	github.com/aws/aws-sdk-go-v2/service/ec2 v1.279.0
	github.com/aws/aws-sdk-go-v2/service/ecr v1.55.1
	github.com/aws/aws-sdk-go-v2/service/ecs v1.70.1
	github.com/aws/aws-sdk-go-v2/service/efs v1.41.10
	github.com/aws/aws-sdk-go-v2/service/eks v1.80.1
	github.com/aws/aws-sdk-go-v2/service/elasticache v1.51.10
	github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2 v1.54.6
	github.com/aws/aws-sdk-go-v2/service/eventbridge v1.45.18
	github.com/aws/aws-sdk-go-v2/service/globalaccelerator v1.35.10
	github.com/aws/aws-sdk-go-v2/service/glue v1.136.0
	github.com/aws/aws-sdk-go-v2/service/iam v1.53.1
	github.com/aws/aws-sdk-go-v2/service/kafka v1.46.7
	github.com/aws/aws-sdk-go-v2/service/kinesis v1.42.10
	github.com/aws/aws-sdk-go-v2/service/kms v1.49.5
	github.com/aws/aws-sdk-go-v2/service/lambda v1.87.0
	github.com/aws/aws-sdk-go-v2/service/opensearch v1.57.1
	github.com/aws/aws-sdk-go-v2/service/rds v1.113.1
	github.com/aws/aws-sdk-go-v2/service/redshift v1.61.5
	github.com/aws/aws-sdk-go-v2/service/route53 v1.62.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.95.0
	github.com/aws/aws-sdk-go-v2/service/secretsmanager v1.41.1
	github.com/aws/aws-sdk-go-v2/service/sesv2 v1.59.2
	github.com/aws/aws-sdk-go-v2/service/sfn v1.40.6
	github.com/aws/aws-sdk-go-v2/service/sns v1.39.11
	github.com/aws/aws-sdk-go-v2/service/sqs v1.42.21
	github.com/aws/aws-sdk-go-v2/service/ssm v1.68.1
	github.com/aws/aws-sdk-go-v2/service/wafv2 v1.70.8
	github.com/aws/aws-sdk-go-v2/service/xray v1.36.17
	github.com/aws/smithy-go v1.24.1
	github.com/docker/docker v27.3.1+incompatible
	github.com/docker/go-connections v0.6.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	google.golang.org/grpc v1.77.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/AdaLogics/go-fuzz-headers v0.0.0-20240806141605-e8a1dd7889d6 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/aws/aws-sdk-go-v2/aws/protocol/eventstream v1.7.4 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.6 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.18 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/internal/v4a v1.4.18 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/checksum v1.9.7 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/endpoint-discovery v1.11.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/s3shared v1.19.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.5 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/klauspost/compress v1.18.2 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.2 // indirect
	github.com/morikuni/aec v1.1.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.39.0 // indirect
	go.opentelemetry.io/otel/metric v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.2 // indirect
)

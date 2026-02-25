# Providers

Providers are the bridge between Picklr and infrastructure APIs. Each provider implements the `ProviderServer` gRPC interface to manage resources of a specific platform.

## Provider Interface

Every provider implements these operations:

| Method | Description |
|--------|-------------|
| `GetSchema` | Returns the provider's PKL schema |
| `Configure` | Initializes the provider with credentials and region |
| `Plan` | Compares desired vs. prior state, returns the planned action |
| `Apply` | Executes a create, update, or replace operation |
| `Read` | Refreshes resource state from the real infrastructure |
| `Delete` | Removes a resource |

## Resource Type Naming

Resource types follow a namespaced format:

```
<provider>:<Service>.<Resource>
```

Examples:
- `aws:S3.Bucket`
- `aws:EC2.Instance`
- `aws:IAM.Role`
- `aws:Lambda.Function`
- `docker_container`
- `null_resource`

## AWS Provider

The AWS provider supports a wide range of services. It uses the AWS SDK for Go v2 and authenticates using the standard AWS credential chain (environment variables, shared credentials file, IAM roles, etc.).

### Supported Resources

| Service | Resource Types |
|---------|---------------|
| **S3** | `aws:S3.Bucket` |
| **EC2** | `aws:EC2.Instance`, `aws:EC2.Vpc`, `aws:EC2.Subnet`, `aws:EC2.SecurityGroup`, `aws:EC2.InternetGateway`, `aws:EC2.RouteTable`, `aws:EC2.ElasticIP`, `aws:EC2.NatGateway`, `aws:EC2.KeyPair` |
| **IAM** | `aws:IAM.Role`, `aws:IAM.Policy`, `aws:IAM.InstanceProfile` |
| **Lambda** | `aws:Lambda.Function` |
| **RDS** | `aws:RDS.Instance`, `aws:RDS.SubnetGroup` |
| **ECS** | `aws:ECS.Cluster`, `aws:ECS.TaskDefinition`, `aws:ECS.Service` |
| **ELB** | `aws:ELB.LoadBalancer`, `aws:ELB.TargetGroup`, `aws:ELB.Listener` |
| **DynamoDB** | `aws:DynamoDB.Table` |
| **SNS** | `aws:SNS.Topic` |
| **SQS** | `aws:SQS.Queue` |
| **CloudWatch** | `aws:CloudWatch.LogGroup`, `aws:CloudWatch.Alarm` |
| **Route53** | `aws:Route53.HostedZone`, `aws:Route53.Record` |
| **CloudFront** | `aws:CloudFront.Distribution` |
| **ECR** | `aws:ECR.Repository` |
| **KMS** | `aws:KMS.Key` |
| **SecretsManager** | `aws:SecretsManager.Secret` |
| **ACM** | `aws:ACM.Certificate` |
| **EFS** | `aws:EFS.FileSystem` |
| **StepFunctions** | `aws:SFN.StateMachine` |

### Configuration

The AWS provider is configured automatically when resources reference it. It uses the standard AWS SDK credential resolution:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
2. Shared credentials file (`~/.aws/credentials`)
3. IAM instance profile / ECS task role
4. SSO credentials

Region is determined from:
1. Resource-level configuration
2. `AWS_REGION` / `AWS_DEFAULT_REGION` environment variables
3. Shared config file (`~/.aws/config`)

### PKL Schema Example

```pkl
import "../../pkg/schemas/aws/S3.pkl"
import "../../pkg/schemas/aws/EC2.pkl"

aws {
  buckets = new Listing {
    new S3.Bucket {
      name = "app-assets"
      bucket = "my-company-assets"
      acl = "private"
    }
  }

  instances = new Listing {
    new EC2.Instance {
      name = "web-server"
      ami = "ami-0abcdef1234567890"
      instanceType = "t3.micro"
      subnetId = ptr("aws:EC2.Subnet/public-1/id")
      securityGroupIds = new Listing { ptr("aws:EC2.SecurityGroup/web-sg/id") }
      tags {
        ["Name"] = "web-server"
        ["Environment"] = "production"
      }
    }
  }
}
```

## Docker Provider

The Docker provider manages containers, networks, volumes, and images on a Docker daemon.

### Supported Resources

| Resource Type | Description |
|--------------|-------------|
| `docker_container` | Docker containers |
| `docker_network` | Docker networks |
| `docker_volume` | Docker volumes |
| `docker_image` | Docker images |

### Example

```pkl
import "../../pkg/schemas/docker/Container.pkl"

docker {
  containers = new Listing {
    new Container.Container {
      name = "nginx"
      image = "nginx:latest"
      ports {
        new {
          internal = 80
          external = 8080
        }
      }
    }
  }
}
```

## Null Provider

The null provider is a testing/no-op provider. It doesn't manage any real infrastructure but follows the full provider lifecycle. Useful for:

- Learning Picklr without cloud credentials
- Testing configuration and state management
- CI pipeline validation

### Resource Type

`null_resource` — accepts arbitrary `triggers` properties.

### Example

```pkl
resources {
  new {
    type = "null_resource"
    name = "example"
    provider = "null"
    properties {
      ["triggers"] = new Mapping {
        ["timestamp"] = "2025-01-01"
      }
    }
  }
}
```

## Cross-Resource References

Resources can reference attributes from other resources using the `ptr://` protocol:

```
ptr://<provider>:<Type>/<name>/<attribute>
```

References are resolved at apply time from the state. The engine automatically determines dependency ordering from these references.

### Examples

```pkl
// Reference a VPC ID
subnetVpcId = "ptr://aws:EC2.Vpc/main/id"

// Reference an S3 bucket ARN
bucketArn = "ptr://aws:S3.Bucket/logs/arn"

// Reference a security group ID
sgId = "ptr://aws:EC2.SecurityGroup/web/id"
```

## Adding a New Provider

To add a new provider to Picklr:

1. **Create PKL schemas** under `pkg/schemas/<provider>/`
2. **Implement `ProviderServer`** in `providers/<provider>/provider.go`:
   - `GetSchema()` — return the PKL schema
   - `Configure()` — initialize API clients
   - `Plan()` — compare desired vs. current state
   - `Apply()` — execute changes
   - `Read()` — refresh state from the API
   - `Delete()` — remove resources
3. **Register** in `internal/provider/registry.go` `LoadProvider` switch
4. **Add examples** under `examples/`

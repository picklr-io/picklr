# Configuration Guide

Picklr uses [PKL](https://pkl-lang.org/) as its configuration language, providing type safety, modularity, and human-readable infrastructure definitions.

## Configuration Structure

A Picklr configuration file amends the `Config.pkl` schema:

```pkl
amends "../../pkg/schemas/Config.pkl"

// Provider-specific resource blocks
aws {
  buckets = new Listing { ... }
  instances = new Listing { ... }
}

// Outputs
outputs {
  ["bucket_arn"] = ptr("aws:S3.Bucket/logs/arn")
}
```

## Resources

Resources are the core building blocks. Each resource has:

| Field | Type | Description |
|-------|------|-------------|
| `type` | String | Resource type (e.g., `aws:S3.Bucket`) |
| `name` | String | Unique name within the type |
| `provider` | String | Provider name (e.g., `aws`, `docker`, `null`) |
| `properties` | Mapping | Resource-specific configuration |
| `dependsOn` | Listing<String> | Explicit dependency addresses |
| `lifecycle` | Lifecycle? | Lifecycle rules |
| `timeout` | String? | Per-resource timeout (e.g., `"10m"`) |

### Typed Resources

Provider schemas define typed resource classes. Using typed resources gives you autocompletion and compile-time validation:

```pkl
import "../../pkg/schemas/aws/S3.pkl"

new S3.Bucket {
  name = "logs"
  bucket = "my-app-logs"
  acl = "private"
}
```

### Generic Resources

You can also define resources generically with explicit properties:

```pkl
new {
  type = "null_resource"
  name = "example"
  provider = "null"
  properties {
    ["key"] = "value"
  }
}
```

## Dependencies

### Implicit Dependencies

Picklr automatically detects dependencies from `ptr://` references. If resource B references an attribute of resource A, B depends on A:

```pkl
new EC2.Subnet {
  name = "public"
  vpcId = "ptr://aws:EC2.Vpc/main/id"  // Implicit dependency on Vpc.main
}
```

### Explicit Dependencies

Use `dependsOn` for dependencies that aren't captured by references:

```pkl
new {
  type = "null_resource"
  name = "step2"
  provider = "null"
  dependsOn = new Listing { "null_resource.step1" }
  properties { ["triggers"] = new Mapping { ["order"] = "second" } }
}
```

## Lifecycle Rules

Control how Picklr handles resource changes:

```pkl
new S3.Bucket {
  name = "critical-data"
  bucket = "important-data-bucket"
  lifecycle {
    preventDestroy = true          // Block deletion
    ignoreChanges = new Listing {  // Ignore specific attribute changes
      "tags"
    }
  }
}
```

| Rule | Description |
|------|-------------|
| `preventDestroy` | Error if the plan would destroy this resource |
| `ignoreChanges` | List of attributes to ignore when computing changes |

## Outputs

Outputs expose values from your infrastructure for use by other tools or configurations:

```pkl
outputs {
  ["vpc_id"] = "ptr://aws:EC2.Vpc/main/id"
  ["app_url"] = "https://myapp.example.com"
}
```

Outputs are saved in the state file and displayed after `picklr apply`.

## External Properties

Pass values into your configuration at runtime using the `-D` flag:

```bash
picklr apply -D environment=prod -D instance_count=3
```

Access these in PKL using external properties:

```pkl
const environment: String = read("prop:environment")
```

## Modules and Reuse

### for_each / count

Expand resources dynamically:

```pkl
new {
  type = "null_resource"
  name = "worker"
  provider = "null"
  count = 3  // Creates worker-0, worker-1, worker-2
  properties {
    ["triggers"] = new Mapping {
      ["index"] = "${count.index}"
    }
  }
}
```

With `for_each`:

```pkl
new {
  type = "aws:S3.Bucket"
  name = "env-bucket"
  provider = "aws"
  forEach = new Mapping {
    ["dev"] = "dev"
    ["staging"] = "staging"
    ["prod"] = "prod"
  }
  properties {
    ["bucket"] = "myapp-${each.value}-assets"
  }
}
```

### Module Schema

Picklr supports a module schema (`Module.pkl`) for creating reusable infrastructure packages:

```pkl
amends "../../pkg/schemas/Module.pkl"

moduleName = "vpc-network"
moduleVersion = "1.0.0"

input {
  ["cidr_block"] = "10.0.0.0/16"
  ["name"] = "main"
}

resources {
  // VPC, subnets, gateways, etc.
}

output {
  ["vpc_id"] = "ptr://aws:EC2.Vpc/main/id"
}
```

## PKL Project Files

Each Picklr project and example uses a `PklProject` file for PKL dependency resolution:

```pkl
amends "pkl:Project"

package {
  name = "my-infra"
  version = "0.1.0"
  baseUri = "package://example.com/my-infra"
}

dependencies {
  ["picklr-schemas"] {
    uri = "package://github.com/picklr-io/picklr/pkg/schemas"
  }
}
```

## Resource Addresses

Resources are identified by their address: `<type>.<name>`:

- `aws:S3.Bucket.logs`
- `aws:EC2.Instance.web-server`
- `null_resource.example`

Addresses are used in:
- `--target` flags
- `dependsOn` lists
- State lookups
- Plan output

## Timeouts

Set per-resource timeouts to control how long Picklr waits for an operation:

```pkl
new EC2.Instance {
  name = "large-db"
  timeout = "15m"  // Override default 30-minute timeout
  // ...
}
```

The default timeout is 30 minutes. Operations that exceed the timeout are cancelled and treated as failures.

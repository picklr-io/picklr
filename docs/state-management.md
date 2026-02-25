# State Management

Picklr tracks the real-world state of your infrastructure in a state file. This document covers how state works, remote backends, locking, encryption, and workspaces.

## How State Works

After each `picklr apply`, Picklr writes a state file that records:

- **Version** — state format version (currently `1`)
- **Serial** — incrementing counter, bumped on every apply
- **Lineage** — UUID generated on `picklr init`, identifies the state's origin
- **Resources** — list of managed resources with their inputs, outputs, and provider
- **Outputs** — key-value outputs defined in your configuration

The state is used during `picklr plan` to compute the diff between your desired configuration and the actual infrastructure.

## Local State

By default, state is stored at `.picklr/state.pkl` relative to your project directory. The file is a valid PKL document that amends the `State.pkl` schema.

Example state file:

```pkl
// Picklr state file
amends "../../pkg/schemas/State.pkl"

version = 1
serial = 3
lineage = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

outputs = new {}

resources {
  new {
    type = "aws:S3.Bucket"
    name = "logs"
    provider = "aws"
    inputs {
      ["bucket"] = "my-app-logs"
      ["acl"] = "private"
    }
    inputsHash = "abc123"
    outputs {
      ["id"] = "my-app-logs"
      ["arn"] = "arn:aws:s3:::my-app-logs"
    }
  }
}
```

## Remote State: S3 Backend

For team workflows, store state remotely in S3 with optional DynamoDB-based locking.

### Configuration

Configure the S3 backend by passing backend configuration when initializing the state manager:

```json
{
  "type": "s3",
  "config": {
    "bucket": "my-picklr-state",
    "key": "prod/state.pkl",
    "region": "us-east-1",
    "dynamodb_table": "picklr-locks",
    "encrypt": "true",
    "profile": "production"
  }
}
```

| Parameter | Required | Default | Description |
|-----------|----------|---------|-------------|
| `bucket` | Yes | — | S3 bucket name |
| `key` | No | `picklr/state.pkl` | S3 object key for the state file |
| `region` | No | `us-east-1` | AWS region |
| `dynamodb_table` | No | — | DynamoDB table for state locking |
| `encrypt` | No | `false` | Enable S3 server-side encryption (AES-256) |
| `profile` | No | — | AWS CLI profile to use |

### DynamoDB Locking

When `dynamodb_table` is set, Picklr uses a DynamoDB table for distributed locking. Create the table with the following schema:

- **Table name:** your chosen name (e.g., `picklr-locks`)
- **Partition key:** `LockID` (String)

```bash
aws dynamodb create-table \
  --table-name picklr-locks \
  --attribute-definitions AttributeName=LockID,AttributeType=S \
  --key-schema AttributeName=LockID,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST
```

The lock uses a conditional write to prevent concurrent modifications. If a lock is stuck, manually delete the item with the matching `LockID` from the DynamoDB table.

### S3 Bucket Setup

Recommended bucket configuration:

```bash
aws s3 mb s3://my-picklr-state --region us-east-1

# Enable versioning for state history
aws s3api put-bucket-versioning \
  --bucket my-picklr-state \
  --versioning-configuration Status=Enabled
```

## State Encryption

Picklr supports client-side AES-256-GCM encryption for state files. This works with both local and remote backends.

### Enabling Encryption

Set the `PICKLR_STATE_ENCRYPTION_KEY` environment variable:

```bash
export PICKLR_STATE_ENCRYPTION_KEY="your-32-character-secret-key!!!"
picklr apply
```

The key must be provided on every operation. If the key is missing when reading an encrypted state, Picklr will return an error.

Encrypted state files are prefixed with `# PICKLR_ENCRYPTED_STATE` and the content is base64-encoded.

### Key Management

- Use a 32-byte key for AES-256 (shorter keys are zero-padded)
- Store the key in a secrets manager (AWS Secrets Manager, HashiCorp Vault, etc.)
- Rotate keys by decrypting with the old key and re-encrypting with the new key

## State Locking

### Local Locking

Local state uses file-based locking (`.picklr/state.pkl.lock`). The lock file contains the PID and timestamp. Locks older than 10 minutes are considered stale and automatically cleared.

### Remote Locking

The S3 backend uses DynamoDB for distributed locking (see above). Without a `dynamodb_table` configured, no locking is performed — this is safe for single-operator use but not recommended for teams.

## Workspaces

Workspaces allow multiple isolated state environments within a single project (e.g., `dev`, `staging`, `prod`).

```bash
picklr workspace new staging
picklr workspace select staging
picklr apply                    # Applies to staging state
picklr workspace select default
picklr apply                    # Applies to default state
```

Each workspace gets its own state file:
- Default: `.picklr/state.pkl`
- Named: `.picklr/state.<name>.pkl`

### Workspace Commands

```bash
picklr workspace list           # Show all workspaces
picklr workspace show           # Show current workspace
picklr workspace new <name>     # Create workspace
picklr workspace select <name>  # Switch workspace
picklr workspace delete <name>  # Delete workspace and its state
```

## Drift Detection

Use `--refresh` with `plan` or `apply` to detect drift between state and real infrastructure:

```bash
picklr plan --refresh
```

Picklr calls each provider's `Read()` method to fetch current resource attributes and compares them against the stored state. Drifted resources are shown before the plan output.

## State Operations

### Inspecting State

Use the interactive console to explore state:

```bash
picklr console
> state.resources
> resource aws:S3.Bucket.logs
> state.outputs
```

### Migrating from Terraform

Convert a Terraform state file to Picklr format:

```bash
picklr migrate terraform --state-file terraform.tfstate
```

This creates `.picklr/state.pkl` with resources mapped from Terraform types to Picklr equivalents.

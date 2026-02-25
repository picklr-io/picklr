# Architecture

This document describes Picklr's internal architecture and data flow.

## Overview

Picklr follows a pipeline architecture:

```
PKL Config → Evaluator → IR (Config) → Engine (Plan/Apply) → Providers → Cloud APIs
                                ↕
                        State Manager ←→ Backend (local/S3)
```

## Data Flow

### 1. Configuration Loading

The PKL evaluator (`internal/eval`) loads `.pkl` files and evaluates them into Go intermediate representation (IR) structs.

```
main.pkl → eval.Evaluator.LoadConfig() → ir.Config
```

The evaluator handles:
- PKL schema validation and type checking
- External property injection (`-D key=value`)
- Dependency resolution via `PklProject` files

### 2. State Loading

The state manager (`internal/state`) loads the current infrastructure state:

```
.picklr/state.pkl → state.Manager.Read() → ir.State
```

State can be stored locally or in S3. The manager handles:
- Encryption/decryption (AES-256-GCM)
- File-based locking (local) or DynamoDB locking (S3)
- Workspace-specific state files

### 3. Planning

The engine (`internal/engine`) compares desired config with current state:

```
(ir.Config, ir.State) → engine.CreatePlan() → ir.Plan
```

Planning involves:
1. **Provider loading** — ensure all referenced providers are available
2. **for_each/count expansion** — expand dynamic resources
3. **Dependency graph** — topological sort of resources via `BuildDAG()`
4. **Per-resource planning** — call each provider's `Plan()` method
5. **Lifecycle enforcement** — check `preventDestroy`, `ignoreChanges`
6. **Deletion detection** — find resources in state but not in config

### 4. Applying

The engine executes the plan:

```
(ir.Plan, ir.State) → engine.ApplyPlan() → ir.State (updated)
```

Apply features:
- **Parallel execution** — up to 10 concurrent operations with dependency ordering
- **Retry with backoff** — transient cloud API errors are retried (3 attempts, exponential backoff)
- **Per-resource timeouts** — default 30 minutes, configurable per resource
- **Continue-on-error** — optional mode to apply remaining resources despite failures
- **Progress callbacks** — real-time events for CLI rendering

### 5. State Persistence

After a successful apply, the updated state is written back:

```
ir.State → state.Manager.Write() → .picklr/state.pkl
```

The serial number is incremented on every write.

## Key Types

### `ir.Config`
Top-level configuration with resources and outputs.

### `ir.Resource`
A single managed resource:
- `Type` — namespaced resource type (e.g., `aws:S3.Bucket`)
- `Name` — unique name
- `Provider` — provider name
- `Properties` — resource configuration as `map[string]any`
- `DependsOn` — explicit dependency addresses
- `Lifecycle` — lifecycle rules
- `Timeout` — operation timeout

### `ir.State`
Persistent infrastructure state:
- `Version` — format version
- `Serial` — monotonic counter
- `Lineage` — UUID identifying the state origin
- `Resources` — list of `ResourceState`
- `Outputs` — configuration outputs

### `ir.Plan`
Execution plan:
- `Changes` — list of `ResourceChange`
- `Summary` — counts by action type
- `Outputs` — desired outputs
- `Metadata` — timestamp

### `ir.ResourceChange`
A planned change:
- `Address` — resource address
- `Action` — CREATE, UPDATE, DELETE, REPLACE, NOOP
- `Desired` — desired resource config
- `Prior` — prior resource config
- `Diff` — property-level diff

## Dependency Graph

The `graph.go` module builds a directed acyclic graph (DAG) from resource dependencies:

- **Implicit deps:** extracted from `ptr://` references in properties
- **Explicit deps:** from the `dependsOn` field

The DAG provides:
- `CreationOrder()` — topological sort for creation/updates
- `TransitiveDeps(addr)` — all transitive dependencies of a resource

## Provider Architecture

Providers implement the `ProviderServer` gRPC interface defined in `proto/provider/provider.proto`. Currently, providers are compiled into the binary (not external plugins).

### Provider Registry

The registry (`internal/provider/registry.go`) maps provider names to implementations:

```go
switch name {
case "null":   // → providers/null
case "docker": // → providers/docker
case "aws":    // → providers/aws
}
```

### AWS Provider Structure

The AWS provider is organized by service:
- `providers/aws/provider.go` — main provider, routing to service handlers
- `providers/aws/s3.go` — S3 bucket operations
- `providers/aws/ec2.go` — EC2 instance, VPC, subnet operations
- `providers/aws/iam.go` — IAM role, policy operations
- etc.

Each service file implements `apply<Resource>()` and optionally `plan<Resource>()` functions.

## Error Handling

- Errors are wrapped with context using `fmt.Errorf("context: %w", err)`
- Transient cloud errors (throttling, timeouts) are retried automatically
- Apply writes partial state on failure to prevent losing successful changes
- Continue-on-error mode collects all errors and returns an aggregate

## Audit Logging

Operations are logged to `.picklr/audit.log` in JSONL format:

```json
{"operation":"apply","timestamp":"...","changes":[...],"summary":{...}}
```

## Structured Logging

Internal logging uses Go's `slog` package with configurable levels (debug, info, warn, error). Log output goes to stderr to keep stdout clean for user-facing output.

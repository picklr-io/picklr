# CLAUDE.md

## Project Overview

Picklr is a type-safe Infrastructure as Code (IaC) tool built on Apple's [PKL](https://pkl-lang.org/) configuration language and Go. It provides a declarative way to manage cloud infrastructure with type-safe resource definitions, Git-native state management, and human-readable plans and state files.

**Module path:** `github.com/picklr-io/picklr`
**Go version:** 1.24.1
**Key dependencies:** pkl-go, aws-sdk-go-v2, docker/docker, cobra, protobuf/gRPC, testify

## Repository Structure

```
cmd/picklr/          # CLI entrypoint (main.go)
internal/
  cli/               # Cobra CLI commands: init, validate, plan, apply, destroy
  engine/            # Core orchestration: plan creation and apply execution
  eval/              # PKL evaluator: loads .pkl configs/state into Go IR structs
  ir/                # Intermediate Representation: Config, Resource, State, Plan types
  provider/          # Provider registry: loads and manages provider instances
  state/             # State manager: reads/writes .picklr/state.pkl files
providers/
  aws/               # AWS provider (S3, EC2, IAM, Lambda, RDS, ECS, etc.)
  docker/            # Docker provider (containers, networks, volumes, images)
  null/              # Null provider (testing/noop)
pkg/
  schemas/           # PKL schema definitions (Config.pkl, Resource.pkl, State.pkl, Plan.pkl)
    aws/             # AWS resource PKL schemas (EC2.pkl, S3.pkl, IAM.pkl, etc.)
    docker/          # Docker resource PKL schemas
    null/            # Null resource PKL schema
  proto/provider/    # Generated protobuf/gRPC code for provider interface
proto/provider/      # Protobuf source: provider.proto
examples/            # Example PKL configurations for all providers
```

## Architecture

### Data Flow

1. **PKL config** (`main.pkl`) is evaluated by `eval.Evaluator` into `ir.Config`
2. **State** (`.picklr/state.pkl`) is loaded by `state.Manager` into `ir.State`
3. **Engine** (`engine.Engine`) compares desired config with current state to produce `ir.Plan`
4. **Providers** implement the `ProviderServer` gRPC interface (Plan/Apply/Read/Delete)
5. **State** is written back after successful apply

### Key Types (internal/ir/)

- `Config` — top-level configuration with resources and outputs
- `Resource` — a single managed resource (type, name, provider, properties)
- `State` — persistent infrastructure state (version, serial, resources, outputs)
- `ResourceState` — state of a single resource (inputs, outputs, hashes)
- `Plan` — execution plan with changes and summary
- `ResourceChange` — a planned action (CREATE, UPDATE, DELETE, REPLACE, NOOP)

### Provider Interface (proto/provider/provider.proto)

All providers implement the `ProviderServer` interface:
- `GetSchema` — returns PKL schema for the provider
- `Configure` — initializes the provider with credentials/config
- `Plan` — compares desired vs. prior state, returns planned action
- `Apply` — executes the planned change, returns new state
- `Read` — refreshes resource state from the real infrastructure
- `Delete` — removes a resource

Providers are currently compiled-in (not plugins). The registry in `internal/provider/registry.go` maps provider names ("null", "docker", "aws") to implementations.

### Resource Type Naming

Resource types use a namespaced format: `<provider>:<Service>.<Resource>`
- AWS: `aws:S3.Bucket`, `aws:EC2.Instance`, `aws:IAM.Role`, `aws:Lambda.Function`
- Docker: `docker_container`, `docker_network`, `docker_volume`, `docker_image`
- Null: `null_resource`

### Resource References (ptr://)

Cross-resource references use the `ptr://` protocol:
```
ptr://<provider>:<Type>/<name>/<attribute>
```
Example: `ptr://aws:EC2.Vpc/my-vpc/id` resolves at apply time from state outputs.

### PKL Schema Pattern

Each provider's PKL resources extend `Resource.Resource` and define:
- Fixed `provider` field (e.g., `provider = "aws"`)
- Typed fields for the resource's configuration
- A `properties` mapping that serializes typed fields to key-value pairs for the engine

Example pattern (from `pkg/schemas/aws/S3.pkl`):
```pkl
class Bucket extends Resource.Resource {
  provider = "aws"
  bucket: String?
  acl: String = "private"
  properties = new {
    ["bucket"] = bucket
    ["acl"] = acl
  }
}
```

## CLI Commands

| Command | Description |
|---------|-------------|
| `picklr init` | Creates `.picklr/` directory, `main.pkl` template, and empty state file |
| `picklr validate [path]` | Validates PKL configuration syntax and types |
| `picklr plan [path]` | Generates execution plan (diff desired vs. current state) |
| `picklr apply [path]` | Applies changes to infrastructure (with confirmation prompt) |
| `picklr destroy` | Destroys all managed resources (currently a stub/TODO) |

Commands accept an optional path argument (directory or file). Defaults to `main.pkl` in the current working directory.

## Build and Development

### Building

```bash
go build -o picklr ./cmd/picklr
```

### Running Tests

```bash
go test ./...
```

Test files follow Go conventions (`*_test.go` alongside source). Key test locations:
- `internal/engine/engine_test.go` — plan creation tests with null provider
- `internal/state/state_test.go` — state read/write round-trip tests
- `providers/null/provider_test.go` — null provider Plan/Apply tests
- `internal/eval/evaluator_test.go` — evaluator compilation test (PKL evaluation tests are limited due to local dependency resolution)

Tests use `github.com/stretchr/testify` (assert/require).

### Protobuf Generation

Source: `proto/provider/provider.proto`
Generated output: `pkg/proto/provider/`

```bash
protoc --go_out=. --go-grpc_out=. proto/provider/provider.proto
```

### Validating Examples

Each example under `examples/` has its own `PklProject` file. Validate with:
```bash
picklr validate examples/<example-name>/main.pkl
```

## Code Conventions

### Go Patterns

- **CLI framework:** Cobra (`github.com/spf13/cobra`). Commands are in `internal/cli/`, registered in `root.go`'s `init()`.
- **Error handling:** Wrap errors with `fmt.Errorf("context: %w", err)`. Return errors up to CLI layer.
- **Provider pattern:** Each provider file handles one AWS service. Apply functions follow `apply<Resource>(ctx, req)` naming. Plan functions follow `plan<Resource>(ctx, req)` naming.
- **JSON marshaling:** Provider data exchange uses JSON-encoded protobuf bytes. Internal config/state use `map[string]any` for dynamic properties.
- **No Makefile:** Build commands run directly with `go build`, `go test`, etc.

### PKL Patterns

- Config files use `amends "../../pkg/schemas/Config.pkl"` to inherit the schema
- Each example has a `PklProject` file for PKL dependency resolution
- The root schemas PklProject is at `pkg/schemas/PklProject`
- Resources are declared under provider-specific blocks (e.g., `aws { buckets = new Listing { ... } }`)

### Adding a New AWS Resource

1. Create/extend the PKL schema in `pkg/schemas/aws/<Service>.pkl` extending `Resource.Resource`
2. Add the typed listing field to `pkg/schemas/aws/Provider.pkl` Config class
3. Add the resource to `allResources` flattening in `Provider.pkl`
4. Add `apply<Resource>` and optionally `plan<Resource>` functions in `providers/aws/<service>.go`
5. Register the resource type in the `Apply` and `Plan` switch statements in `providers/aws/provider.go`
6. Add an example under `examples/`

### Adding a New Provider

1. Create PKL schemas under `pkg/schemas/<provider>/`
2. Implement the `ProviderServer` interface in `providers/<provider>/provider.go`
3. Register in `internal/provider/registry.go` LoadProvider switch
4. Add provider config to `pkg/schemas/Config.pkl`

## State Management

- State is stored at `.picklr/state.pkl` relative to the project directory
- State files are PKL that amend the `State.pkl` schema
- State serialization is basic (TODOs exist for proper PKL generation of complex types)
- The `serial` field increments on each apply
- Resource addresses follow the pattern `<type>.<name>` (e.g., `null_resource.my_test`)

## Known Limitations and TODOs

- `destroy` command is a stub — not yet implemented
- Plan output does not render detailed property diffs (shows `...` placeholder)
- Plan-to-file output (`--out` flag) is not yet implemented
- State serialization of complex nested types (maps, lists) falls back to `"<complex type>"`
- PKL evaluator tests are limited because local PKL dependencies are hard to resolve without publishing
- Provider loading is compiled-in; no plugin system yet (future: go-plugin)
- Engine apply is synchronous/atomic — no granular progress events

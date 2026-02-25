# Getting Started

This guide walks you through installing Picklr, creating your first project, and deploying infrastructure.

## Prerequisites

- **Go 1.24+** — Picklr is built with Go
- **PKL CLI** — Install from [pkl-lang.org](https://pkl-lang.org/main/current/pkl-cli/index.html)
- **Cloud credentials** — AWS credentials configured (for AWS resources)

## Installation

```bash
go install github.com/picklr-io/picklr/cmd/picklr@latest
```

Or build from source:

```bash
git clone https://github.com/picklr-io/picklr.git
cd picklr
go build -o picklr ./cmd/picklr
```

## Initialize a Project

Create a new Picklr project directory and initialize it:

```bash
mkdir my-infra && cd my-infra
picklr init
```

This creates:
- `.picklr/` — project state directory
- `.picklr/state.pkl` — empty state file with a unique lineage UUID
- `main.pkl` — starter configuration template

## Write Your First Configuration

Edit `main.pkl` to define resources. Here's a minimal example using the null provider (no cloud credentials required):

```pkl
amends "../../pkg/schemas/Config.pkl"

resources {
  new {
    type = "null_resource"
    name = "hello"
    provider = "null"
    properties {
      ["triggers"] = new Mapping {
        ["message"] = "Hello from Picklr!"
      }
    }
  }
}
```

For an AWS S3 bucket:

```pkl
amends "../../pkg/schemas/Config.pkl"

import "../../pkg/schemas/aws/S3.pkl"

aws {
  buckets = new Listing {
    new S3.Bucket {
      name = "my-app-logs"
      bucket = "my-company-app-logs-2025"
      acl = "private"
    }
  }
}
```

## Validate Configuration

Check your PKL configuration for syntax and type errors:

```bash
picklr validate
```

Or validate a specific file:

```bash
picklr validate path/to/main.pkl
```

## Preview Changes

Generate an execution plan to see what Picklr will do:

```bash
picklr plan
```

The plan output shows:
- Resources to **create** (green `+`)
- Resources to **update** (yellow `~`)
- Resources to **replace** (yellow `±`)
- Resources to **delete** (red `-`)

## Apply Changes

Apply the plan to create or modify infrastructure:

```bash
picklr apply
```

Picklr will:
1. Load your configuration
2. Compare it against the current state
3. Show the execution plan
4. Ask for confirmation
5. Apply changes and update the state file

Use `--auto-approve` to skip the confirmation prompt (useful in CI):

```bash
picklr apply --auto-approve
```

## Destroy Resources

Remove all managed resources:

```bash
picklr destroy
```

This generates a "delete all" plan and, after confirmation, removes every resource tracked in state.

## What's Next

- [CLI Reference](./cli-reference.md) — Full command and flag documentation
- [State Management](./state-management.md) — Local and remote state, locking, encryption
- [Providers](./providers.md) — AWS, Docker, and null provider details
- [Configuration Guide](./configuration.md) — PKL patterns, modules, and references

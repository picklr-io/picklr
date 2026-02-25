# CI/CD Integration

Picklr integrates with CI/CD pipelines through JSON output, GitHub Actions, and policy checks.

## GitHub Actions

Picklr provides an official GitHub Action (`action.yml`) for use in workflows.

### Basic Usage

```yaml
name: Infrastructure
on:
  pull_request:
  push:
    branches: [main]

jobs:
  plan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: picklr-io/picklr@main
        with:
          command: plan
          working-directory: ./infrastructure

  apply:
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/main'
    needs: plan
    steps:
      - uses: actions/checkout@v4

      - uses: picklr-io/picklr@main
        with:
          command: apply
          auto-approve: true
          working-directory: ./infrastructure
```

### Action Inputs

| Input | Default | Description |
|-------|---------|-------------|
| `command` | `plan` | Picklr command to run (`plan`, `apply`, `destroy`) |
| `working-directory` | `.` | Working directory for the command |
| `auto-approve` | `false` | Skip confirmation for apply/destroy |
| `version` | `latest` | Picklr version to install |
| `targets` | — | Comma-separated resource targets |

### Action Outputs

| Output | Description |
|--------|-------------|
| `plan-json` | JSON plan output (when using `plan` command) |
| `exit-code` | Exit code of the command |

### PR Comments

When running on a `pull_request` event, the action automatically posts the plan output as a PR comment, giving reviewers visibility into infrastructure changes.

## JSON Output for Pipelines

All commands support `--json` for machine-readable output:

```bash
# Save plan to file
picklr plan --json > plan.json

# Apply a saved plan
picklr apply plan.json --auto-approve --json
```

The JSON output includes structured data suitable for parsing with `jq` or programmatic consumption:

```bash
# Count resources to be created
picklr plan --json | jq '.summary.create'

# List addresses of changes
picklr plan --json | jq '.changes[].address'
```

## Policy Checks

Run policy checks against a plan before applying:

```bash
# Generate plan
picklr plan --json > plan.json

# Check against policies
picklr policy-check plan.json
```

### Policy File Format

```json
{
  "rules": [
    {
      "name": "no-public-buckets",
      "description": "S3 buckets must not be public",
      "resource_type": "aws:S3.Bucket",
      "type": "property_not_equals",
      "property": "acl",
      "value": "public-read",
      "severity": "error"
    },
    {
      "name": "require-tags",
      "description": "All EC2 instances must have Name tag",
      "resource_type": "aws:EC2.Instance",
      "type": "require_property",
      "property": "tags.Name",
      "severity": "warning"
    },
    {
      "name": "no-destroy-prod",
      "description": "Cannot destroy resources in production",
      "type": "deny_action",
      "action": "DELETE",
      "severity": "error"
    }
  ]
}
```

### Policy Types

| Type | Description |
|------|-------------|
| `deny_action` | Block specific actions (CREATE, UPDATE, DELETE, REPLACE) |
| `property_equals` | Require a property to have a specific value |
| `property_not_equals` | Require a property to NOT have a specific value |
| `require_property` | Require a property to be present |

### Severity Levels

- **error** — Fails the policy check (non-zero exit code)
- **warning** — Prints a warning but doesn't fail

## Pipeline Patterns

### Plan on PR, Apply on Merge

```yaml
# On PR: plan and comment
plan:
  if: github.event_name == 'pull_request'
  steps:
    - run: picklr plan --json > plan.json
    - run: picklr policy-check plan.json

# On merge to main: apply
apply:
  if: github.ref == 'refs/heads/main'
  steps:
    - run: picklr apply --auto-approve --json
```

### Multi-Environment

```yaml
strategy:
  matrix:
    environment: [dev, staging, prod]

steps:
  - run: |
      picklr workspace select ${{ matrix.environment }}
      picklr apply --auto-approve
```

### Drift Detection

Schedule periodic drift checks:

```yaml
on:
  schedule:
    - cron: '0 */6 * * *'  # Every 6 hours

jobs:
  drift:
    steps:
      - run: picklr plan --refresh --json
      - if: steps.plan.outputs.exit-code == '2'
        run: echo "Drift detected!" && exit 1
```

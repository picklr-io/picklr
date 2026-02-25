# CLI Reference

Picklr provides a set of commands to manage infrastructure lifecycle. All commands accept an optional path argument pointing to a directory or `.pkl` file; the default is `main.pkl` in the current working directory.

## Global Flags

| Flag | Description |
|------|-------------|
| `--target <address>` | Target specific resources (repeatable). Dependencies are included automatically. |

## Commands

### `picklr init`

Initialize a new Picklr project.

```bash
picklr init
```

Creates:
- `.picklr/` directory
- `.picklr/state.pkl` with a generated lineage UUID
- `main.pkl` configuration template

### `picklr validate [path]`

Validate PKL configuration syntax and types.

```bash
picklr validate
picklr validate ./environments/prod/
picklr validate ./main.pkl
```

### `picklr plan [path]`

Generate an execution plan comparing desired configuration with current state.

```bash
picklr plan
picklr plan --refresh          # Refresh state from providers before planning
picklr plan --json             # Output plan as JSON
picklr plan --target aws:S3.Bucket.logs
```

| Flag | Description |
|------|-------------|
| `--refresh` | Refresh resource state from providers before planning (drift detection) |
| `--json` | Output the plan in JSON format |
| `-D key=value` | Set external properties passed to the PKL configuration |

### `picklr apply [path]`

Apply configuration changes to infrastructure.

```bash
picklr apply
picklr apply --auto-approve
picklr apply --on-error continue
picklr apply saved-plan.json    # Apply a previously saved plan
```

| Flag | Description |
|------|-------------|
| `--auto-approve` | Skip interactive confirmation |
| `--refresh` | Refresh state before applying |
| `--json` | Output results in JSON format |
| `--on-error <mode>` | Error handling: `fail` (default, stop on first error) or `continue` (apply remaining resources) |
| `-D key=value` | Set external properties |

When `--on-error continue` is set, Picklr will attempt to apply all independent resources even if some fail. Resources that depend on a failed resource are automatically skipped. The partial state is always saved.

### `picklr destroy`

Destroy all managed resources.

```bash
picklr destroy
picklr destroy --auto-approve
picklr destroy --on-error continue
```

| Flag | Description |
|------|-------------|
| `--auto-approve` | Skip interactive confirmation |
| `--json` | Output results in JSON format |
| `--on-error <mode>` | Error handling: `fail` (default) or `continue` |

### `picklr fmt [path]`

Format PKL configuration files.

```bash
picklr fmt
picklr fmt --check             # Check formatting without writing
picklr fmt ./environments/
```

| Flag | Description |
|------|-------------|
| `--check` | Verify formatting without modifying files (exit code 1 if unformatted) |
| `--write` | Write formatted output (default: true) |

### `picklr console`

Launch an interactive REPL for inspecting state and configuration.

```bash
picklr console
```

Available console commands:

| Command | Description |
|---------|-------------|
| `state` | Show full state |
| `state.resources` | List resources in state |
| `state.outputs` | Show state outputs |
| `resource <address>` | Show details of a specific resource |
| `output <name>` | Show a specific output value |
| `config` | Show loaded configuration |
| `config.resources` | List configured resources |
| `json` | Output current state as JSON |
| `help` | Show available commands |
| `exit` | Exit the console |

### `picklr workspace <subcommand>`

Manage workspaces for isolated state environments.

```bash
picklr workspace list          # List all workspaces
picklr workspace new staging   # Create a new workspace
picklr workspace select staging # Switch to a workspace
picklr workspace delete staging # Delete a workspace
picklr workspace show          # Show current workspace
```

Each workspace has its own state file (`state.<name>.pkl`) and lineage UUID.

### `picklr policy-check [plan-file]`

Check a plan against policy rules.

```bash
picklr policy-check plan.json
```

Policies are defined in a JSON file with rules that can:
- Deny specific actions (`deny_action`)
- Require property values (`property_equals`, `property_not_equals`)
- Require properties to be set (`require_property`)

### `picklr migrate terraform`

Migrate from Terraform to Picklr.

```bash
picklr migrate terraform
picklr migrate terraform --state-file ./terraform.tfstate
```

Reads `terraform.tfstate` and converts it to a Picklr state file, mapping Terraform resource types to Picklr equivalents.

## JSON Output

All plan/apply/destroy commands support `--json` for structured output suitable for CI pipelines and tooling integration. The JSON output includes:

- Plan changes with actions and property diffs
- Summary counts (create, update, delete, replace, noop)
- Resource state after apply
- Outputs

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (configuration, provider, or apply failure) |
| 2 | Plan has changes (useful for CI: `picklr plan` exits 2 if drift is detected) |

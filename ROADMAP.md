# Picklr Roadmap: Terraform Parity

This document catalogs every gap between Picklr's current implementation and a
production-ready IaC tool competitive with Terraform/OpenTofu. Items are grouped
into phases ordered by criticality — later phases assume earlier ones are done.

---

## Phase 0 — Critical Bugs (must fix before anything else)

These are correctness issues in the current code that will silently lose data or
produce wrong results.

### 0.1 State is never persisted after apply
`internal/cli/apply.go` calls `eng.ApplyPlan()` but never calls
`stateMgr.Write()`. Every apply produces the correct in-memory state and then
throws it away. The next plan will always show CREATE for everything.

### 0.2 State serialization drops inputs and outputs
`internal/state/state.go:99-103` writes `inputs = new {}` and
`outputs = new {}` regardless of actual data. Even once 0.1 is fixed, all
resource attributes are lost on round-trip.

### 0.3 State append without dedup on UPDATE/REPLACE
`internal/engine/apply.go:104` unconditionally appends `newState` to
`state.Resources`. On UPDATE or REPLACE this creates a duplicate entry instead
of replacing the existing one.

### 0.4 DELETE action is a no-op in the engine
`internal/engine/apply.go:106-108` has `case "DELETE":` with only a comment.
No provider `Delete()` call is made and the resource is not removed from state.

### 0.5 Provider auto-loading is broken
`internal/cli/plan.go:69` and `internal/cli/apply.go:64` hardcode
`registry.LoadProvider("null")`. If the config references `aws` or `docker`
resources, the engine cannot find those providers.  The engine's `CreatePlan`
does call `LoadProvider` per resource, but the CLI never loads non-null
providers either — this needs to be unified so the engine handles it fully.

### 0.6 Prior state not forwarded during apply
`internal/engine/apply.go:81` leaves `priorJSON` nil for all actions. On
UPDATE/REPLACE the provider receives no prior state, so it cannot do
incremental updates or idempotent re-creation.

### 0.7 Debug print left in production code
`internal/cli/plan.go:61` — `fmt.Printf("DEBUG: wd=%s, …")` prints to stdout
on every plan invocation.

### 0.8 Duplicate client init in AWS provider
`providers/aws/provider.go:139-146` initializes codebuild, codecommit,
codedeploy, and codepipeline clients twice.

---

## Phase 1 — Core Engine Completeness

These features are table-stakes for any IaC tool.

### 1.1 Dependency graph & topological sort
Currently resources are planned and applied in array order. Implement a DAG
built from `ir.Resource.DependsOn` fields **and** implicit `ptr://` references
extracted from property values. Apply must walk the graph in topological order;
destroy must walk it in reverse.

### 1.2 Lifecycle rule enforcement
`ir.Resource.Lifecycle` already has `CreateBeforeDestroy`, `PreventDestroy`,
and `IgnoreChanges` fields, but the engine never checks them. Wire these into
plan generation and apply execution:
- `PreventDestroy` — error if the plan includes a DELETE for this resource
- `CreateBeforeDestroy` — on REPLACE, create the new resource before deleting
  the old one
- `IgnoreChanges` — strip listed attributes before diffing in Plan()

### 1.3 Property diff rendering in plan output
`internal/cli/plan.go:145-147` shows `...` as a placeholder. Implement a
recursive diff of `Desired.Properties` vs `Prior.Properties` with colorized
`+`/`-`/`~` lines (like `terraform plan`). The `ir.PropertyDiff` struct
already exists but is never populated.

### 1.4 Save and load plans
The `--out` flag is declared (`planOutFile`) but never used. Serialize the
`ir.Plan` to a file (JSON or PKL) so that `picklr apply <plan-file>` can
execute a pre-approved plan without re-calculating.

### 1.5 Proper PKL state serializer
Replace the `fmt.Fprintf` template in `state.go` with a recursive PKL
generator that correctly serializes:
- Nested maps (`map[string]any`)
- Lists (`[]any`)
- Typed scalars (string, int, float, bool)
- Null/nil values

### 1.6 Implement the `destroy` command
`internal/cli/destroy.go` is a stub. Implementation:
1. Read state
2. Build the dependency graph from state resources
3. Generate a plan where every resource is DELETE (reverse topo order)
4. Prompt for confirmation
5. Execute via `ApplyPlan` (or per-resource Delete calls)
6. Write empty state

### 1.7 State write after apply
Add `stateMgr.Write(ctx, newState)` call at the end of `runApply`. Also add
partial-state write on failure (write whatever succeeded so far, matching
Terraform's behavior on partial failure).

---

## Phase 2 — State Management

### 2.1 State locking
Add a `Lock()`/`Unlock()` interface to state backends. For local state, use
`flock`/`lockfile`. For remote backends, use conditional writes or lease-based
locks (e.g. DynamoDB for S3 backend).

### 2.2 Remote state backends
Implement a `Backend` interface with at least:
- **Local** (current, default)
- **S3 + DynamoDB** (most popular Terraform backend)
- **GCS** (Google Cloud)
- **HTTP** (generic REST backend)

Configuration via a `backend` block in PKL config or `.picklr/backend.pkl`.

### 2.3 State lineage & UUID
Generate a UUID on `picklr init` and store it as `lineage`. Use it to prevent
accidentally applying state from a different project.

### 2.4 State import (`picklr import`)
New CLI command: `picklr import <resource-address> <cloud-id>`. Calls the
provider's `Read()` method and inserts the result into state without modifying
the actual infrastructure.

### 2.5 State manipulation commands
```
picklr state list          # list resources in state
picklr state show <addr>   # show attributes of a single resource
picklr state mv <src> <dst> # rename/move a resource address
picklr state rm <addr>     # remove a resource from state (no destroy)
picklr state pull          # download remote state to local file
picklr state push          # upload local state to remote backend
```

### 2.6 State encryption at rest
Encrypt the state file (AES-256-GCM) with a key from environment variable,
KMS, or Vault. Important because state often contains secrets.

---

## Phase 3 — Drift Detection & Refresh

### 3.1 `picklr refresh`
New CLI command that calls `Read()` on every resource in state, updates state
with what actually exists in the cloud, and reports any drift.

### 3.2 Auto-refresh before plan
Add a `--refresh=true` (default) flag to `plan` and `apply`. When enabled,
refresh state before diffing. Add `--refresh=false` for offline plans.

### 3.3 Drift detection in plan output
When refreshed state differs from stored state, display a separate "drift"
section before the plan changes. Mark resources as "drifted" so the user
understands the discrepancy.

---

## Phase 4 — CLI Feature Parity

### 4.1 `--target` flag
Allow `picklr plan --target=aws:S3.Bucket.my-bucket` to restrict operations to
a subset of resources (and their dependencies).

### 4.2 JSON output mode
`--json` or `-json` flag on `plan`, `apply`, `destroy`, `output` commands.
Emit structured JSON instead of human-readable text, for CI/CD integration.

### 4.3 `--no-color` flag
Disable ANSI color codes. Important for CI logs and piped output.

### 4.4 `picklr output` command
Print outputs from the current state.

### 4.5 `picklr show` command
Print the full current state in human-readable form.

### 4.6 `picklr graph` command
Emit the dependency graph in DOT format so users can visualize with Graphviz.

### 4.7 `picklr fmt` command
Format `.pkl` files consistently (delegate to `pkl format` if available, or
implement a basic formatter).

### 4.8 `picklr console` (REPL)
Interactive console for evaluating PKL expressions against the current state
and config. Lower priority but a nice differentiator.

### 4.9 `picklr taint` / `picklr untaint`
Force a resource to be recreated on next apply (taint) or undo that (untaint).

### 4.10 `picklr workspace` (multi-environment)
Support named workspaces (dev, staging, prod) with separate state files.
Equivalent to Terraform workspaces.

---

## Phase 5 — Provider System

### 5.1 Plugin-based provider loading
Replace the compiled-in `switch` in `registry.go` with a plugin protocol
(HashiCorp's `go-plugin` over gRPC, matching the existing proto definition).
This allows third-party providers.

### 5.2 Provider versioning & lock file
Add a `.picklr.lock` file that pins provider versions. On `picklr init`,
resolve provider versions and write the lock. On subsequent runs, verify.

### 5.3 Provider registry
A central registry (like `registry.terraform.io`) where providers are
published and discovered. Start with a simple GitHub-based approach.

### 5.4 Provider configuration from PKL
Currently the AWS provider hardcodes `us-east-1`. Wire the `Configure()` RPC
so that PKL config blocks like:
```pkl
providers {
  aws {
    region = "eu-west-1"
    profile = "production"
  }
}
```
are passed through to the provider.

### 5.5 Per-resource Plan() implementations
Most AWS resource types fall through to the naive
`string(desired) != string(prior)` comparison in `provider.go:182-195`. Each
resource type should have its own `plan<Resource>()` with proper semantic
diffing (only S3.Bucket and EC2.Instance have custom plan logic today).

---

## Phase 6 — Execution & Reliability

### 6.1 Parallel apply
Apply independent resources concurrently. Use the dependency graph to determine
which resources can be applied in parallel (no dependency edges between them).
Terraform defaults to 10 concurrent operations.

### 6.2 Progress events / streaming output
Refactor `Engine.ApplyPlan` to emit per-resource progress events via a
callback or channel. The CLI can then render live output like:
```
aws:S3.Bucket.logs: Creating...
aws:S3.Bucket.logs: Creation complete after 2s [id=my-logs-bucket]
aws:EC2.Instance.web: Creating...
```

### 6.3 Error recovery & partial state
On failure during apply, write state for resources that succeeded. Currently
if any resource fails, the entire state update is lost.

### 6.4 Timeouts
Add per-resource timeout configuration. If a provider apply exceeds the
timeout, cancel the context and report the failure.

### 6.5 Retry logic
Add configurable retry policies for transient cloud API errors (throttling,
network timeouts). Exponential backoff with jitter.

---

## Phase 7 — Modules & Reuse

### 7.1 PKL module system
Leverage PKL's native module system for reusable infrastructure patterns.
Define a convention for Picklr modules:
```pkl
// modules/vpc/main.pkl
amends "picklr:Module"

input {
  cidr: String
  azCount: Int = 3
}

resources { ... }
outputs { ... }
```

### 7.2 Module registry
Allow modules to be published and consumed from a registry (similar to
Terraform Registry). PKL's package system (`package://`) is a natural fit.

### 7.3 `for_each` / `count` equivalent
PKL's `Listing` and `Mapping` types + comprehensions provide this natively,
but the engine needs to flatten them into individual resources with unique
addresses (e.g., `aws:S3.Bucket.bucket["prod"]`).

---

## Phase 8 — Security & Compliance

### 8.1 Sensitive value masking
Mark certain outputs and properties as sensitive. Mask them in plan output,
logs, and state display. The `PropertyDiff.Sensitive` field exists but is
unused.

### 8.2 Policy as code
Integrate OPA/Rego or PKL-based policy checks that run before apply.
Example: "no S3 buckets with public ACLs", "all EC2 instances must have
tags".

### 8.3 Audit logging
Log all apply/destroy operations with timestamps, user identity, and resource
changes. Write to a configurable audit backend.

---

## Phase 9 — Testing & Quality

### 9.1 Expand unit test coverage
Priority test areas:
- Engine apply (currently untested) — especially UPDATE, DELETE, REPLACE paths
- State serialization round-trip with real data (nested maps, lists)
- Provider auto-loading across all three providers
- Dependency graph construction and topological sort
- Lifecycle rule enforcement
- `ptr://` reference resolution

### 9.2 Integration tests with LocalStack
Add integration tests that run against LocalStack for AWS resources. This
validates the full flow: config → plan → apply → state → plan (no-op).

### 9.3 End-to-end CLI tests
Test the full CLI binary: `picklr init`, `validate`, `plan`, `apply`,
`destroy`. Use `testscript` or shell-based test harness.

### 9.4 Provider conformance tests
Create a test suite that any provider must pass (similar to Terraform's
acceptance test framework). Tests Plan/Apply/Read/Delete lifecycle.

---

## Phase 10 — Developer Experience & Ecosystem

### 10.1 LSP / IDE support
PKL already has LSP support. Ensure Picklr schemas work well with it —
autocomplete for resource types, property validation, inline docs.

### 10.2 Documentation site
Generate reference docs from PKL schemas. Host a documentation site with:
- Getting started guide
- Provider reference (all resource types and properties)
- CLI reference
- Examples gallery

### 10.3 GitHub Actions / CI integration
Publish a `picklr-action` GitHub Action for plan-on-PR, apply-on-merge
workflows. Include plan comment on PRs.

### 10.4 Migration tool
`picklr migrate-from-terraform` — convert Terraform HCL + state to Picklr
PKL + state. Even a partial converter would lower the adoption barrier.

### 10.5 Structured logging & debug mode
Add `--log-level` flag (debug, info, warn, error). In debug mode, show full
provider requests/responses. Use `slog` for structured output.

---

## Competitive Differentiators (Picklr's Advantages)

While catching up to Terraform on features, lean into what makes Picklr unique:

1. **Type safety via PKL** — Terraform's HCL is loosely typed. PKL catches
   errors at config evaluation time, before any API calls.
2. **Git-native state** — PKL state files are human-readable and diffable in
   Git, unlike Terraform's JSON state blobs.
3. **Unified language** — Config, plans, and state are all PKL. No context
   switching between HCL, JSON, and plan output formats.
4. **No DSL** — PKL is a real programming language with functions, classes,
   and a module system. No need for workarounds like Terraform's
   `templatefile()`, `for_each` hacks, or external `terragrunt`.
5. **Simpler provider interface** — The gRPC/protobuf provider protocol is
   straightforward. Building a new provider is significantly less code than
   a Terraform provider.

---

## Suggested Priority Order

| Priority | Items | Rationale |
|----------|-------|-----------|
| **P0 — Now** | Phase 0 (all bugs) + 1.7 (state write) | Nothing works correctly without these |
| **P1 — Next** | 1.1, 1.3, 1.5, 1.6 | Core engine must be complete |
| **P2 — Soon** | 1.2, 1.4, 2.1-2.3, 3.1-3.2 | State management & drift detection |
| **P3 — Important** | 4.1-4.6, 5.4, 5.5, 6.1-6.3 | CLI parity & reliability |
| **P4 — Growth** | 5.1-5.3, 7.1-7.3, 9.1-9.4 | Ecosystem & testing |
| **P5 — Polish** | 4.7-4.10, 6.4-6.5, 8.1-8.3, 10.1-10.5 | DX & compliance |

# Local extensions and persistent memory

Standard builds (`make build`, CI and development helpers) enable
`feature_lsp,feature_wasm_ext` with CGO disabled. Without `feature_wasm_ext`,
`doctor` reports `disabled-by-build`; a runtime load failure is reported separately.
All processes and loaded modules belong to the CLI scope. Nothing survives exit.

## Configuration and activation

`mcp add` and `extensions add` save **disabled** configurations. Configuration
lives in a private per-workspace TOML file beneath the user data directory,
reported by `extensions list` and `doctor`. `extensions.file` can select another
absolute user-private file outside the workspace; project TOML cannot set it.
Parent symlinks are resolved when checking the workspace boundary. Stores use
atomic replacement and user-only permissions/ACLs.

```sh
opera mcp add local stdio /absolute/path/to/server -- arg1 arg2
opera mcp add service streamable_http https://example.com/mcp mcp-token-reference
opera mcp enable local
opera resume <run-id> --approve <approval-id>
opera mcp disable local
opera mcp refresh service
opera extensions add example.echo /absolute/path/to/module logs y_tools
opera extensions load example.echo
opera extensions unload example.echo
```

The initial `ask` policy persists approval before executing external code.
`trusted` is an explicit user policy choice; `read-only` rejects activation and
external mutations. Approval binds server configuration or module bytes,
manifest and grants. Changes require new consent. Disabling a broken WASM module
does not require its file to remain available. Adding a configuration starts no
server, module or provider. Never put literal credentials in command arguments.

HTTP transport accepts HTTPS, or HTTP on loopback for local servers. Userinfo,
query strings and fragments are rejected. `credential_ref` resolves from the
selected secret store; `env_refs` maps stdio environment names to secret references.
No ambient API-key environment variables are inherited. Only basic path/home,
locale, platform and temporary-directory variables are inherited by stdio children.
Use `auth set <reference>` to store values from stdin. HTTP bearer references are
resolved per request, allowing rotation without copying secrets into settings.

Management actions use a deterministic internal provider through Service.Execute,
the public AgentRunner, approvals, checkpoints and OperationStore. No remote
model or model credentials are needed. New activations take effect in the next
CLI invocation or a supported idle reload. A pending approval exits 3; an uncertain
effect exits 4 and requires explicit reconciliation before retry.

## Native provider and tool-pack replacement

Providers are registered in an instance-scoped `providercatalog.Catalog` with an
ID, selection metadata and factory. The same catalog supplies execution and the
chat provider/model selectors. Generic configuration accepts provider IDs and
composition reports an unregistered ID with context. The default native
implementations live in `internal/adapters/providers/<provider>`; the core
aggregates the selected entries and applications may pass a
different `ProviderCatalog` to `core.Open` or `ui/cli.App` without adding a provider switch.
`TestCustomProviderCatalogExecutesOwnImplementation` in
`internal/core/integration_test.go` demonstrates an offline scripted provider
executing through the real application service.
`TestHeadlessRunUsesInjectedProviderCatalog` checks the same boundary through CLI.

Native tool packs are selected by ID in core composition. Pass
`DisabledToolPacks` to
remove a default pack, or `ToolPacks` to contribute a separately owned pack. The
executable example `Example_customToolPack` in
`internal/core/toolpacks_example_test.go` removes the default packs and
registers a custom tool using the same public Y registry consumed by the runner.
Pack registration occurs during the tools component's `Start`; the component
owns the registry and closes every registration together. Extensions continue to
use the Y `extensions.Host` and its component scopes for dynamic lifecycle.

```go
opts := harness.Options{
	ToolPacks: []harness.ToolPack{{
		ID: "review-tools",
		Register: registerReviewTools,
	}},
	DisabledToolPacks: []string{"shell"},
}
```

Pack IDs must be unique. Unknown disabled IDs and duplicate registrations fail
before a runtime starts. Factories only describe registration; they must not
open resources or start workers.

## Tools, resources and prompts

```sh
opera tools list
opera tools call read_file '{"path":"README.md"}'
opera mcp tools local
opera mcp resources service
opera mcp templates service
opera mcp prompts service
opera mcp call mcp.service.resources.read '{"uri":"test://data"}'
opera mcp call mcp.service.prompts.get '{"name":"review"}'
opera mcp call mcp.local.echo '{}'
```

MCP descriptor caches are inspection aids, not executing handlers. `mcp tools`
and `tools list` can inspect cached external descriptors without connecting.
Resources, prompts and tool calls use live scoped clients and the ordinary durable
tool path. Unknown/external tools are conservatively mutating: one workspace queue
serializes them with other agents' writes, and policy applies even to descriptors
with no capabilities or sensitivity annotations. Results have an output budget;
truncation preserves the recorded success/failure of an already completed effect.
Approvals show remote paths as arguments and do not read them as local files.

## Memory

```sh
opera memory enable
opera memory status
opera memory save '{"type":"project_fact","scope":"workspace","summary":"Checks","content":"Run make check"}'
opera memory search checks
opera memory read <id>
opera memory list
opera memory forget <id>
opera memory disable
```

Memory uses the public filesystem adapter, isolated by canonical workspace beneath
user data. It survives runtime shutdown; disabling does not delete stored items.
Save and forget require explicit user intent and workspace policy. Consent is
reconstructed from the persisted user transcript after approval handoff. Automatic
model extraction is disabled. Historical recall is a context source, never a new
system instruction, and is bounded by item/byte/token limits.

The private settings file supports `[memory] enabled = true` and `fail_open = true`.
Fail-open affects recall reads only. Startup readiness and writes remain required;
failed writes expose degraded persistence in live inspection. `memory status`
reports configuration offline; `/memory` or `extensions inspect` shows live state.

## WASM and safe points

A module directory contains the public Y `extension.toml` manifest and its ABI
module. `extensions schema <id>` displays its manifest/schema. The product grants
only `logs` and `y_tools`. The latter supports classified, non-sensitive read tools
through the **captured durable runtime view**, with a derived operation identity.
Mutating, task/subagent, agent skill, recursive WASM and sensitive host calls are refused.
Filesystem, network, process and secret host grants are unsupported and rejected.
Guest tools themselves remain visible operations, subject to policy and the
workspace mutation queue. Manifest/runtime memory, execution and output limits
apply; this host does not claim unsupported fuel-metering guarantees.

`extensions inspect` and `/extensions` expose component states, generations,
bindings, tools, context sources, hooks and persistence through public contracts.
`list`/`discover` are offline and never load a guest. Inspection starts only
previously authorized enabled components; failures remain errors, never a silent
fallback to a different module. Offline inspection includes discovery errors so
broken components can still be disabled.

`/extensions reload` requires idle execution and no unresolved approvals/children.
A single existing WASM activation toggle is supported; captured views retain their
handler scopes until released. Invalid candidates preserve the current generation.
MCP/memory replacement, new directories, changed warm module bytes/grants and
multiple WASM toggles require a new CLI scope. `extensions reload` in a fresh CLI
checks the same persisted safe point before opening that new scope. No daemon,
background reload watcher or detached worker is introduced.

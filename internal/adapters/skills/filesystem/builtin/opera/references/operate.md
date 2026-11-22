# Operate and diagnose Opera

## Entry points

- `opera` in an interactive terminal opens chat. Use `opera --help` for options and the terminal's command help or completion for slash commands. `/model` and `/thinking` select advertised model and reasoning capabilities. `/review [task]` selects the read-only reviewer. Plan mode is for inspecting and proposing work.
- `opera run 'task'` runs headless. `opera worker 'task'` adds a durable root task. Local subagents share the workspace and consume the same worker budget; they do not create isolated checkouts. Use the developer guide for prompt and runtime ownership.
- `--format jsonl` emits machine-readable events. `--response-format` and `--response-schema` control the generated model content. Consumers should parse event envelopes, respect schema versions, and inspect the final execution result and exit status.
- `opera skills list`, `opera skills show opera` and `opera skills check` work without a provider. In chat use `/skills`, `/skills opera` or `$opera`. The agent can use `skill_catalog`, `skill_load` and `skill_resource` when advertised.

## Configuration and diagnosis

- Start with `opera --version` and `opera doctor --format jsonl`. Doctor reports effective composition and tool support without starting providers, extensions or tool processes. Missing optional support, an invalid selection and an activation error have different remedies.
- Inspect `docs/configuration.md` for precedence and paths. Project settings have a narrower trust boundary than user configuration. Keep provider credentials in the configured credential store or supported ambient sources. Keep credential values out of arguments, logs and project files.
- Use `opera models --provider <id>` for model metadata. Some providers contact their endpoint. Advertised capabilities do not establish account entitlement. Use the provider's supported `opera auth` commands for credentials.
- Use `opera tools list` to inspect built-in schemas and cached MCP descriptors. `opera tools call <name> '<JSON>'` executes through the durable policy path. Cached descriptors do not establish that a remote server is reachable.

## Recover work explicitly

- Distinguish a completed run from one waiting on approval or a child, and from an uncertain operation. Use `opera sessions` and its subcommands to identify the run and inspect its records. `opera usage run <id>` reports usage for a run and its descendants.
- Resume the same workspace and original provider, model and runtime configuration with `opera resume <run-id>`. Resolve the exact pending approval with `--approve <approval-id>` or `--deny <approval-id>`. Approval is bound to one operation and its observed inputs. A changed file can require a fresh approval.
- Exit status 3 means input or child work is pending. Exit status 4 means an effect is in doubt. Inspect the operation and reconcile its recorded outcome before attempting recovery. An unexplained interruption is not evidence that an external effect failed.
- To send a new message after a completed run, use `opera run --continue <run-id> 'task'`. Continuation creates a new run in the conversation.

For exact syntax and limits, read the source checkout's `docs/cli-contract.md`, `docs/configuration.md`, `docs/operations.md`, `docs/tools.md`, `docs/skills.md` and `docs/usage.md`. Release archives also carry operational documentation.

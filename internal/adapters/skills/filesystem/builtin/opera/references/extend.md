# Extend Opera

Use `docs/extensions.md`, `docs/composition.md`, `docs/configuration.md`, `docs/skills.md` and `docs/ui-extensions.md` in the source tree for exact contracts. Select the extension surface based on the required capability and trust boundary.

## Compiled providers, tools and components

- Give a provider its own adapter under `internal/adapters/providers/<provider>`. The catalog entry declares its selection metadata and factory, and may provide login, model discovery or native search. Register it in the core's composed provider catalog so selection, auth and execution resolve the same entry. Keep provider-specific credential and transport protocols in the adapter.
- Add a compiled tool through an explicit tool pack when it belongs in the existing Y registry. Use a component when a capability has its own lifecycle, service or replacement boundary. Check the executable tool-pack and storage composition examples in `internal/core`.
- Compiled code has the privileges of the Opera process. Effects remain subject to workspace policy, approval, durable identity and auditing. For untrusted code, use mediated WASM or a transport with an appropriate external trust boundary.

## MCP, WASM and UI

- `opera mcp add` saves a disabled server. Specify stdio or streamable HTTP, plus credential references where needed. `opera mcp enable` activates it under policy. Stdio children run with local process privileges and a bounded inherited environment. Tools, resources, templates and prompts are exposed through namespaced descriptors. Inspect cached descriptors offline before choosing to connect.
- `opera extensions add` records a disabled WASM bundle. The module directory contains the public Y `extension.toml` and its ABI module. Load under explicit consent for the manifest, bytes and grants. Opera currently supports `logs` and classified read operations via `y_tools` in the captured view. Ambient filesystem, process, network and secret host grants are outside that contract.
- Inspect with `extensions list`, `extensions discover` or `extensions inspect`, checking whether the command is offline or starts authorized components. Idle reload supports a single compatible WASM activation toggle. MCP or memory replacement, new directories and changed grants or warm module bytes require a new CLI scope. Read the accepted safe-point contract before describing reload behavior. External bundle distribution is covered as a proposal in ADR 011.
- UI extensions supply bounded, sanitized presentation data through the documented contribution types. TUI components render from state and route typed actions through their owners. Use `docs/ui-extensions.md` for contribution limits and lifecycle.

## Skills and memory

- Put project skills under `.agents/skills/<name>/SKILL.md` and user skills under `~/.agents/skills/<name>/SKILL.md`. Use YAML `name` and `description`, and keep optional references within the skill directory. Workspace entries shadow same-name user entries. The reserved `opera` skill is embedded and wins over disk copies.
- The model discovers metadata and loads only the relevant instructions or resources. Scripts are read as resources and require an ordinary authorized execution path to run. Instructions and `allowed-tools` metadata do not grant authority. Resume checks the pinned catalog generation.
- Memory is optional, durable and isolated by workspace. Use the documented memory tools and commands. Save and forget require explicit intent. Memory recall is context, with limits, and disabling memory preserves its stored contents.

# Models and tools

`opera models --provider <id>` exposes public SDK model metadata/capabilities
without reading credentials. Lists may be curated SDK fallbacks, not an account's
remote entitlement. `/model` offers keyboard selection at idle; `/model <provider>
<model>` is the direct/accessible form. `/thinking` accepts only advertised
reasoning support. A capability flag cannot guarantee that an endpoint or account
accepts a particular request. Unknown OpenAI-compatible contracts are not inferred.

`tools list` exposes the active built-in schemas and cached enabled MCP descriptors
without invoking them. `/tools` inspects the live registry. `tools call <name>
'<JSON>'` is a durable operation with the same policy as model-generated calls.

| Family | Examples | Limits |
| --- | --- | --- |
| Filesystem | read_file, write_file, edit, patch, list_directory, search | Canonical workspace checks, bounded results, operation-bound file previews |
| Process | run_command | Tool timeout, cancellation/tree cleanup, conservative mutation policy |
| Git | git_status, git_diff, git_log, git_show, git_blame | Requires Git; workspace-shared effects still need policy |
| Search | search_text, search_ast, outline, find_files | rg fallback; ast-grep optional and never auto-installed |
| Semantic navigation | definition, references, hover, symbols, callers/callees | Scoped lazy LSP; external server/capabilities required |
| Tasks/children | task_create/refine/start/complete/list, subagent_spawn/list/wait/cancel | Persisted IDs, process-owned children and shared reservation/write limits |
| MCP | namespaced tools/resources/templates/prompts | Explicit activation/credential references, scoped clients, conservative policy |
| Memory | memory.search/read/list/save/forget | Workspace-isolated durable store; explicit write/forget intent |
| WASM | manifest-provided tools | Explicit module/grant consent, ABI/limits, classified-read host bridge only |

A read tool's wrapper may need to start an external process; process startup has
its own policy boundary where supported by the SDK. Unknown tools and shell/MCP/
WASM effects are serialized. The queue does not establish logical edit isolation
or sandbox arbitrary executables. `doctor` reports missing programs and supported
fallbacks. Inspection never installs programs. See [operations](operations.md) for
large-output archives, filtering and explicit recovery.

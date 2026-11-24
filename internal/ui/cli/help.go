package cli

const help = `Opera — local coding harness

Usage: opera [flags] [command]

Execution:
  opera / chat                        Interactive conversation in this workspace
  worker <task>                       Foreground planning with local subagents
  run <prompt>                        Foreground execution (text or JSONL)
  retry <run-id>                       Explicit retry with uncertain-effect protection
  compact <run-id>                     Compact a completed checkpoint
  sessions list [offset] [limit]       List latest runs per session
  sessions events|operations|decisions|output <run-id>
                                      Filter and export durable diagnostics
  sessions inspect|export <run-id>      Inspect/export redacted durable records
  sessions branch <run-id> [version]   Branch a completed current/archived checkpoint
  resume <run-id>                      Explicit checkpoint/approval recovery
  onboarding                          Choose provider, model and workspace policy
  update [check|apply]                Check for or install the latest Opera release
  deps [check|update]                  Check or update Go module dependencies
  reconcile <run-id> <key> <result>    Record an explicitly observed uncertain effect

Configuration and inspection:
  usage [workspace|run|session|agent|turn] [id] [turn-id]
                                      Inspect durable tokens and cost provenance
  models                              Inspect SDK model metadata and capabilities
  demo [chat|run|worker|headless]               Verify offline in a disposable workspace
  context [paths...]                  Inspect instruction/attachment sources
  settings                            Inspect resolved configuration
  config [show|paths|schema|get <key>] Inspect configuration and provenance
  tools [list|call <name> <JSON>]      Inspect schemas or execute a durable tool operation
  skills [list|show <name>|check]     Discover and validate Agent Skills
  mcp add|enable|disable|refresh       Configure and approve MCP stdio/HTTP connections
  mcp list|tools|resources|prompts     Inspect cached tools or server capabilities
  mcp call <tool> <JSON>              Invoke a namespaced MCP capability
  extensions add|load|unload          Configure and approve local WASM modules
  extensions inspect|schema|capabilities|reload
                                      Inspect components, grants and generations
  memory enable|disable|status        Configure persistent workspace memory
  memory save|read|search|recall|list|forget
                                      Manage memory through durable tools
  doctor                              Local diagnostics without contacting providers
  auth set <reference>                Store a secret from piped stdin
  auth status <reference>             Check presence without displaying a secret
  auth delete <reference>             Delete a stored secret
  auth login <provider>               OAuth device/PKCE login with URL notifications

Flags (accepted before or after a command):
  --help, -h                 Show help without reading configuration
  --version                  Show version without reading configuration
  --workspace <path>         Workspace directory (default: current directory)
  --user-config <path>       Explicit core settings file (JSON or TOML; must exist)
  --project-config <path>    Explicit project TOML file (must exist)
  --continue <run-id>        Continue a completed conversation in a new run
  --attach <path>            Add a bounded UTF-8 text or supported image attachment
  --thinking <level>         Select reasoning supported by the model
  --provider <id>            Provider selection
  --model <id>               Model selection
  --base-url <url>           Explicit user-controlled provider endpoint
  --policy <policy>          read-only, ask (recommended), or trusted
  --approve <approval-id>    Resume the exact reviewed operation
  --deny <approval-id>       Resume with an explicit denial
  --format <format>          Event transport: text (default) or jsonl
  --response-format <kind>   Model content: text or json_object
  --response-schema <path>   Bounded portable JSON Schema for model content
  --event-type <type>        Filter sessions events/decisions
  --tool <name>              Filter inspected operations/events
  --operation-id <id>        Filter operation/tool-call ID; select archived output
  --after <cursor>           Event sequence or operation offset
  --limit <count>            Inspection page size (1..1000)
  --store <store>            keyring (default) or file (unencrypted, user-only)
  --no-motion[=false]        Disable animation
  --accessible[=false]       Append-only chat without continuous redraw
  --set <key=value>          Override any key listed by config schema

Chat: Enter sends; Alt+Enter or Ctrl+J adds a line. Ctrl+C cancels active
execution; at idle it exits. Empty Ctrl+D exits. /approve and /deny resolve
pending operations. /steer <message> explicitly steers; other active input is
queued as a follow-up. No alternate screen or detached execution is used.

Precedence: flags > OPERA_* > project > user (global/workspace) > defaults.
Secrets belong in auth, never TOML, flags or version-controlled files.
File storage is explicit; an unavailable keyring never triggers fallback.
Headless approval persists and exits with code 3. Uncertain effects require
explicit reconciliation (code 4), never automatic retry. Workers and children stay attached to this CLI process.
`

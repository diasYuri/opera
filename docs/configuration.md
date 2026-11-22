# Configuration and local paths

`opera config` shows every effective value and its source. `config get <key>`
inspects one key, `config schema` lists the schema and environment keys, and
`config paths` reports locations. These commands never read the keyring.
`--help`, `--version` and `config schema` also work with invalid configuration or
an unavailable workspace. Headless commands never open an interactive form; `onboarding` is TTY-only.

## Locations

Core settings are stored in `~/.agents/opera/settings.json`; custom providers are
stored in `~/.agents/opera/providers.json`. `opera config paths` reports both
paths. `OPERA_CONFIG_DIR` overrides the directory. Both files must be regular
files in an application directory private to the current user (0700 directory,
0600 file on Unix; current-user-only ACL on Windows). `providers.json` may hold
API keys.

| Platform | Data | Cache |
| --- | --- | --- |
| Linux | `$XDG_DATA_HOME/opera`, otherwise `~/.local/share/opera` | `$XDG_CACHE_HOME/opera`, otherwise `~/.cache/opera` |
| macOS | `~/Library/Application Support/opera` | `~/Library/Caches/opera` |
| Windows | `%LOCALAPPDATA%/opera` | `%LOCALAPPDATA%/Cache/opera` |

If Windows application-data variables are unset, use `~/AppData/Roaming` and
`~/AppData/Local`. `OPERA_CONFIG_DIR`, `OPERA_DATA_DIR` and `OPERA_CACHE_DIR`
override their respective application directories and must be absolute.
Discovery is read-only: paths need not exist until explicitly storing data.

The workspace is the current directory by default. `--workspace` overrides
`OPERA_WORKSPACE`; canonicalization resolves symlinks and requires an existing
directory. Project configuration remains `<canonical-workspace>/.opera/config.toml`.
There is no parent-directory config search. `--user-config` and
`--project-config` select explicit files and require them to exist. Missing
default files are optional. Settings, providers and project files are limited to 1 MiB.
Workspace-specific interactive preferences are stored under
`~/.agents/opera/workspaces/`.

## Precedence, types and trust

Apply defaults, global user `settings.json`, private per-workspace settings,
project TOML, environment, then flags. `settings.json` uses JSON with the same
core keys shown by `config schema`, nested under objects such as `agents`,
`limits` and `ui`. Onboarding writes workspace-specific choices under
`~/.agents/opera/workspaces/<workspace-hash>.json` with private permissions; its
provenance is `user-workspace`. It never writes trust into version-controlled
project configuration. Empty strings
and `false` are explicit values. Each key retains `default`, `user`, `project`,
`environment` or `flag` provenance. Invalid values in any layer are errors even
when a later layer would override them. Unknown keys/tables and incorrect JSON
or TOML types are errors. Parsing errors omit input values so accidentally supplied
secrets do not appear in diagnostics.

Policy order is `read-only < ask < trusted`. Project policy cannot exceed the
user policy (default `ask`); attempted elevation is an error, not a silent grant.
An invalid project remains invalid even if a flag subsequently selects trusted.
Choose elevated policy explicitly in user configuration, environment or a flag,
and remove conflicting project elevation. Project `credentials.store` and `base_url` are always
rejected; its selection is user-controlled. This is policy configuration, not
an operating-system sandbox.

Environment names are `OPERA_` plus the uppercase dotted key with dots replaced
by underscores. For example, `agents.max_depth` becomes `OPERA_AGENTS_MAX_DEPTH`.
Only documented environment keys are read. Core settings have no API secret
fields; custom provider credentials are defined separately in the private provider file.

When `search.provider` is empty (the default), Opera enables the provider's
hosted web search for `openai-subscription` (Codex sign-in) and `anthropic`
(Claude sign-in). It uses the active provider's saved login and resolves the
credential only after `web_search` is approved. Other model providers do not
enable external search automatically. Codex sign-in uses the public OpenAI
Responses API `web_search` tool; it does not call ChatGPT's internal `backend-api`
routes. Search availability still depends on the selected model and account
policy. If a project selects the model provider,
set `search.provider = "native"` in user settings to opt in. Set
`search.provider = "disabled"` to turn search off, or choose `"brave"` to use
Brave Search with a separate key.
For Brave, store the key with `opera auth set brave-search.default` or set
`BRAVE_SEARCH_API_KEY`. Project configuration cannot enable external search.
An approved search sends its query to the selected provider's search service;
returned titles and URLs provide citation evidence.

In the interactive chat, configure these choices at `/settings` → `Web search`:
select the search backend, then open `Options` → a provider → `Config options`.
OpenAI, Anthropic and Brave share defaults for maximum results, country and
allowed/excluded domains. OpenAI also supports context size; Brave supports
language and safe-search level. `max_results = 0` and empty values keep the
provider default. A setting fills a search request only when that request does
not specify the control, so an explicit per-search value takes precedence.
These options are stored in private workspace settings; project configuration
cannot set search selection or filters.

Each provider menu also has `Authentication`. Codex uses ChatGPT sign-in; Claude
offers Claude account sign-in or an Anthropic API key; Brave accepts a Brave
Search API key. API keys are masked while entered and stored in the configured
credential store, never in workspace or project settings. Accessible mode does
not accept echoed secrets; use `opera auth set <provider>.<credential_ref>` from
piped stdin instead.

```toml
[search.options.openai-subscription]
context_size = "low"
max_results = 8
country = "BR"
allowed_domains = "openai.com, docs.openai.com"

[search.options.anthropic]
country = "BR"
excluded_domains = "example.invalid"

[search.options.brave]
language = "pt"
safe_search = "moderate"
```

```sh
opera config get policy --policy read-only
opera config get ui.no_motion --no-motion=false
opera config --set agents.max_concurrent=2
opera config --provider openai --model your-model-id --format jsonl
```

`NO_COLOR` is honored by its presence, including an empty value.
`--accessible --no-motion` selects append-only screen-reader input; see [keyboard behavior](accessibility.md).
The interactive `/settings` menu lets you choose the terminal palette. Built-in
choices are `system`, `midnight`, `ocean`, `forest`, `sunset`,
`mono`, `dark` and `light`; the five curated palettes are Midnight, Ocean,
Forest, Sunset and Mono. The choice is stored in private per-workspace user
settings and does not change the terminal background. `NO_COLOR` still disables
all color output.

## Resource budgets

| Key | Default | Accepted range / meaning |
| --- | --- | --- |
| `agents.max_concurrent` | 4 | 1–32 children at once; bounded shared capacity |
| `agents.max_depth` | 1 | 0–8; zero disables delegation |
| `limits.context_tokens` | 64000 | 1024–2000000; application budget, capped by model context in F1/F2 |
| `limits.output_bytes` | 1048576 | 1024–67108864; per-tool stdout/stderr/result budget |
| `limits.attachment_bytes` | 10485760 | 1024–104857600; per-attachment size, also subject to provider limits |
| `limits.process_timeout` | `2m` | Positive Go duration, at most 24 hours |
| `limits.shutdown_timeout` | `10s` | Positive Go duration, at most 24 hours; separate cleanup context |

Limits are validated when loading configuration. F1 enforces process/tool timeout, filesystem output bounds, bounded run turns and an estimated context budget before provider calls. Responses default to at most 4096 output tokens. Attachment and child-agent limits become active with their later-phase capabilities. A requested context budget does not assert that a model
supports that many tokens. Output truncation must be visible; tool metadata and
operation identity must survive truncation. Diagnostic tools never install
dependencies: missing `rg` falls back to filesystem text search in F2, missing
`ast-grep` disables AST search, missing language servers disable semantic
navigation, and missing Git disables Git operations. No fallback bypasses policy.

The core JSON can select a provider defined in `providers.json`:

```json
{
  "provider": "custom:acme",
  "model": "acme-reasoner",
  "policy": "ask",
  "ui": { "theme": "system" }
}
```

The [example](../examples/config.toml) documents project TOML keys. Custom
provider definitions are documented in [custom providers](custom-providers.md).
`credential_ref` continues to name credentials stored in the separate store.

F2 adds `thinking` (`off` by default; `OPERA_THINKING`/`--thinking`) and enforces
`limits.attachment_bytes` within the context budget. Interactive selections are
session-local; use user settings or flags for persistent defaults. `--attach` may
repeat; `context`/`/context` show the loaded instruction and attachment sources.

F3 enforces `agents.max_concurrent` (default 4), `agents.max_depth` (default 1)
and `limits.worker_tokens` (default 0, unlimited). Set a positive value to cap
the durable shared reservation ledger across a parent and descendants: each
provider request reserves estimated input plus its 4096-token output cap,
including compaction. It is a conservative work budget, not billing/accounting.
Child queues are bounded to 16 times concurrency and 64 children per parent.
Cancellation releases queued work and all held slots.

F4/F5 add the user-only `extensions.file` and `accounting.prices_file` paths.
Projects cannot choose them. The former stores external capability consent outside
the workspace; the latter selects an explicitly sourced/dated optional price table.
See [extensions](extensions.md) and [accounting](usage.md). Neither file may contain
literal API credentials. Response schema flags are execution contracts, independent
of `output.format`'s event transport.

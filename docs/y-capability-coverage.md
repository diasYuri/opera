# Y capability coverage — F5 baseline

Baseline: **github.com/diasYuri/y v1.4.0**, published commit
`0a0f4665c7b397eda960aa0e19c9f73ac81c1e73`. F0 audited v1.2.0; F1 implemented the authorized changes in `~/git/diasYuri/y` and published the SDK. [F1 evidence](f1-validation.md) records the tests and real-provider proof.

The module was resolved through Go module download with checksum
`h1:snEpSTQfYBkrU7XvW5DUBQyr/H7js+Yv2vtDvFNANgo=`. This is the actual published
baseline, not a local replacement. The old roadmap's API families are expanded
below. Integration coverage is recorded per phase; extension/provider parity remains a final coverage gate.

## Reproducible exported-declaration inventory

[The generated index](y-public-api.json) records every exported top-level type,
constant, variable and public receiver method/function in consumer-accessible
`pkg/` source, including file/line/build constraint. Public interface members and
struct fields belong to their indexed type; this is a declaration inventory,
not an assertion that every method is a distinct product feature. Platform and
enabled/disabled variants are retained, so counts are declarations, not unique
names. Nested `internal` packages and `_test.go` files are excluded. Generate with
`GOTOOLCHAIN=go1.27.4 go run ./scripts inventory`; CI checks reproducibility.

| Public package | Declarations | Product applicability / coverage family |
| --- | ---: | --- |
| `pkg/agent` | 299 | Runner, operations, state, events, controls and runtime views (C04–C09). |
| `pkg/agent/agenttest` | 19 | Test-only scripted providers; supports fake execution fixtures. |
| `pkg/agent/branch` | 20 | Branch tree/clone primitives (C10). |
| `pkg/agent/compaction` | 20 | Compaction policy, token/file-operation summaries (C11). |
| `pkg/ai` | 96 | Normalized messages, content, events, usage and structured output (C02/C12). |
| `pkg/coding` | 28 | Reusable coding/context helpers; Opera owns prompts and profiles (C13). |
| `pkg/component` | 87 | Scopes, factories, descriptors, generations, bindings and graph validation (C01/C21). |
| `pkg/config` | 19 | Capability/schema validation; do not inherit Y CLI defaults (C22). |
| `pkg/context` | 40 | Scoped sources/resolution/versioned snapshots (C13). |
| `pkg/extensions` | 25 | Scoped extension host and legacy adapters (C21). |
| `pkg/extensions/mcp` | 14 | Stdio/streamable HTTP and tools/resources/prompts (C17). |
| `pkg/extensions/memory` | 64 | Recall/extraction/queue/readiness/redaction and tools (C18). |
| `pkg/extensions/subagents` | 54 | Bounded local delegation, backend and store interfaces (C16). |
| `pkg/extensions/task-manager` | 34 | Hierarchical task tools and agent option (C15). |
| `pkg/extensions/wasm` | 160 | ABI, manifests, capabilities, policy, limits and components (C19). |
| `pkg/extensions/wasm/wasmtest` | 19 | Test-only ABI module builder; supports WASM fixtures. |
| `pkg/lsp` | 60 | Clients and managed language servers (C14). |
| `pkg/memory` | 36 | Store/extractor contracts, candidates and persistence readiness (C18). |
| `pkg/policy` | 37 | Decisions, approval, quotas and scope/identity (C03). |
| `pkg/providers` | 55 | Provider/model catalogs, retries, middleware, errors and token helpers (C02). |
| `pkg/search` | 17 | Provider-neutral typed web-search requests, capabilities and citation evidence, implemented by OpenAI, Anthropic and Brave adapters. |
| `pkg/providers/anthropic` | 19 | Provider adapter, models, stream/capabilities and HTTP fixtures (P01–P04). |
| `pkg/providers/auth` | 33 | Credentials, sources/resolver and refresh interface (C05). |
| `pkg/providers/google` | 19 | Provider adapter, models, stream/capabilities and HTTP fixtures (P01–P04). |
| `pkg/providers/openai` | 20 | Provider adapter, models, stream/capabilities and HTTP fixtures (P01–P04). |
| `pkg/providers/openai_compatible` | 19 | Provider adapter, models, stream/capabilities and HTTP fixtures (P01–P04). |
| `pkg/providers/providertest` | 19 | Test-only fake provider and capability helpers. |
| `pkg/rpc` | 37 | Remote server hosting is outside this product; normalized event/error contracts may inform local output (C23). |
| `pkg/runtime` | 171 | Harness/service keys apply; network/queue hosting is outside CLI scope (C01/C23). |
| `pkg/session` | 3 | Session and branch store interfaces (C10). |
| `pkg/telemetry` | 40 | Tracing, dimensions and accounting (C20). |
| `pkg/tools` | 121 | Registry, policy/middleware, snapshots and coding tool registrations (T01–T30), including the generic web-search registration helper. |
| `pkg/tools/code_search` | 2 | Public convenience registration adapter; same enforcement as its pkg/tools family. |
| `pkg/tools/filesystem` | 2 | Public convenience registration adapter; same enforcement as its pkg/tools family. |
| `pkg/tools/git` | 2 | Public convenience registration adapter; same enforcement as its pkg/tools family. |
| `pkg/tools/gitignore` | 8 | Reusable ignore/walk rules for context and search (C13/T03). |
| `pkg/tools/lsp` | 2 | Public convenience registration adapter; same enforcement as its pkg/tools family. |
| `pkg/tools/shell` | 2 | Public convenience registration adapter; same enforcement as its pkg/tools family. |

## F1 SDK additions verified

All six SDK gaps below were observed in v1.2.0 and are closed by the published
v1.3.4. Their original baseline descriptions are retained as an audit trail.

| Gap | Public contract now available | Evidence | Status |
| --- | --- | --- | --- |
| Y1 | `pkg/providers/oauth.Client` Login/Refresh, Config, Notice, injected HTTP/clock/wait | SDK HTTP fixtures and Opera CLI OAuth notification/storage fixture | verified |
| Y2 | `pkg/storage/sqlite.Store`, Sessions and metadata APIs | Transactional claim/recovery tests, session/branch tests, Opera close/reopen/resume | verified |
| Y3 | `pkg/memory/filesystem.NewFilesystemStore` | Existing memory tests moved with the single implementation; native locks; external consumer | verified |
| Y4 | `task-manager.Manager.Snapshot/Restore` | Identity/hierarchy/allocator round trip and failed restore atomicity | verified |
| Y5 | `AgentRunner.Subscribe/Steer/FollowUp/CancelRun/Close` | Active control/cancellation tests and Opera streaming/recovery/lifecycle tests | verified |
| Y6 | Captured-view durable registry decoration | RuntimeView integration observes Begin/Complete and blocks uncertain effects | verified |

New public packages in the generated inventory:

- `pkg/providers/oauth`: 5 exported declarations.
- `pkg/storage/sqlite`: 30 exported declarations.
- `pkg/memory/filesystem`: 10 exported declarations.

The minimum F1 read/write path, OpenAI live proof and HTTP-compatible fixture are verified. F2 integrates coding tools, context and scoped LSP; F4/F5 integrate extensions, usage and structured output; upstream symbols alone are not acceptance evidence.

## Coverage row conventions

The current consumer pin is Y **v1.4.0**. Rows describe Opera product coverage;
evidence retains the SDK version under which it was gathered. `pending` means a
product implementation and its named acceptance test remain future work; no
planned test is claimed to pass.
The per-row **owner** is the responsible Opera/SDK component, not a person.
The package links above and generated index resolve precise file/line symbols.

Enforcement profiles (referenced on every row) define delivery modes, policy,
persistence, queue and evidence requirements:

- **A**: applicable to chat/run/worker through the shared application service;
  read-only/ask/trusted and per-operation approval; SQLite snapshots/events/
  operations, canonical-workspace mutation queue, one agent-labelled approval
  queue; remote smoke opt-in only where the provider needs it. Feature-specific
  limits appear per row. F0 has not activated these paths.
- **R**: local read-only inspection (`config`, `doctor`, future inspection commands),
  no execution store/queue and no provider/credential read; applies independently
  of chat/run/worker. F0 evidence is the local CLI/config tests.
- **L**: lifecycle/configuration shared by chat/run/worker and inspection;
  policy applies before activating a privileged acquisition; store/queue inherited
  from the associated operations. Resource cleanup preserves durable data.
- **X**: not applicable to any mode because it hosts remote/detached execution;
  no store/queue/test required beyond architecture checks and scope review.

| ID | Public symbol / capability | Tag | Applicable / reason | Command or slash | Profile | Owner / phase | Automated evidence or required test | Provider limitation | Status |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| C01 | runtime.NewHarness, Resolve; component.Define, Scope, Runtime | none | yes: public composition | all modes | L | harness / F1 | F0 TestPublishedDependencies passes; F1 graph rollback/close integration | none | verified F1 |
| C02 | providers.Provider, Catalog, RetryPolicy, Middleware; ai.Message/Event/Usage | none | yes: normalized inference | models, /model, /thinking | A | harness/providers / F1–F2 | contract fixtures per adapter, retry/cancel/error propagation | capabilities differ by model | Four SDK factories have F5 contract coverage; OpenRouter/Groq/DeepSeek aliases added over the compatible adapter, contract fixtures pending |
| C03 | policy.Engine, ApprovalRequest/Resolution; tools.Registry.Handle | none | yes: operation authorization | approvals, config policy | A | approval/workspace / F1 | allow/deny/headless and changed-diff tests pass; project escalation blocked | none | verified F1 |
| C04 | agent.NewAgentRunner, RunRequest, RunResponse, Run | none | yes: same backend | chat/run/worker/resume | A | app/harness / F1 | mode parity + streaming/tool/checkpoint integration | public runner controls available; worker integration verified | verified F3 chat/run/worker/resume |
| C05 | providers/auth.CredentialStore, Resolver, TokenRefresher | none | yes: secret resolution | auth, onboarding | L | credentials + Y OAuth / F1 | F0 file lifecycle/CLI no-leak; F1 resolver/OAuth HTTP fixtures | OAuth public in v1.3.4; broader model/profile wiring remains | verified: public OAuth/resolver/store; actual model entitlement remains provider-specific |
| C06 | agent.StateStore, EventStore, TranscriptStore, LeaseStore | none | yes: consistent state | resume, sessions inspect/export | A | persistence + Y adapters / F1–F3 | SQLite contract + migrations + optimistic conflict/crash tests | Public SQLite adapter available; session catalog and recovery verified | verified F3 recovery/catalog |
| C07 | agent.OperationStore, OperationReconciler, JSONLStore | none | yes: replay safety | retry/reconcile, tools inspect | A | persistence/approval / F1 | effect-before-commit crash; in-doubt retry blocked; reconciliation | arbitrary external effects lack exactly-once | verified F3 uncertain effects/reconcile |
| C08 | agent.EventEnvelope, RunControl, runtime protocol types | none | yes: inspection/control contracts | /steer, follow-up, cancel | A | app + Y runner / F1/F3 | public active control event sequence/backpressure and terminal races | public active controls verified in SDK; F3 UI and control journal verified | verified F3 controls + journal |
| C09 | agent.RuntimeViewSource, RuntimeView; RegistrySnapshot | none | yes: pinned handler generation | tools inspect, execution | A | harness + Y runner / F1/F4 | BeginOperation/CompleteOperation observed on captured registry; Y6 | captured registry enforcement verified by SDK integration | F4 integrated; durable captured view and handoff verified |
| C10 | session.Store, BranchStore; agent/branch.BranchManager | none | yes: session identity and forks | sessions list/inspect/export/branch | A | persistence/app / F3 | branch provenance/new operations/no inherited grants | branches share workspace files | verified F3 branches/provenance |
| C11 | agent/compaction.Compactor, agent compaction/retry options | none | yes: bounded long conversations | /compact, /retry | A | app/context / F3 | retain instructions/tasks/operation refs and prevent replay | summarization model budget must be checked | verified F3 manual compaction/retry |
| C12 | ai.ResponseFormat, ValidateResponseFormat; structured output | none | yes: schema-constrained results | run/worker structured output | A | app/output / F5 | supported/unsupported/invalid schema fixtures | validate actual provider/model support | F5 verified: valid schema/content, invalid JSON/type and unsupported-capability refusal |
| C13 | context.Resolver, Source; coding helpers, tools/gitignore | none | yes: scoped file/instruction context | /context, attachments | A | context / F2 | AGENTS scope/provenance/ignore/budget/modalities fixtures | attachments require model modality support | verified F2/F3: scoped context/modalities, budget and compaction |
| C14 | lsp.Client, Manager; tools.RegisterLSP | feature_lsp | yes: semantic code navigation | coding tools, doctor | A | harness/LSP / F2 | Unicode/UTF16 fixture server, missing-server fallback, shutdown | external language server required | F2 integrated; external server required |
| C15 | task-manager.Manager, Task, RegisterScoped, AgentOption | none | yes: planning hierarchy | worker, /tasks | A | app/tasks + Y task manager / F1/F3 | Y4 snapshot round-trip IDs/hierarchy and invalid references | Public snapshot/restore verified in F1 | verified F3 task snapshot/restore |
| C16 | subagents.Manager, Spawn/Get/List/Wait, Store/MemoryStore | none | yes: synchronous local children | worker, /agents | A | app/subagents / F3 | 4-child/depth-one bounds, cancellation, shared writes, persisted hierarchy | process-owned scheduler and SQLite product records | verified F3 application scheduler; public child records |
| C17a | mcp.New, TransportStdio, RegisterTools/InstallScoped | none | yes: local MCP transport/tools | mcp, tools | A | harness/MCP / F4 | fixture stdio tool, malformed server, timeout and process cleanup | server availability and capability claims vary | F4 verified: stdio, consent, cleanup and partial startup failure |
| C17b | mcp.TransportStreamableHTTP, ServerConfig | none | yes: external tool service access, not remote Opera worker | mcp | A | harness/MCP / F4 | HTTP/auth headers/redirect boundaries/cancel fixture | network server required | F4 verified: HTTP/auth, deadline and failed activation |
| C17c | MCP resources.list/templates.list/read, prompts.list/get registrations | none | yes: context and prompt discovery | mcp resources/prompts | A | context/MCP / F4 | list/read/get fixtures with safe context handling | server may omit resources/prompts | F4 verified: resources.read/prompts.get and discovery schemas |
| C18a | memory.Store, ReadinessChecker; extension WithStore/InstallScoped | none | yes: durable recall | memory inspect/recall | A | persistence/memory + Y adapter / F1/F4 | Y3 filesystem round-trip/readiness/corrupt-write tests | Public filesystem adapter verified in F1 | F4 verified: private filesystem lifecycle |
| C18b | memory.Extractor, extension queue/worker/fail_open | none | yes: extraction and persistence errors | memory, execution context | A | context/memory / F4 | fail-open reads vs required writes, bounded queue/worker close | extraction may require a model | F4 recall/degraded/readiness/write verified; auto-extraction disabled by product |
| C18c | memory.Store.Forget and memory tool registrations | none | yes: explicit data management | memory forget | A | approval/memory / F4 | authorized deletion and preserved data during cleanup | none | F4 verified: save/read/authorized forget across CLI invocations |
| C19a | wasm.Manager, ComponentDefinitions, manifest/ABI | feature_wasm_ext | yes: local components | extensions list/load/unload | L | harness/WASM / F4 | small module load/unload/trap and no-cgo host fixture | ABI/host version validated | F4 verified: CGO-free load/call/unload/trap and invalid candidate |
| C19b | wasm.Capability, Policy, HostInvoker, Limits | feature_wasm_ext | yes: capability-scoped host effects | extensions capabilities, tools | A | approval/WASM / F4 | host call follows policy/store/queue; reject missing grant | no capability granted by project implicitly | F4 verified: explicit grants, durable classified-read bridge; raw host effects unsupported |
| C20 | telemetry.Accounting, Tracer, Emitter, Dimensions | none | yes: usage and inspection | /usage, events inspect | A | output/telemetry / F5 | dedup children/resume/branch; token/cost reconciliation | pricing may be estimated/unavailable | F5 verified: durable invocation ledger, SDK telemetry aggregation, policy/event inspection and provenance |
| C21 | extensions.Host, RuntimeHooks; component.Manager.Reconcile/AcquireSnapshot | none | yes: scoped generations | extensions inspect/reconfigure | L | harness / F4 | safe points, old-generation drain and failed-reconcile rollback | only public supported reconfiguration transitions | F4 verified: idle gate, pinned scope drain and rollback |
| C22 | config.CapabilityRegistry, schemas | none | yes: inspect compiled support without Y CLI defaults | config schema, doctor | R | config/diagnostics / F0/F4 | F0 config/doctor tests pass; F4 map public capability registry | F0 uses Opera-owned schema | F4 Opera schema and compiled/runtime diagnostics; SDK defaults not imported |
| C23a | rpc.Server and runtime HTTP/WebSocket/gRPC/queue hosting | feature_rpc for RPC; runtime transports none | no: remote hosting and detached scheduling are excluded | none | X | harness / F0/F5 | architecture checks; repeat scope review at F5 | not a reason to omit reusable event/error types | not-applicable |
| C23b | agenttest, providertest, wasmtest fixture helpers | none | yes: test support, not user-facing tools | tests | L | test infrastructure / F1–F4 | use in deterministic provider/WASM acceptance fixtures | no real credentials in default tests | F4 fixtures use public agenttest/providertest/wasmtest |
| F01 | Opera config loader and schema (uses TOML v2.4.3) | none | yes: effective configuration | config, doctor | R | config / F0 | TestPrecedenceAndProvenance, TestProjectCannotElevateOrSelectStore | no network access | verified |
| F02 | Opera credentials.Store, FileStore, native keyring boundary | OS-specific ACL implementation | yes: separate secret storage | auth set/status/delete | R | credentials / F0 | file lifecycle/no-leak/unsafe permissions tests; Windows tests configured for native CI | native keyring round-trip deliberately opt-in | implemented |
| P01 | providers/openai.New, Models, Stream, Capabilities | none | yes: supported SDK provider | --provider openai, /model | A | harness/providers / F1–F2 | openai HTTP stream/tool/error/cancel fixtures; real smoke opt-in | Responses API and model capabilities | verified: native SDK Responses live write/approval/resume/read; fake/default regressions |
| P02 | providers/anthropic.New, Models, Stream, Capabilities | none | yes: supported SDK provider | --provider anthropic, /model | A | harness/providers / F1–F2 | anthropic HTTP stream/tool/error/cancel fixtures; real smoke opt-in | Messages API and thinking/modalities | verified: native Messages HTTP fixture, auth injection, tool execution and accounting through Opera |
| P03 | providers/google.New, Models, Stream, Capabilities | none | yes: supported SDK provider | --provider google, /model | A | harness/providers / F1–F2 | google HTTP stream/tool/error/cancel fixtures; real smoke opt-in | GenerateContent and thought signatures | verified: native GenerateContent HTTP fixture, auth injection, tool execution and accounting through Opera |
| P04 | providers/openai_compatible.New, Models, Stream, Capabilities | none | yes: supported SDK provider | --provider openai-compatible, /model | A | harness/providers / F1–F2 | openai-compatible HTTP stream/tool/error/cancel fixtures; real smoke opt-in | Chat Completions and endpoint-advertised capabilities | verified: headless compatible tools/approval/resume; unknown structured capabilities refused |
| P05 | OpenAI-compatible provider aliases and remote `/models` discovery | none | yes: adapter wrapper over public Y provider | --provider openrouter/groq/deepseek, models | A | provideradapters / F5 | HTTP fixtures for endpoint parsing, credential routing, cancellation and provider error behavior | Chat Completions only; model capability metadata is conservative | Implemented; build-compiled, provider-specific contract fixtures pending |
| T01 | tools.RegisterFilesystem → read_file | none | yes: coding read | tools inspect; agent tool read_file | A | harness/tools / F2–F3 | read_file descriptor/schema/behavior fixture + policy/queue/cancel assertion | bounded bytes/line ranges | verified coding/worker fixtures |
| T02 | tools.RegisterFilesystem → write_file | none | yes: coding mutate | tools inspect; agent tool write_file | A | harness/tools / F2–F3 | write_file descriptor/schema/behavior fixture + policy/queue/cancel assertion | write limit and approval | verified coding/worker fixtures |
| T03 | tools.RegisterFilesystem → list_files | none | yes: coding read | tools inspect; agent tool list_files | A | harness/tools / F2–F3 | list_files descriptor/schema/behavior fixture + policy/queue/cancel assertion | ignore/symlink/entry limit | implemented; shared F3 policy/store/queue |
| T04 | tools.RegisterFilesystem → search | none | yes: coding read | tools inspect; agent tool search | A | harness/tools / F2–F3 | search descriptor/schema/behavior fixture + policy/queue/cancel assertion | built-in text fallback | verified coding/worker fixtures |
| T05 | tools.RegisterFilesystem → edit | none | yes: coding mutate | tools inspect; agent tool edit | A | harness/tools / F2–F3 | edit descriptor/schema/behavior fixture + policy/queue/cancel assertion | exact replacement and approved diff | verified coding/worker fixtures |
| T06 | tools.RegisterFilesystem → patch | none | yes: coding mutate | tools inspect; agent tool patch | A | harness/tools / F2–F3 | patch descriptor/schema/behavior fixture + policy/queue/cancel assertion | validated patch and atomic failure | verified coding/worker fixtures |
| T07 | tools.RegisterShell → run_command | none | yes: coding mutate | tools inspect; agent tool run_command | A | harness/tools / F2–F3 | run_command descriptor/schema/behavior fixture + policy/queue/cancel assertion | process tree, timeout and bounded stdout/stderr | implemented; shared F3 policy/store/queue |
| T08 | tools.RegisterGit → git_status | none | yes: coding read | tools inspect; agent tool git_status | A | harness/tools / F2–F3 | git_status descriptor/schema/behavior fixture + policy/queue/cancel assertion | requires Git | implemented; shared F3 policy/store/queue |
| T09 | tools.RegisterGit → git_diff | none | yes: coding read | tools inspect; agent tool git_diff | A | harness/tools / F2–F3 | git_diff descriptor/schema/behavior fixture + policy/queue/cancel assertion | requires Git | implemented; shared F3 policy/store/queue |
| T10 | tools.RegisterGit → git_log | none | yes: coding read | tools inspect; agent tool git_log | A | harness/tools / F2–F3 | git_log descriptor/schema/behavior fixture + policy/queue/cancel assertion | requires Git | implemented; shared F3 policy/store/queue |
| T11 | tools.RegisterGit → git_branch | none | yes: coding mutate | tools inspect; agent tool git_branch | A | harness/tools / F2–F3 | git_branch descriptor/schema/behavior fixture + policy/queue/cancel assertion | conservative classification; inspect arguments | implemented; shared F3 policy/store/queue |
| T12 | tools.RegisterGit → git_checkout | none | yes: coding mutate | tools inspect; agent tool git_checkout | A | harness/tools / F2–F3 | git_checkout descriptor/schema/behavior fixture + policy/queue/cancel assertion | approval and workspace serialization | implemented; shared F3 policy/store/queue |
| T13 | tools.RegisterGit → git_commit | none | yes: coding mutate | tools inspect; agent tool git_commit | A | harness/tools / F2–F3 | git_commit descriptor/schema/behavior fixture + policy/queue/cancel assertion | explicit paths and approved diff | implemented; shared F3 policy/store/queue |
| T14 | tools.RegisterGit → git_show | none | yes: coding read | tools inspect; agent tool git_show | A | harness/tools / F2–F3 | git_show descriptor/schema/behavior fixture + policy/queue/cancel assertion | bounded optional patch | implemented; shared F3 policy/store/queue |
| T15 | tools.RegisterGit → git_history | none | yes: coding read | tools inspect; agent tool git_history | A | harness/tools / F2–F3 | git_history descriptor/schema/behavior fixture + policy/queue/cancel assertion | commit and line provenance | implemented; shared F3 policy/store/queue |
| T16 | tools.RegisterGit → git_blame | none | yes: coding read | tools inspect; agent tool git_blame | A | harness/tools / F2–F3 | git_blame descriptor/schema/behavior fixture + policy/queue/cancel assertion | line provenance | implemented; shared F3 policy/store/queue |
| T17 | tools.RegisterGit → git_search_history | none | yes: coding read | tools inspect; agent tool git_search_history | A | harness/tools / F2–F3 | git_search_history descriptor/schema/behavior fixture + policy/queue/cancel assertion | string/regex history query | implemented; shared F3 policy/store/queue |
| T18 | tools.RegisterCodeSearch → search_text | none | yes: coding mutate | tools inspect; agent tool search_text | A | harness/tools / F2–F3 | search_text descriptor/schema/behavior fixture + policy/queue/cancel assertion | external rg process conservatively queued; explicit built-in fallback | implemented; shared F3 policy/store/queue |
| T19 | tools.RegisterCodeSearch → search_ast | none | yes: coding mutate | tools inspect; agent tool search_ast | A | harness/tools / F2–F3 | search_ast descriptor/schema/behavior fixture + policy/queue/cancel assertion | external ast-grep required; no silent text-equivalence claim | implemented; shared F3 policy/store/queue |
| T20 | tools.RegisterCodeSearch → outline | none | yes: coding mutate | tools inspect; agent tool outline | A | harness/tools / F2–F3 | outline descriptor/schema/behavior fixture + policy/queue/cancel assertion | external AST tooling; inspect actual parser support | implemented; shared F3 policy/store/queue |
| T21 | tools.RegisterCodeSearch → find_files | none | yes: coding read | tools inspect; agent tool find_files | A | harness/tools / F2–F3 | find_files descriptor/schema/behavior fixture + policy/queue/cancel assertion | fallback walk and ignore | implemented; shared F3 policy/store/queue |
| T22 | tools.RegisterLSP → definition | feature_lsp | yes: coding read | tools inspect; agent tool definition | A | harness/tools / F2–F3 | definition descriptor/schema/behavior fixture + policy/queue/cancel assertion | LSP UTF16/source positions | implemented; shared F3 policy/store/queue |
| T23 | tools.RegisterLSP → references | feature_lsp | yes: coding read | tools inspect; agent tool references | A | harness/tools / F2–F3 | references descriptor/schema/behavior fixture + policy/queue/cancel assertion | semantic references | implemented; shared F3 policy/store/queue |
| T24 | tools.RegisterLSP → implementations | feature_lsp | yes: coding read | tools inspect; agent tool implementations | A | harness/tools / F2–F3 | implementations descriptor/schema/behavior fixture + policy/queue/cancel assertion | server capability dependent | implemented; shared F3 policy/store/queue |
| T25 | tools.RegisterLSP → hover | feature_lsp | yes: coding read | tools inspect; agent tool hover | A | harness/tools / F2–F3 | hover descriptor/schema/behavior fixture + policy/queue/cancel assertion | server capability dependent | implemented; shared F3 policy/store/queue |
| T26 | tools.RegisterLSP → type_definition | feature_lsp | yes: coding read | tools inspect; agent tool type_definition | A | harness/tools / F2–F3 | type_definition descriptor/schema/behavior fixture + policy/queue/cancel assertion | server capability dependent | implemented; shared F3 policy/store/queue |
| T27 | tools.RegisterLSP → callers | feature_lsp | yes: coding read | tools inspect; agent tool callers | A | harness/tools / F2–F3 | callers descriptor/schema/behavior fixture + policy/queue/cancel assertion | call hierarchy capability | implemented; shared F3 policy/store/queue |
| T28 | tools.RegisterLSP → callees | feature_lsp | yes: coding read | tools inspect; agent tool callees | A | harness/tools / F2–F3 | callees descriptor/schema/behavior fixture + policy/queue/cancel assertion | call hierarchy capability | implemented; shared F3 policy/store/queue |
| T29 | tools.RegisterLSP → document_symbols | feature_lsp | yes: coding read | tools inspect; agent tool document_symbols | A | harness/tools / F2–F3 | document_symbols descriptor/schema/behavior fixture + policy/queue/cancel assertion | nested symbols | implemented; shared F3 policy/store/queue |
| T30 | tools.RegisterLSP → workspace_symbols | feature_lsp | yes: coding read | tools inspect; agent tool workspace_symbols | A | harness/tools / F2–F3 | workspace_symbols descriptor/schema/behavior fixture + policy/queue/cancel assertion | language-specific server | implemented; shared F3 policy/store/queue |

The descriptor inventory lists 30 built-in coding tools. `RegisterGit` may use
argument-sensitive capabilities; the product's conservative classification must
be verified against actual descriptors/requests before changing a mutation into
a parallel read. External shell/search processes are conservatively serialized
until their classification is established. F0 `TestPublicToolCatalog` verifies all 30 descriptors without invoking a tool or
starting a language server. Extension-provided task/subagent/
memory/MCP/WASM tools are represented by their registration families and expanded
into concrete descriptors when the F1/F4 composition is built.

## SDK gap backlog and responsibility

| Gap | Observed baseline | Required change / owner | Acceptance gate |
| --- | --- | --- | --- |
| Y1 | Public auth resolver/refresh contracts exist; OAuth/device/PKCE/loopback flows are in `internal/auth` | Y auth component: export reusable injected HTTP/config/notification flows; Opera credentials component adapts storage/UI | F1 external-consumer OAuth fixture; no terminal I/O from SDK |
| Y2 | Public State/Event/Transcript/Operation/Session/Branch interfaces; reusable SQLite adapter lives in `internal/storage` | Y persistence component: public SQLite adapter, migrations/readiness and all required store implementations; do not assume private adapter already implements every store | F1 contracts + crash/transaction tests with CGO=0 |
| Y3 | Public memory.Store/ReadinessChecker; filesystem implementation in `internal/memory` | Y memory/persistence component: public filesystem adapter with injected root | F1 store contract and F4 persistence lifecycle |
| Y4 | Task Manager exports Create/List/Get/Refine/Start/Complete but no public snapshot/restore | Y task-manager component: versioned snapshot/restore preserving IDs/parentage/order | F1 round-trip + malformed hierarchy rejection |
| Y5 | AgentRunner constructs a private Agent; durable events/control interfaces exist | Y runner component: public active execution event/control boundary for steer/follow-up/cancel | F1 public consumer, concurrency and cancellation fixtures |
| Y6 | Runner wraps its base registry; runtime views can supply captured registries | Y runner/tools component: prove or repair captured-view durable enforcement | F1 actual RuntimeView + OperationStore integration observes Begin/Complete/dedup/in-doubt |

No gap is marked not-applicable for lack of public access. The Opera F1 harness
component coordinates these changes, updates SDK documentation/changelog and
consumes a **published version** containing them. Temporary local integration is
allowed, but permanent `replace` directives and private imports fail acceptance.

## Closure rules

F0 completed discovery and assignment. F1 closed Y1–Y6 and the minimum vertical flow; full product capability coverage remains a F5 gate. The inventory
and this matrix must be regenerated/reviewed after selecting a new published SDK,
when concrete extension descriptors are available in F4, and before F5 closure.
Each applicable pending row must gain an implementation link and actual passing
acceptance evidence. N/A rows need an explicit scope reason. Source-level presence
or passing upstream tests alone never means Opera integration is verified.

## F2 integration evidence

`docs/f2-validation.md` records the product integration of filesystem edits/patches,
shell, Git, text/AST search, LSP discovery/lifecycle, scoped instruction context,
text/image attachments and idle model/reasoning selection. All registered tools
use the same durable runner and workspace policy/queue. The coding integration
fixtures exercise edit approval, shell approval/read-only denial, semantic fallback
and durable read/Git operations. Full provider capability parity, workers and
extensions remain assigned to their later phases in this matrix.

## F3 integration evidence

See `docs/f3-validation.md`. Opera uses the public task manager for task tools and
versioned snapshots, the public compactor for manual transcript rewrites, and the
same public AgentRunner through Service.Execute for every mode and child. Child
records use the public subagent types; admission, approval routing and scheduling
belong to Opera so no alternate execution entry bypasses its application checks.
The SDK's standalone subagent Manager remains an alternative composition, not a
second backend activated by Opera. Product session catalog/branches bind public
checkpoints/events/operations to Opera metadata without private table access.

## F4 integration evidence

See [F4 validation](f4-validation.md) and [supported extension behavior](extensions.md).
Y v1.3.4 preserves `AgentSnapshot.RuntimeViewInstanceID` across restore; the
published-consumer and Opera approval fixtures cover a process handoff. New
extension tools use the same policy, queue and OperationStore as coding tools.
Only `logs` and `y_tools` classified reads are granted to WASM guests. Direct
filesystem/network/process/secret host capabilities are unsupported and refused;
this is an explicit product boundary for the final F5 matrix, not parity with
every SDK host operation. Memory extraction is disabled: writes/forget require
explicit intent and the operation policy. Arbitrary live MCP/memory replacement,
new WASM directories, changed warm bytes/grants and multi-module toggles require
a new CLI scope. A supported single-module transition drains captured views.

## Final applicability review (F5)

The generated index contains 1767 public declarations, including build variants,
not 1767 independent product requirements. Every capability family above has a
product implementation/evidence or an explicit scope boundary. No applicable
row remains pending for lack of a public adapter. [F5 validation](f5-validation.md)
records local evidence and the native-CI gate separately.

Y v1.4.0 publishes a provider-neutral web-search contract and registry tool
(`pkg/search` and `pkg/tools`). Opera registers `web_search` through the normal
audited tool registry and selects hosted OpenAI Responses search for
`openai-subscription` or Anthropic Messages search for `anthropic` by default.
Those adapters resolve the active provider's saved login only after approval.
Brave Search remains available as an explicit choice with a separate key, and
`search.provider = "disabled"` turns search off. Project configuration cannot
enable the external capability by selecting a provider alone; a project-selected
provider requires the user's explicit `search.provider = "native"` opt-in.

The following public SDK mechanisms are alternate compositions or intentionally
inactive product profiles, rather than missing adapters:

- Remote RPC/HTTP/WebSocket/gRPC/queue hosting and detached scheduling remain
  outside the process-owned CLI scope (C23a).
- Opera composes providers, task snapshots and process-owned child records through
  its own factories/scheduler; the SDK catalog, standalone coding convenience loop
  and standalone child scheduler are alternatives, not extra active backends.
- Memory recall/readiness/write/forget are active; automatic model extraction and
  its separate worker queue are inactive because product memory writes require
  explicit user intent. Fail-open does not bypass durable persistence.
- WASM raw filesystem/network/process/secret host grants are unsupported and
  refused; logs and classified read calls through the durable y_tools bridge are
  the supported profile. Fuel guarantees absent from the host are not claimed.
- Live reconfiguration is limited to public supported idle transitions. Aggregate
  MCP/memory replacement and changed warm bytes/grants require a new scope.
- Structured content follows actual declared provider capabilities; an unknown
  compatible endpoint is refused rather than assumed to support native contracts.
- SDK telemetry aggregates local measurements; remote tracing/export backends and
  distributed quota/tenant hosting are not enabled services in this local product.
  Durable invocation IDs, timing, policy rows and events provide local inspection.

Y v1.3.4 consumes a decision after its pending tool so a later operation receives
independent authorization. The external SDK regression covers approve and deny;
Opera's demo covers subsequent command/read/task operations and explicit close/
reopen. Its MIT license also closes the dependency-notice packaging gate.

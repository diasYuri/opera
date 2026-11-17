# F4 validation — local extensions and inspection

Date: 2026-11-16. Go 1.27.4. Published SDK Y v1.3.2, commit
`588aa70ea7a3aec37e88166605a1e72ca68f51b6`, no replace/go.work.
SDK [PR #3](https://github.com/diasYuri/y/pull/3) was merged on 2026-11-16
at merge commit `97679bd144ec20454656b11f96a7ede666f019d2`. Its v1.3.2 tag
is published and downloaded by both Opera and the independent SDK consumer.
F4 is closed; F5 remains the next planned phase.

## Implementation

`internal/modules/extensions` owns private consent settings, MCP, filesystem memory,
WASM grants/lifecycle and generation inspection. `internal/core` composes
public SDK components; CLI and slash commands share its application boundary.
The deterministic management provider feeds tools through the same AgentRunner
and SQLite operation journal as chat/run/worker. SDK v1.3.2 checkpoints the logical
runtime-view instance identity, preserving exact approvals across process handoff.

## Deterministic acceptance evidence

| Flow | Evidence |
| --- | --- |
| MCP HTTP/auth | Native MCP fixture: disabled add opens no connection; enable waits for approval; unannotated external mutation waits for policy; resumed effect executes once; credential reference is resolved without a value in JSONL |
| MCP resources/prompts | Native resource read and prompt get return fixture content through the durable tool path; discovery registers list/template schemas |
| MCP stdio | Child test process speaks native stdio transport; excludes inherited API secrets; process exits after activation/call and after a second server fails during partial startup |
| MCP failure | Deadline fixture leaves configuration disabled; no silent successful activation |
| Memory | Enable/save/forget approval handoffs, new-scope read of durable fact, authorized removal; fail-open recall versus strict readiness/write errors; degraded persistence inspection |
| WASM | Public wasmtest modules: load/call/unload with approvals, ABI trap becomes failure, missing module can be disabled; build-tag-disabled path tested independently |
| Grants/durability | Unsupported grants and workspace-controlled store refused; missing grant denied; bridge refuses mutating/no-view calls; outer and nested read both observed in OperationStore |
| Generations | Captured view survives unload until released; invalid candidate leaves previous active settings/tools; unsupported replacement and busy safe points rejected |
| Regression | F0–F3 tests remain in the full suite; worker cancellation allows code 4 only with a pending uncertain operation, preserving reconciliation requirements |

Default tests use fake providers, local HTTP/stdio fixtures and tiny generated
WASM modules. No real provider credential or paid inference was used for F4.

## Verification

`go run ./scripts check` covers formatting, architecture, version pins, module
verification, vet, all tests, independent consumer and CGO-free build.
`go run ./scripts lint` runs pinned golangci-lint. The full Opera suite runs with `-race` and both feature tags. Cross-builds target Linux/macOS/
Windows on amd64/arm64 with CGO disabled. Untagged extension/CLI diagnostics tests
also run. The SDK agent subtree and runtime-view regression race tests pass.
The Windows amd64 CLI tests also cross-compile, including process-cleanup helpers.
A compiled-binary smoke in isolated user directories passes doctor/tools/MCP/
extensions/memory inspection with valid JSONL and no model credential.
Native Windows/Linux executions remain CI work; cross-compilation is not a claim
that those operating systems ran locally.

## Supported limits

See [extension behavior](extensions.md). WASM host grants are logs and classified
read calls through y_tools; raw host effects are refused. Automatic memory
extraction is disabled. Arbitrary live graph replacement is not supported by the
product. Active scopes close at CLI exit; persisted data survives. A configuration
change can require restart and a changed tool generation rejects an old approval;
there is no automatic replay of uncertain effects. Final public capability review,
accounting, packaging and broader accessibility remain F5.

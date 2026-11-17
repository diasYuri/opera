# F1 completion and validation

Date: 2026-11-15. Toolchain: **Go 1.27.4**. Local host: macOS/arm64.
Published SDK: **github.com/diasYuri/y v1.3.1**, commit
`b25d3450185f58573c8ddb4e62e82391c78000b9`.
SDK APIs: [diasYuri/y#1](https://github.com/diasYuri/y/pull/1) (merged).
Streaming patch: [diasYuri/y#2](https://github.com/diasYuri/y/pull/2) (draft).

## Delivered

- Public Y OAuth device/PKCE/refresh APIs with injected HTTP/config/notifications,
  deterministic fixtures and no terminal/browser/storage coupling.
- Public pure-Go SQLite adapters for state/transcripts, events, operations,
  sessions, branches and execution metadata; public filesystem memory adapter;
  validated task snapshot/restore preserving IDs, hierarchy and allocator.
- Public active runner subscriptions, steering, follow-ups, cancellation and
  close; captured RuntimeView tool registries retain durable operation enforcement.
- Opera-owned factories composed through `runtime.Harness`, with dependency-ordered
  scopes and application execution draining before runners/providers/stores close.
- One application service for interactive chat, foreground run and explicit resume;
  worker mode will consume this same service in F3. No agent loop is copied.
- TTY onboarding with Huh, scoped user-owned provider/model/policy selections,
  credential/OAuth wiring and a Bubble Tea inline chat without alternate screen.
- Read/write filesystem tools, operation-bound approvals with arguments/path/diff,
  approval fingerprint revalidation under a cancellable workspace mutation queue,
  and durable workspace state. Headless approval persists and exits 3.
- Consistent checkpoints, explicit cancelled/crash continuation, completed-result
  replay, and explicit reconciliation of uncertain operations before resume (4).
- Append-only accessible chat, streaming text/JSONL, cancellation, broken-output
  propagation, English help/docs and a separately compiled public SDK consumer.

## Automated evidence

| Scope | Evidence |
| --- | --- |
| Published consumer | `testdata/sdk-consumer` is an independent module pinned to v1.3.1, without Opera imports, replacements or integration workspace; included in `scripts check` |
| Opera vertical flow | `TestVerticalApprovalCheckpointAndResume`: stream → pending approval → close → reopen → approve → write → durable replay |
| Real CLI/provider protocol | `TestHeadlessHTTPProviderApprovalAndResume`: HTTP/SSE fixture through the actual provider adapter and CLI, JSONL validation, exit 3, approved resume and no credential output |
| Denial and stale approval | `TestDenialAndChangedDiffRequireReview`: changed precondition requires a new approval ID, denial produces no mutation |
| Cancellation/recovery | `TestCancellationPersistsAndExplicitResumeContinues`; `TestUncertainEffectBlocksUntilExplicitReconciliation` |
| Lifecycle | `TestStartupFailureReleasesScopeAndWorkspace`; `TestCloseCancelsAndDrainsApplicationBeforeStores`; concurrent process ownership rejection |
| Terminal | `TestTerminalRestoredWithoutAlternateScreen` with a real PTY; bounded Unicode streaming/resize test; append-only accessibility path |
| OAuth wiring | CLI device-code fixture verifies notification and separate secret storage; SDK fixtures cover PKCE state, refresh, denial, expiry and cancellation |
| Y durable boundaries | `TestRunnerRuntimeViewCannotBypassOperationStore`, including in-doubt blocking; `TestRunnerActiveControlsAndClose`; transactional cross-connection claims/recovery and task snapshot round trip |
| OpenAI live regression | Y `TestToolNameSurvivesArgumentsEventsWithoutName` preserves tool identity from output-item metadata when argument events omit the name |

Default tests use fake providers and local HTTP/PTY fixtures. Real credentials are
used only by the explicitly enabled live proof. No secret values are recorded in
this document or test output.

## Real-provider proof

`OPERA_LIVE_TEST=1 GOTOOLCHAIN=go1.27.4 go test -run TestLiveVerticalProof -v ./internal/core`
passed against **OpenAI gpt-5-mini** using the published SDK v1.3.1:

- Run `UQTB4E35CSPWYSCXOHIZMTMDW2` reached pending approval before mutation.
- The first harness was closed; another harness explicitly resumed the checkpoint.
- The approved write created `live-proof.txt` with exactly `opera-f1`; a read tool
  verified it. Both operations passed through the durable store.
- Final checkpoint version: **7**. Durable operations: **2**.
- The serialized checkpoint did not contain the API credential. The throwaway
  workspace was removed by test cleanup.

The first live attempt exposed an existing OpenAI streaming bug (missing names
on argument completion events). It was fixed in the SDK with a sanitized regression
fixture and published as v1.3.1 before this successful proof. v1.3.0 retains the
new APIs; Opera consumes the corrected patch version.

## Validation commands and limits

Local checks include `scripts check`, golangci-lint v2.15.0, race detection on the
changed Opera packages, SDK default/full-feature suites, SDK vet/architecture/lint,
and SDK race tests for changed runtime/storage/auth/task components. The six-target
matrix compiles Linux/macOS/Windows on amd64/arm64 with `CGO_ENABLED=0`, including
credential and dependency test binaries. Build/mod resolution uses `GOWORK=off`;
the temporary local integration workspace was removed.

Cross-compilation is not native Linux/Windows execution. Opera CI is configured
for all three systems but has not been published/run remotely from this local
repository. Native Windows ACL/console behavior remains delegated to that CI.
The SDK API PR #1 was merged and its Linux/macOS/Windows checks passed. The
v1.3.1 streaming fix follows in draft PR #2; its tag is published, while review
and remote checks for that follow-up are separate from the local evidence.

F2–F5 remain planned: broader coding tools/context/LSP, complete session management,
workers/subagents, MCP/WASM/memory product commands, accounting and packaging.
Opera has not been published. The SDK tag is the required external dependency.

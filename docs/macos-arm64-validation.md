# Native macOS arm64 acceptance

Validated on 2026-11-16 on macOS 26.5.2 (25F84), Darwin arm64, using Go
1.27.4 and published Y v1.3.4, without a local module replacement. All commands
below exited successfully. No product code changes were necessary.

| Validation | Result |
| --- | --- |
| `go run ./scripts check` | Formatting/boundaries, module verification, vet, tests, external consumer and CGO-free build passed |
| `go run ./scripts lint` | golangci-lint v2.15.0: zero issues |
| `go test -count=1 -json ./...` | 96 passed test/subtest events; one opt-in live test skipped |
| `go test -race -count=1 -json ./...` | 96 passed test/subtest events; no race reports; live test skipped |
| Feature-disabled `go test -count=1 -json ./...` | 93 passed test/subtest events; live test skipped |
| `go -C testdata/sdk-consumer test -count=1 -mod=readonly -json ./...` | Independent public SDK consumer passed |
| Opt-in `TestLiveVerticalProof` | OpenAI gpt-5-mini passed: approved write, checkpoint/resume and durable read; two operations, checkpoint 7 |
| `go run ./scripts package-smoke` | Extracted package passed version, help, doctor and all four demos |

Counts include parent tests and subtests; they are not counts of independent
scenarios. Packages with no test files are separate from the single skipped live
test. The live test was run separately with an existing credential and passed;
credentials are not included in evidence.

The normal profile uses `GOTOOLCHAIN=go1.27.4`, `GOWORK=off`, `CGO_ENABLED=0`
and `GOFLAGS=-tags=feature_lsp,feature_wasm_ext`. The race invocation uses
`CGO_ENABLED=1`, as required by the detector. The feature-disabled invocation
uses empty `GOFLAGS`. Enable the live invocation with `OPERA_LIVE_TEST=1` and
run `go test -count=1 -json -run '^TestLiveVerticalProof$' ./internal/core`.

The uncached suite covered approval/denial, changed preconditions, persisted
resume, branches, compaction, uncertain effects, steering/follow-ups,
subagent hierarchy and budgets, serialized workspace writes and canceled queue
waiters, memory, MCP stdio/HTTP cleanup, WASM capabilities/reload, accounting,
structured output and public runtime-view operation mediation. Terminal coverage
included the native Unix PTY restoration test, no alternate screen, Unicode,
long streaming, resize, paste, keyboard selection and accessible status output.
Some terminal behaviors are exercised through model-level tests rather than
manual terminal interaction.

Package smoke ran the extracted `0.1.0-local` binary outside the checkout. The
chat/run/worker demos traversed planning, child-agent work, approved and denied
effects, persistence and explicit handoffs using the shared execution service.
The headless demo used separate CLI processes for pending approval, approved
resume and denial. Package checks verified metadata, dependency pins, checksums
and the absence of a local SDK replacement or a CGO build dependency.

Tested archive: `dist/opera_0.1.0-local_darwin_arm64.tar.gz`.
SHA-256: `9a4d1a2b30f26ef348ab10d6362d66c204be98e0f3f180b8a173c31ad9f95642`.
Live run: `3ENBTKGLFAH252SUDXFSNVVO22`.

Raw logs, individual test outcomes, commands, elapsed times and exit codes are
retained in `.agents/validation/macos-arm64-2026-11-16/`. The accompanying
`run.py` driver reproduces each named validation mode from this checkout.

Native acceptance for this macOS arm64 environment passed. Linux, Windows and
macOS amd64 execution remain separate, pending CI gates; these results do not
certify those environments. No Opera release or publication was performed.

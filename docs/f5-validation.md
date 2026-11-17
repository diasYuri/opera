# F5 validation — implementation and local acceptance

Date: 2026-11-16. Go 1.27.4. Published Y **v1.3.4**, commit
`3fc84a570391fc0b3b7b2067ee6d6904e0ec6a94`; Opera and the independent SDK consumer
use the published module, without replace/go.work. SDK [PR #4](https://github.com/diasYuri/y/pull/4)
was merged on 2026-11-16 at commit
`aa1c4b1c01734898d1b28ca6b6cc884dde5bdcde`. The v1.3.4 tag remains the pinned
published dependency. No Opera repository/release/registry
publication was performed.

## Delivered behavior

| Roadmap | Implementation and evidence |
| --- | --- |
| F5.1–2 usage | `internal/accounting` journals provider invocations before streaming, updates cumulative samples, preserves incomplete requests and records price source/date; public telemetry builds aggregates. Accounting tests verify repeated samples, estimates, unavailable cost and sourced prices; harness tests cover replay/session/branch/compaction preservation; demo aggregates descendants |
| F5.3 diagnostics | Public policy AuditEvent rows persist allow/deny/require-approval with argument hash/redaction. Session event/operation/decision/output filters use IDs/cursors. Over-budget output is privately archived before preview truncation and included in redacted export; large-output regression proves it survives |
| F5.4 structured content | Capability gate and public schema/result validation; valid JSON, intermediate tool followed by JSON, wrong type, invalid JSON and unsupported provider tests. Response contracts persist in metadata and SDK snapshots; transport JSONL remains independent |
| F5.5 accessibility | Keyboard selector confirmation/escape repaired; NO_COLOR presence, no-motion, bounded Unicode/paste/resize and append-only responsive follow-ups/steering/approval input. Unix PTY tests pass; Windows ConPTY tests compile and are required in native jobs |
| F5.6 docs | Installation, quickstart, auth/config, models/tools, accounting/schema, policies/inspection/recovery, extensions/memory, accessibility and troubleshooting in English |
| F5.7 examples | `demo chat/run/worker`, shell/PowerShell examples, and `demo headless` for real CLI-process exit-3 approval/resume/denial in isolated temporary workspaces |
| F5.8 packages | Six local Linux/macOS/Windows amd64/arm64 archives with binary, MIT license, linked dependency notices, documentation/examples, version metadata and SHA256SUMS; pinned Go, feature tags, CGO=0 and public SDK verified |
| F5.9 native package tests | Extraction/checksum/build-info/version/help/doctor and offline execution smoke implemented; native macOS arm64 executed locally. Six native runner jobs include package smoke and actual PTY/ConPTY tests. Remaining native results require CI; cross-builds are not native execution |
| F5.10 SDK review | Regenerated 1746-declaration inventory at v1.3.4; final capability-family matrix documents supported profiles, alternatives and scope exclusions rather than leaving applicable adapter gaps |
| F5.11 acceptance | Deterministic suite plus offline demo traverse a persisted three-task hierarchy, a child read, allowed/denied effects, command/read, persistence and explicit handoffs through chat/run/worker. Separate regression fixtures cover overlapping writers, active controls/cancel, uncertain-effect reconciliation, branches/compaction and memory/MCP/WASM |

## Reproducible commands

```sh
GOTOOLCHAIN=go1.27.4 go run ./scripts check
GOTOOLCHAIN=go1.27.4 go run ./scripts lint
GOTOOLCHAIN=go1.27.4 CGO_ENABLED=1 GOFLAGS=-tags=feature_lsp,feature_wasm_ext go test -race ./...
GOTOOLCHAIN=go1.27.4 go run ./scripts package
GOTOOLCHAIN=go1.27.4 go run ./scripts package-smoke
```

Checks cover formatting, boundaries, module pins/checksums, vet, tests, the external
consumer and a CGO-free build. Package smoke executes extracted binaries away from
the source checkout. Feature-disabled tests distinguish build availability from
runtime failure. Windows amd64/arm64 UI/ConPTY test binaries cross-compile.
The public SDK agent subtree, approval/runtime-view race regressions and agent
lint pass. SDK v1.3.4 fixes a stale decision affecting later operations, validates structured final content after tools, and includes
the missing module license discovered by the package audit.

## Real provider proof

Opt-in `TestLiveVerticalProof` passed with OpenAI `gpt-5-mini`:
run `QCFACEOPR33E3RFS4PPLXIQMPW`, checkpoint 7, two durable operations. The test
uses a temporary workspace, a user credential resolver and actual write approval,
handoff and read verification; snapshots are checked for credential leakage.
Default tests remain account-free. Earlier attempts showed nondeterministic tool
selection; the proof now requests one initial write explicitly and independently
checks for a successful durable read, using an explicit verification follow-up if
needed. It never substitutes a text claim for the observed effect/read.

Default status accents have calculated contrast ratios of 6.45:1 (ANSI 25 on
white) and 9.06:1 (ANSI 75 on black); custom terminal palettes can differ.
Documentation links and fenced blocks were checked.

## Remaining execution gate

The fresh, uncached [native macOS arm64 acceptance](macos-arm64-validation.md)
on 2026-11-16 passed checks, lint, full/race/feature-disabled suites, the external
consumer, a real-provider proof and extracted-package execution. Per-command logs
and summaries are retained under `.agents/validation/macos-arm64-2026-11-16/`.

Implementation and local validation are complete. Full cross-platform acceptance
is **not yet certified**: native Linux and Windows (and the other unexecuted native
architecture jobs) have not run from this macOS machine. `.github/workflows/ci.yml`
defines all six native package jobs and retains local build artifacts for seven
days without a release. Those jobs require an authorized CI checkout; Opera has
not been published merely to trigger them. Windows ConPTY is cross-compiled,
not claimed executed locally. The roadmap leaves F5.9/F5.11's remote execution
and final all-platform sign-off open until those results exist.

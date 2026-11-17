# F0 completion and validation

Date: 2026-11-15. Local host: macOS/arm64. Toolchain: **go1.27.4**.

## Delivered

- Git repository on `main`; module `github.com/diasYuri/opera`; `cmd/opera` and
  a local `bin/opera` build. No commit, push or public release was created.
- Exact Go/Charm and published Y v1.2.0 dependency pins/checksums. TOML, keyring,
  terminal detection and lint tools are pinned too; no `replace` or `go.work`.
- Working help/version, configuration/provenance/schema/paths, local doctor and
  credential set/status/delete. Help/version/schema need no workspace or account.
- Full config precedence and typed validation, explicit empty/false handling,
  resource budgets, and project permission/store escalation rejection.
- Native keyring adapter plus explicitly selected unencrypted file fallback;
  private ownership/permissions or Windows ACLs, atomic per-reference writes,
  no raw secret arguments/output and safe native-error handling.
- Six accepted architecture decisions; English docs/examples and the stable
  JSONL/exit-code/path contracts.
- Reproducible Y public API index (1690 declarations in 37 consumer packages),
  30 built-in tool descriptors verified through public registrations, and
  capability coverage with responsible components, future tests and Y1–Y6 gaps.
- Pinned CI on Linux/macOS/Windows: formatting/architecture, vet, tests, lint,
  CGO-free builds, module/inventory reproducibility, CLI smoke and native race
  jobs; six-target cross-compilation of binaries and platform/dependency tests.

## Checks executed locally

| Check | Result / scope |
| --- | --- |
| `GOTOOLCHAIN=go1.27.4 go run ./scripts check` | Passed: exact pins, no private Y import/local replacement, gofmt, `go mod verify`, `go vet`, tests, CGO-free binary |
| `GOTOOLCHAIN=go1.27.4 go run ./scripts lint` | Passed with golangci-lint v2.15.0: zero issues |
| `GOTOOLCHAIN=go1.27.4 CGO_ENABLED=1 go test -race ./...` | Passed on macOS/arm64 |
| `GOTOOLCHAIN=go1.27.4 go run ./scripts matrix` | Passed: Linux/macOS/Windows × amd64/arm64; CLI and credential/dependency test binaries compiled with CGO=0 |
| Fresh public module cache | Download/verify and complete `go test -mod=readonly ./...` passed with a temporary empty GOMODCACHE, GOPROXY=https://proxy.golang.org, GOSUMDB=sum.golang.org, GOWORK=off and CGO=0; cache removed afterwards |
| Published SDK baseline | Remote v1.2.0 resolves to the audited local commit; no source diff; published module checksum verified |
| Real compiled-binary smoke | Passed help/version/config/paths/JSONL doctor, piped dummy-secret set/status/delete, missing status and exit code 2 for invalid/unavailable commands; no secret emitted |
| Dependency consumer | Passed public empty Harness lifecycle + actual Charm constructors together; no terminal/provider started |
| Config/security regressions | Passed precedence, explicit false/empty, invalid TOML/types/limits, project escalation/store rejection, safe errors, private Unix permissions/symlinks, secret persistence and independent reference writes |
| Documentation/inventory consistency | Local links, Markdown fences and deterministic inventory verified |

The clean-cache check explicitly selected the cached Go 1.27.4 GOROOT as well as
its executable: the host exports an older GOROOT globally. No global installation
or environment configuration was modified. Ordinary documented commands use
GOTOOLCHAIN=go1.27.4, which selects the matching root automatically.

## Validation boundaries

Native Linux/Windows execution and Windows ACL behavior are configured in CI but
were not executed on this macOS host. Cross-compilation is not a native runtime
test. The repository has not been pushed, so no remote GitHub Actions result is
claimed. The Windows-specific suite rejects an Everyone ACL and the lifecycle
suite runs against the user-only ACL implementation on a native runner.

No real keyring entry, OAuth credential or paid provider was used during testing.
The keyring adapter uses a pinned native integration; explicit `auth` operations
can exercise the user's native service. F1 must add opt-in live provider/OAuth
tests. Doctor only checks local presence/configuration, not remote authentication.

F0's acceptance is complete: local commands work, dependencies resolve from a
fresh public module cache, architecture/gaps are assigned and checks pass locally.
F1–F5 remain unimplemented. Nothing here claims a working coding conversation,
execution engine, operation store integration, worker or resumable agent yet.

This document is the historical F0 record. [F1 validation](f1-validation.md) supersedes its statements about capabilities not yet implemented and the SDK baseline.

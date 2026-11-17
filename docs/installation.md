# Local installation and packages

Opera ships as local artifacts only. It has no daemon, remote worker service,
automatic worktree or background job. The CLI owns every execution and child.

## Build from source

Use Go **1.27.4**. From the Opera checkout:

```sh
GOTOOLCHAIN=go1.27.4 CGO_ENABLED=0 go build -tags=feature_lsp,feature_wasm_ext -trimpath -o bin/ ./cmd/opera
./bin/opera --version
./bin/opera --help
./bin/opera doctor
./bin/opera demo worker
```

On PowerShell set `$env:GOTOOLCHAIN='go1.27.4'` and `$env:CGO_ENABLED='0'`,
then use the same build command and `./bin/opera.exe`. Standard helpers/CI set
both feature tags. An untagged build reports WASM disabled by build rather than
pretending a load failure. No checkout of Y or local replacement is needed.

## Generate and verify packages

```sh
GOTOOLCHAIN=go1.27.4 go run ./scripts package
GOTOOLCHAIN=go1.27.4 go run ./scripts package-smoke
```

Default version: `0.1.0-local`. Set `OPERA_VERSION` to a semantic version for a
local build; this does not create a release, tag or publication. Packaging builds
Linux/macOS/Windows on amd64/arm64 with CGO disabled and `-trimpath`. It writes
six `.tar.gz`/`.zip` archives and `dist/SHA256SUMS`. Each includes the binary,
MIT license, third-party notices, README, operational docs, examples and
`version.json`. Notices are generated from modules actually linked by any of
the six targets and the Go runtime license; a missing dependency license fails
the package command. No package registry or GitHub release is contacted.

`package-smoke` verifies the native archive checksum, extracts it into a temporary
directory, checks embedded Go/SDK settings and absence of module replacements,
then runs version/help/doctor, the chat/run/worker offline demos and a headless
exit-3 approval/denial proof across CLI processes. The demos
need neither Go on PATH nor credentials when run from the packaged CLI. The Go
helper itself requires the pinned development toolchain. Builds carry no Y
checkout path and never invoke the `y` binary as a backend.

## Install an extracted binary

Verify the archive's SHA-256 against `SHA256SUMS` before extracting it. On Unix,
use `sha256sum` or `shasum -a 256`; on PowerShell use `Get-FileHash -Algorithm
SHA256`. Extract the matching archive and copy `opera`/`opera.exe` into a directory
on your PATH. Keep the bundled license/notices with redistributed copies.
Run `opera doctor` and `opera demo run` from a separate empty directory.

Native package jobs target Ubuntu amd64/arm64, macOS Intel/Apple Silicon and
Windows amd64/arm64, using documented [GitHub runner labels](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
Native CI includes real Unix PTY or Windows ConPTY tests. Local cross-builds do
not substitute for those native runs; see [F5 validation](f5-validation.md).

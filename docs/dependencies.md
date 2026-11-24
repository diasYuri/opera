# Pinned foundation dependencies

Selected and resolved on 2026-11-20. `go.mod` fixes the graph and `go.sum` records
module/content checksums. There are no replacements or integration workspaces.

| Module/tool | Exact version | Verification |
| --- | --- | --- |
| Go | 1.27.4 | Latest stable 1.27 patch in [official downloads](https://go.dev/dl/?mode=json); actual local execution verified |
| `charm.land/bubbletea/v2` | v2.1.2 | [Tag module declaration](https://github.com/charmbracelet/bubbletea/blob/v2.1.2/go.mod) |
| `charm.land/bubbles/v2` | v2.2.2 | [Tag module declaration](https://github.com/charmbracelet/bubbles/blob/v2.2.2/go.mod) |
| `charm.land/huh/v2` | v2.0.3 | [Tag module declaration](https://github.com/charmbracelet/huh/blob/v2.0.3/go.mod) |
| `charm.land/lipgloss/v2` | v2.1.1 | [Tag module declaration](https://github.com/charmbracelet/lipgloss/blob/v2.1.1/go.mod) |
| `github.com/charmbracelet/harmonica` | v0.2.0 | [Release](https://github.com/charmbracelet/harmonica/releases/tag/v0.2.0) |
| `github.com/diasYuri/y` | v1.4.0 | Published SDK commit `0a0f4665c7b397eda960aa0e19c9f73ac81c1e73`; includes the provider-neutral web-search contract and registry tool |
| `github.com/pelletier/go-toml/v2` | v2.4.3 | Go module download and checksums |
| `github.com/zalando/go-keyring` | v0.2.8 | Go module download and checksums; native stores do not require cgo |
| `golang.org/x/sys` | v0.50.0 | Effective module graph; Windows ACL API |
| `golang.org/x/term` | v0.48.0 | Effective module graph; stdin TTY detection |
| `github.com/golangci/golangci-lint/v2` | v2.15.0 | Versioned `go run` from development helper; built with Go 1.27.4 |

The Charm modules use `charm.land` paths, not their GitHub URLs. They are compiled
together with Y in `internal/foundation/dependencies_test.go`; F0 does not start
a terminal UI or provider. SDK tool descriptors are also registered/compared
without invoking external executables.

GitHub Actions checkout/setup-go are pinned to full commit SHAs in the workflow.
Build/test tooling otherwise uses Go's standard tools. The development matrix
generates raw local binaries; release archives use the in-repository packager.
Beta releases are published only from version tags after CI and native package
smokes succeed; local package commands do not publish artifacts.

Development, CI and module minimum use the same exact Go patch. The roadmap
allows updating to a newer stable 1.27 patch when selected explicitly and updated
consistently; CI never floats to latest or downloads a different series implicitly.
All build/check helper subprocesses set `CGO_ENABLED=0` and `GOWORK=off`. Race
testing is a separate supported-native job with cgo enabled.

Local package notices are generated from the union of linked dependency modules
for all six targets, with their versions/checksums and verbatim license/notice
files. Missing licenses fail packaging. The SDK v1.4.0 includes its MIT license.

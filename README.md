# Opera

<p align="center">
  <strong>Your coding agent, in your terminal — under your control.</strong><br>
  Inspect. Plan. Build. Review.
</p>

Opera is a local-first coding workspace built on the public
[Y SDK](https://github.com/diasYuri/y). Choose your model, work in your own
workspace, review proposed changes, and resume conversations from durable
checkpoints.

<p align="center">
  <strong>Beta targets:</strong> macOS (Intel and Apple silicon) · Linux (amd64 and arm64)
</p>

## Get started

After the first beta release, install with one command:

```sh
curl -fsSL https://raw.githubusercontent.com/diasYuri/opera/main/install.sh | sh
```

The installer verifies the archive checksum, installs without Go or `sudo`, and
adds `~/.local/bin` to your shell path when needed. See [installation](docs/installation.md)
for version pinning and manual packages.

Check for and install Opera releases with `opera update check` and
`opera update apply`. In a Go project, `opera deps check` lists available module
versions; `opera deps update` upgrades dependencies within their current major
versions. Updates are explicit and require no model credentials.

Try Opera without configuring a model, then connect your own provider:

```sh
opera demo chat
opera onboarding
opera chat
```

## Why Opera

- **Reviewable changes:** the default `ask` policy prompts before mutations and
  binds approvals to the operation and file state.
- **Long-running conversations:** Backstage consolidates session context and
  compacts history as the model's context window fills. Continue or branch from
  durable checkpoints.
- **Your provider, your credentials:** use supported OAuth sign-in or configure
  an API key through Opera's credential flow.
- **Local collaborators:** delegate bounded tasks to child agents sharing the
  workspace policy and write queue.
- **Terminal-first:** keyboard-driven chat, an accessible text mode, and offline
  demos for trying the workflow without an account.

Opera is a local coding harness. The model provider supplies inference; Opera
keeps workspace tools, approvals, checkpoints and usage records in the CLI.

## Build from source

Use **Go 1.27.4**. The explicit toolchain selection downloads the required version
into Go's toolchain cache without replacing your system installation.

```sh
GOTOOLCHAIN=go1.27.4 CGO_ENABLED=0 go build -tags=feature_lsp,feature_wasm_ext -trimpath -o bin/ ./cmd/opera
./bin/opera --help
./bin/opera --version
./bin/opera config
./bin/opera config paths
./bin/opera config schema
./bin/opera doctor
./bin/opera doctor --format jsonl
```

On PowerShell, set `$env:GOTOOLCHAIN='go1.27.4'` and `$env:CGO_ENABLED='0'`,
then run the same `go build` command and `./bin/opera.exe`.

Configuration precedence is **flags → OPERA_* → project → user → defaults**.
See [configuration](docs/configuration.md), the [example TOML](examples/config.toml)
and the [CLI/output contract](docs/cli-contract.md). The default policy is `ask`.
Project configuration cannot elevate the user's policy or select a secret store.

Secrets are stored in the native keyring by default. `auth set <reference>` reads
the secret from piped stdin; it never accepts a secret as an argument or displays
one. An explicitly selected `--store file` uses an **unencrypted**, user-only
file store. There is no automatic fallback. See [credential storage](docs/adr/006-credentials.md).

## Development

```sh
GOTOOLCHAIN=go1.27.4 go run ./scripts check
GOTOOLCHAIN=go1.27.4 go run ./scripts lint
GOTOOLCHAIN=go1.27.4 go run ./scripts matrix
GOTOOLCHAIN=go1.27.4 go run ./scripts inventory
GOTOOLCHAIN=go1.27.4 go run ./scripts package
GOTOOLCHAIN=go1.27.4 go run ./scripts package-smoke
```

The helper works on Linux, macOS and Windows without Make or shell scripts.
`make check`, `make lint`, `make matrix` and `make inventory` are shortcuts.
`check` verifies formatting, architectural boundaries, dependency pins/checksums,
vet, tests and a CGO-free build. Lint is pinned to golangci-lint **v2.15.0** and
compiled using the selected Go version. It does not require a globally installed linter.
Cross-builds cover Linux/macOS/Windows on amd64/arm64, including compilation of the
real dependency consumer and platform-specific credential tests.

Race detection is separate: `GOTOOLCHAIN=go1.27.4 CGO_ENABLED=1 go test -race ./...`
requires a supported native C compiler. Distributed binaries remain CGO-free.

The published Y baseline is **v1.4.0**. No local replacement or adjacent checkout
is required to build. [SDK coverage](docs/y-capability-coverage.md) records the
public inventory and verified SDK additions. Opera includes approved web search
through Codex and Claude sign-in, with Brave Search as an explicit alternative.
[Architecture decisions](docs/adr/README.md) define
the boundaries for subsequent phases. [F0 validation](docs/f0-validation.md)
records what was actually checked locally and what requires native CI.

All execution remains attached to the CLI process. Beta releases are published
from explicit `v*-beta*` tags after the native package checks pass; there is no
daemon, remote worker service or automatic worktree.

## Foreground execution

```sh
./bin/opera onboarding
./bin/opera chat
./bin/opera run "Inspect this workspace" --provider openai --model gpt-5-mini --policy ask
./bin/opera run "Create proof.txt with the requested content" --provider openai --model gpt-5-mini --policy ask --format jsonl
./bin/opera resume <run-id> --approve <approval-id>
./bin/opera resume <run-id> --deny <approval-id>
```

Use a provider API-key environment variable or `auth set`/`auth login`; never put
credentials in the prompt or flags. Onboarding saves provider/model/workspace
policy in a private **user-owned workspace settings file**, outside project files.
Interactive chat can start before configuration: `/login` opens the provider
submenu, connects your account and then offers its models. Headless execution
requires explicit provider/model/policy selection and never opens forms. The
OpenAI-compatible provider requires a user-controlled `--base-url`; OpenRouter,
Groq and DeepSeek use their published compatible endpoints by default and allow
a user-controlled `base_url` override. Projects cannot redirect credentials.
See the [PI provider map and Opera plan](docs/pi-provider-map.md) for supported
protocols and limits.

An approval shows the agent, tool, arguments, path and proposed content diff.
Approval IDs bind the operation and file precondition. Changed files require a
new review, including changes while waiting for the mutation queue. Headless
approval persists and returns **3**; cancel returns **130**; an uncertain effect
blocks resume with **4**. `reconcile` records the result explicitly observed by the
user; it does not execute or undo an effect. A workspace has one active Opera
process; all writes within it share a cancellable queue.

Chat uses a minimal Codex/pi-inspired layout with a growing composer, model and
workspace footer, and concise errors (`/details` opens full diagnostics). Finished
messages stay in an independent scrollable history above the fixed composer.
Interactive chat fills the alternate screen and restores the previous screen on exit. Enter sends, Alt+Enter/Ctrl+J inserts a line,
Ctrl+C cancels active work and leaves chat open, and empty Ctrl+D exits. During
execution, ordinary messages become follow-ups; `/steer` is explicit. `/approve`
and `/deny` resolve the displayed operation. `--accessible` uses append-only text.
`/mode` lists the available agent modes, and `/mode <name>` selects one for
future requests. `/plan` or Shift+Tab is a shortcut for Plan mode.
PageUp/PageDown scroll history; Ctrl+End follows the latest messages. Streaming
respects the current reading position.

See [F1 validation](docs/f1-validation.md) for acceptance evidence and scope, and
[OAuth configuration](docs/oauth.md) for registered clients and URL/device flows.
Live validation is opt-in: `OPERA_LIVE_TEST=1 GOTOOLCHAIN=go1.27.4 go test -run
TestLiveVerticalProof -v ./internal/core` (requires OPENAI_API_KEY; uses a
throwaway workspace). Default tests use fake providers and HTTP/PTY fixtures.

## Interactive coding

Build with `make build` (enables the SDK's `feature_lsp,feature_wasm_ext` tags), then run
`./bin/opera chat`. Use `/help`, `/model`, `/thinking`, `/context`, `/skills`, `/settings`,
and `/details`. `/mode` selects an agent mode; `/plan` or Shift+Tab selects Plan.
Alt+Up/Down recalls input;
Alt+Enter or Ctrl+J inserts a newline.
Bracketed paste inserts text without submitting it. Esc closes bounded viewers.

`opera run "Inspect this code" --attach src/main.go` attaches a file and its scoped
AGENTS.md instructions. `opera context src/main.go` inspects source provenance.
PNG/JPEG/GIF/WebP attachments require advertised vision support. Inputs share a
bounded byte/token budget; binary formats beyond these images are rejected.
See [F2 behavior and validation](docs/f2-validation.md).

## Sessions and local workers

```sh
./bin/opera worker "Implement and verify the change"
./bin/opera sessions list 0 20
./bin/opera sessions inspect <run-id>
./bin/opera sessions export <run-id>
./bin/opera sessions branch <run-id> <checkpoint-version>
./bin/opera resume <branch-id> "Continue with this requirement"
./bin/opera run "Next change" --continue <completed-run-id>
./bin/opera compact <completed-run-id>
./bin/opera retry <failed-run-id>
```

`/tasks`, `/agents`, `/compact` and `/retry` expose the same application state in
chat. Workers create a persisted task objective and can refine it with task tools.
Local children share policy, checkpoints and the workspace mutation queue. Defaults
allow four concurrent children and one level with unlimited shared worker tokens;
configure `agents.max_concurrent`, `agents.max_depth` and `limits.worker_tokens`
to cap their usage.
Headless approval exits 3; resume the parent using its displayed approval ID.
Inspect a child session and reconcile uncertain operations explicitly before retry.

Serialization protects overlapping effects; agents can still make logically
conflicting edits. Tools do not provide an operating-system sandbox. See the
[F3 validation report](docs/f3-validation.md) for recovery rules and evidence.

## Agent Skills

Use `$opera` to load the built-in guide to operating, developing and extending Opera.
Inspect it offline with `opera skills show opera`.

Place reusable instructions in `.agents/skills/<name>/SKILL.md` within the
workspace or `~/.agents/skills/<name>/SKILL.md` for the current user. Inspect
them with `opera skills list|show|check`; in chat, `/skills` lists the catalog
and `$` autocompletes a skill. Opera loads full instructions and bundled
resources only when needed. See [Agent Skills](docs/skills.md).

## Extensions and memory

```sh
./bin/opera mcp add local stdio /absolute/path/to/server
./bin/opera mcp enable local
./bin/opera mcp tools local
./bin/opera mcp resources local
./bin/opera extensions add example.echo /absolute/path/to/module logs
./bin/opera extensions load example.echo
./bin/opera extensions inspect
./bin/opera memory enable
./bin/opera memory save '{"type":"project_fact","scope":"workspace","summary":"Tests","content":"Run make check"}'
./bin/opera memory search tests
```

Adding a configuration leaves it disabled. Activation uses the workspace policy
and persists pending approval with exit 3. Approve using `resume` as above.
Management commands use a deterministic local provider through the same durable
runner, without a model API credential. `tools call <name> '<JSON>'` executes
registered tools with normal approvals, mutation serialization and checkpoints.
See [extension configuration and limits](docs/extensions.md) and
[F4 validation](docs/f4-validation.md).

## Usage, structured results and local packages

```sh
./bin/opera usage run <run-id>
./bin/opera sessions operations <run-id> --tool write_file
./bin/opera sessions decisions <run-id>
./bin/opera run "Return a JSON object" --response-format json_object --format jsonl
./bin/opera demo worker
```

`/usage` aggregates each provider invocation once across resume and children;
branches inherit no charge records. Reported tokens, context estimates, known/
estimated cost and unavailable prices remain distinct. JSONL transports events;
a response format validates generated model content. The accessible chat path
supports active follow-ups and steering without continuous redraw.

Start with [installation](docs/installation.md) and [quickstart](docs/quickstart.md).
See [usage/prices/structured output](docs/usage.md), [operations and recovery](docs/operations.md),
[keyboard/accessibility](docs/accessibility.md), [models/tools](docs/tools.md) and
[executable examples](examples/README.md). Local archives in `dist` contain
licenses, third-party notices, docs, examples, version metadata and checksums.
[F5 validation](docs/f5-validation.md) distinguishes verified local results from
native CI gates. Beta tags publish GitHub pre-releases for macOS and Linux after
all CI and native package checks pass; no package registry publication is used.

### ChatGPT subscription (OAuth)

Inside Opera, type `/login`, select **OpenAI**, and complete authorization in
your browser. Opera then opens the account model picker and saves your selection.
The menu supports arrows, Enter, filtering and Esc; Esc or Ctrl+C cancels an
in-progress login while keeping chat open.

Use your ChatGPT plan through Sign in with ChatGPT, with a separate provider
from the OpenAI API-key integration:

```sh
opera auth login openai-subscription
opera models --provider openai-subscription
opera run "Inspect this workspace" --provider openai-subscription --model <slug-from-models>
opera auth status openai-subscription
opera auth logout openai-subscription
```

Open the displayed URL in your browser to finish sign-in. Opera registers its
own client dynamically; no API key, client ID, or client secret is required.
Access depends on your account, workspace policy, granted permissions and plan
limits. See [ChatGPT subscription setup](docs/chatgpt-subscription.md) for
account selection, credential stores and supported inference behavior.

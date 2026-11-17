# Quickstart and command map

Start with [installation](installation.md). For an account-free proof, run:

```sh
opera demo chat
opera demo run --format jsonl
opera demo worker
opera demo headless --format jsonl
```

Each demo creates its own temporary workspace, uses an explicit scripted local
provider through the public harness, creates/refines a three-task plan, approves the fixture's allowed operations,
denies its denied write, verifies a child read and resumes checkpoints by ID.
It deletes that workspace afterward. It does not alter trust in your real project.
`demo headless` additionally verifies exit 3 and explicit approval/denial across
actual CLI processes. The same coding scenario is exercised by `internal/demo/scenario_test.go` and extracted
package smoke tests. Demos do not replace real model authentication tests.

For your workspace:

```sh
opera onboarding
opera models --provider openai
opera chat
opera run 'Inspect this code and propose a small change' --policy ask
opera worker 'Implement and verify the requested change'
opera usage
```

Choose a provider/model you can access. Configure a secret with piped stdin and
`auth set <reference>`, a supported provider API-key environment variable, or
`auth login <provider>` with the registered OAuth client configuration. Never put
a secret into a prompt or command argument. See [OAuth](oauth.md),
[credential storage](adr/006-credentials.md) and [configuration](configuration.md).
A missing keyring does not silently switch to file storage.

| Need | Command / documentation |
| --- | --- |
| Coding conversation | `opera` / `chat`; `/model`, `/thinking`, `/context`, `/mode`, `/help` |
| One foreground task | `run`; optional `--attach`, structured output, text/JSONL |
| Plan and local collaborators | `worker`; `/tasks`, `/agents` |
| Continue completed conversation | `run --continue <run-id>` |
| Recover/approve/retry | `resume`, `retry`, `reconcile`; [operations](operations.md) |
| Sessions and branches | `sessions list/inspect/export/branch`, `compact` |
| Events/operations/decisions | `sessions events/operations/decisions/output` |
| Tokens/cost provenance | `usage`, `/usage`; [usage](usage.md) |
| Provider metadata | `models`, `/model`; [models and tools](tools.md) |
| Credentials/settings | `auth`, `config`, `settings`, `doctor` |
| Tools and extensibility | `tools`, `mcp`, `extensions`, `memory`; [extensions](extensions.md) |
| Reusable agent instructions | `skills`, `/skills`, `$skill-name`; [Agent Skills](skills.md) |
| Accessible input | `chat --accessible --no-motion`; [accessibility](accessibility.md) |

Use [executable examples](../examples/README.md) for chat/run/worker, headless
JSONL and approval/resume. `opera --help` lists every flag and slash command.
All execution stays attached to the CLI; checkpoints require explicit recovery.

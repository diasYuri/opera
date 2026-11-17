# CLI and output contract (v1)

F0 introduced `--help`, `--version`, `config [show|paths|schema|get <key>]`,
`doctor`, and `auth <set|status|delete> <reference>`. F1 adds default `opera`/`chat`, `run <prompt>`, `resume <id>`, `onboarding`, OAuth login and explicit effect reconciliation. No arguments start chat on a TTY; headless callers should use run.
Unavailable commands return usage code 2; no placeholder reports execution success.
Flags may appear before or after a command. `--` ends flag parsing.

Flags: `--workspace`, `--user-config`, `--project-config`, `--provider`, `--model`,
`--policy`, `--format`, `--store`, `--set key=value`, `--no-motion[=false]`,
`--accessible[=false]`, `--help`/`-h`, `--version`, user-only `--base-url`, and resume `--approve`/`--deny`.

`doctor` checks the binary's Go version, the published SDK version, executable
presence on PATH (Git, rg, ast-grep, gopls), and whether a model is configured.
It does not run discovered executables, contact providers, validate remote
credentials, read secrets or write local state. Optional missing dependencies
are informational. A toolchain mismatch is an error. A present executable is
not a guarantee of its identity or compatibility; F2 must validate it before use.
The unrelated Unix `sg` command is not accepted as ast-grep.

## Text and JSONL

Text is the default. JSONL uses one complete JSON object per line, no ANSI output,
progress redraws or form prompts. Successful F0 commands emit one result event:

```json
{"schema_version":1,"sequence":1,"type":"config.value","data":{"value":"ask","source":"default"}}
```

Types: `config.resolved`, `config.value`, `config.paths`, `config.schema`,
`doctor.report`, `auth.status` and `error`. stdout contains result events; stderr
contains error events when JSONL is selected. Help/version remain plain text.
`data` is command-specific. Auth events report presence/action and store kind,
never a secret. Output write failures return 1. Sequence starts at 1 per command.
F1 execution events add timestamps, run/session IDs, durable SDK sequence, text/tool summaries and the normalized SDK payload in `data`. Each invocation ends with an `execution.result` summary containing the checkpoint cursor, state, IDs, exit code and pending approval. This summary is not a newly persisted SDK event.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Command completed; auth status may report a missing reference |
| 1 | Runtime, storage or output failure |
| 2 | Invalid usage/configuration or a missing required prerequisite |
| 3 | Persisted pending approval requiring explicit resume |
| 4 | In-doubt operation requiring explicit reconciliation |
| 130 | Cancelled foreground execution |

Auth set requires non-TTY stdin and accepts at most 65536 bytes after removing
one trailing LF/CRLF. It rejects empty input and never uses a terminal prompt in
F0. Use a secret manager or another trusted stdin source; do not put secrets in
command arguments or shell history. Status reads only the requested reference
and prints presence. Delete requires an explicit reference.

No-TTY approval persists and returns 3 immediately.
Ctrl+C in active chat will cancel the run and return to the editor; idle Ctrl+C
and Ctrl+D on an empty editor will exit. No job is detached from its CLI.

## F3 commands

- `worker <task>` uses the same foreground execution service with a persisted plan.
- `sessions list [offset] [limit]` returns full IDs for the latest run in each session;
  limits are 1–1000. `inspect|export <run-id>` returns versioned, redacted JSON.
- `sessions branch <run-id> [checkpoint-version]` branches a completed checkpoint,
  including archived F3 boundaries. It records source run/version, resets execution
  identities and approvals, and inherits no operation claims. Workspace files remain shared.
- `resume <run-id> [prompt]` explicitly recovers. An optional prompt is accepted for
  a recoverable branch; completed conversations use `run --continue <run-id>`.
- `retry <run-id>` follows the same recovery checks. Completed runs replay their
  durable result; uncertain operations block with code 4 until explicit reconciliation.
- `compact <run-id>` requires a completed checkpoint and archives the original before
  replacing the live transcript. Failure preserves either the original or an auditable
  prepared archive; no external effect is repeated.
- Chat adds `/tasks`, `/agents`, `/compact`, `/retry`. `/steer` is active-only;
  ordinary messages remain ordered follow-ups. Active panels support cancellation.
- Child events carry `parent_run_id`, source `run_id` and its own event sequence.
  Approval results identify agent and task. Parent resume walks its pending children
  and presents the next operation-bound approval without authorizing other operations.

Inspection acquires workspace ownership and opens stores without constructing a
provider or accessing credentials. Legacy F1/F2 IDs are indexed on explicit inspect
or resume; they had no F3 catalog entries at creation. Inspection never schedules work.

F4 adds `tools`, `mcp`, `extensions` and `memory`; see [extension commands](extensions.md).
Agent Skills discovery and inspection are read-only via `skills list|show|check`; see [Agent Skills](skills.md).
Inspection/configuration emits versioned JSONL event envelopes when requested.
Management actions use durable execution events and the existing exit/approval
contract without requiring a model credential. `doctor` never connects to an
external server; configured-unprobed, disabled-by-build and failure are distinct.

## F5 commands and content contracts

`usage [workspace|run|session|agent|turn]` exposes the durable provider-call ledger;
`/usage` selects the active/previous run tree. `models` exposes SDK metadata and
capabilities. `demo chat|run|worker` executes an explicit offline acceptance scenario
in a temporary workspace without credentials or changes to the user's project.

`sessions events|operations|decisions|output <run-id>` supports `--event-type`,
`--tool`, `--operation-id`, `--after` and `--limit` (1–1000). Full exports include
usage, policy decisions and archived over-budget tool results with redaction.
Event cursors are sequences; operation/decision cursors are offsets.

`--response-format text|json_object` or `--response-schema <path>` controls generated
model content. JSONL still transports event envelopes independently. Invalid schema,
unsupported capabilities and invalid model content fail explicitly. Validated content
is in `execution.result.data.structured_output`. Resume retains the stored contract.

JSONL chat input is unsupported and returns usage code 2; use `run --format jsonl`.
Append-only chat supports `/multiline`, `/send`, `/cancel-input` and `/quit` as well
as the ordinary application commands. Non-TTY approval exits 3 without reading an
approval answer. See [quickstart](quickstart.md), [usage](usage.md) and
[inspection/recovery](operations.md).

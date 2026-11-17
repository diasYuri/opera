# Policies, inspection and recovery

Choose a workspace policy explicitly on first execution. `ask` is recommended;
`read-only` denies effects; `trusted` permits effects within the configured limits.
None is an OS sandbox. Project configuration cannot raise the user's trust,
redirect credentialed providers or activate external modules silently.

Approvals show tool/agent IDs, relevant arguments, local file path/precondition
and a diff when available. A decision applies to one operation. Changed file
content or tool generation invalidates the old review. A deny does not authorize
later operations. Children share one workspace write queue and approval route.
Unknown external commands, MCP and WASM tools are conservatively serialized.
Serialization does not prevent logical conflicts between agents.

```sh
opera sessions list 0 20
opera sessions inspect <run-id> --format jsonl
opera sessions events <run-id> --after 12 --limit 20
opera sessions events <run-id> --event-type tool_completed
opera sessions operations <run-id> --tool write_file
opera sessions operations <run-id> --operation-id <tool-call-id-or-key>
opera sessions decisions <run-id>
opera sessions output <run-id> --operation-id <tool-call-id>
opera sessions export <run-id> --format jsonl
```

Event cursors are durable event sequence numbers. Operation cursors are offsets
in the returned stable operation order. IDs distinguish runs, event sequences,
requests and tool calls. Inspection/export applies structured secret redaction
and text scrubbing; private raw records remain local. Decisions select the durable allow/deny/require-approval policy audit rows,
with redacted arguments and their hash. `--event-type` filters their decision
kind; `--after` is an offset for decisions. Session export includes usage and any archived tool output.
Large tool previews are truncated for context/display, while an over-budget raw
result is archived privately before truncation so explicit output inspection can
recover the diagnostic. Native tools can also impose their own upstream limits.
Session inspection does not schedule agents or start providers/extensions.

## Recover explicitly

```sh
opera resume <run-id> --approve <approval-id>
opera resume <run-id> --deny <approval-id>
opera retry <run-id>
opera reconcile <run-id> <operation-key> 'Observed result and evidence'
opera sessions branch <completed-run-id>
opera compact <completed-run-id>
```

Exit 3 means approval is persisted; resolve its exact ID. Exit 4 means an operation
may have produced an effect without durable confirmation. Inspect the real state,
then reconcile the observed result explicitly. Never retry blindly or treat a
cancel as evidence that an external effect did not happen. Reconciliation records
an observation; it does not execute or undo an effect. Completed operations replay
their known result. Branches reset approvals and operation identities, keep source
provenance and share workspace files. Compaction archives its original boundary.

Cancellation reaches children, queued writers and scoped processes. Cleanup uses
an independent deadline. Abrupt process death can leave a recoverable checkpoint,
an incomplete usage observation or an uncertain operation. After approval handoff,
unexecuted companion tool calls receive an explicit interrupted result; they are
not silently performed. A model must issue any needed fresh call. Failed runtime
reconfiguration preserves the current supported generation; larger transitions
require a new scope. See [extension limits](extensions.md).

## Troubleshooting

| Symptom | Action |
| --- | --- |
| Headless execution exits 2 | Choose provider/model/policy; use `run`, not a form or JSONL chat |
| Pending approval | Resume the displayed run and exact approval ID |
| Workspace already owned | Close its other Opera process; inspect recovery after it exits |
| Changed diff or generation | Restore the original configuration or review a fresh operation; no silent permission migration |
| Exit 4 after cancel/crash | Inspect effects and reconcile by operation key |
| Missing rg/ast-grep/Git/LSP | `doctor` lists availability and explicit fallback; install externally if desired |
| WASM disabled by build | Use the standard feature-tagged build |
| MCP startup failure | Inspect disabled configuration, credential references and server; explicitly refresh/enable |
| Memory degraded | Repair filesystem readiness; fail-open does not swallow writes |
| Model rejects JSON or thinking | Check `models` and the endpoint's actual supported contract |
| Context/worker budget exhausted | Compact or explicitly adjust the documented limit; never infer that the billing total equals the budget |
| Screen reader noise | Use `--accessible --no-motion`; optionally set `NO_COLOR` |

The snapshot harness-version field is a stable checkpoint compatibility ID, not
the current SDK release. `doctor` and package build metadata report the pinned SDK.

# F3 sessions and local workers

Date: 2026-11-16. Toolchain: Go 1.27.4. SDK: published Y v1.3.1,
without local replacement. Opera remains a foreground CLI.

## Delivered behavior

- `sessions list [offset] [limit]` lists the latest run in each session with full
  IDs. `inspect` and `export` expose versioned, redacted JSON containing checkpoint,
  durable events, operations, task tree, children, control journal and product boundaries.
  These commands construct no provider, read no credentials and start no child.
- `sessions branch <run-id> [checkpoint-version]` uses a completed latest or archived
  F3 checkpoint. The new run records provenance, copies context/task identities,
  clears authorization, nonce, policy identity and pending control/approval queues,
  and receives no inherited operation claims. Workspace files are shared.
- `compact <run-id>`/`/compact` uses the public SDK compactor with bounded inference,
  archives the original checkpoint and exact dropped messages before committing,
  and keeps instruction configuration, recent messages, task and operation evidence.
  Prepared/committed archive state makes an interrupted transition inspectable.
- `retry <run-id>`/`/retry` explicitly resumes. Completed results replay from storage;
  failed/cancelled work validates workspace, model, harness and tool-generation hash,
  reapplies current policy and repairs interrupted tool-result pairs from durable
  operations. Pending effects block with code 4 until explicit reconciliation.
- `worker <task>` uses `Service.Execute`, just like chat/run/resume. It creates a
  persisted task objective; public Y task tools refine/start/complete the plan.
  The completion hook prevents open tasks from being silently discarded.
- `subagent_spawn/list/wait/cancel` are Opera application orchestration tools over
  the same service and public runner, using public Y child record types. Children
  have stable IDs, task association, parentage, depth, status and result records.
  The Opera scheduler deliberately owns admission/approvals rather than invoking
  an alternate agent loop. No Y agent loop is copied.
- Defaults allow four concurrent children, depth one; both are configurable. Nested
  waits yield/reacquire worker slots so depth > 1 also works with one slot. Queues
  are bounded; each parent can create at most 64 children. A durable shared token
  reservation ledger covers root/descendant inference and manual compaction.
- Before completion, the parent receives a checkpointed evidence message containing
  durable child results. It must make a subsequent model turn to incorporate those
  results. Stable evidence hashes prevent repeated delivery on explicit recovery.
- Tool mutations across all agents share one cancellable FIFO workspace queue.
  Known reads run concurrently; shell/unclassified handlers serialize. The queue
  never spans approval waiting. Fingerprints revalidate under the acquired lock.
  The same middleware remains available to future MCP/WASM mutating tools.
- Ownership uses the directory inode on Unix and a workspace-local lock on Windows,
  plus a file-identity-derived data lock. Symlink/case aliases and a different
  OPERA_DATA_DIR cannot create a second workspace executor.
- The parent exposes one ordered pending-child approval at a time, identifying
  agent/task/tool/arguments/path/diff. Approving one child never authorizes the parent
  or another child. A changed precondition produces another pending review.
- Root cancellation reaches child providers/tools, worker-slot waits, mutation waits
  and descendant scopes. Scope close cancels/drains executions and controls before
  runner/provider/registry/store teardown. Explicitly cancelled children are not
  silently restarted by parent recovery.
- `/tasks`, `/agents`, `/compact`, `/retry`, `/steer` and ordered follow-ups expose
  the application state in chat. Child event envelopes identify their source run
  and parent; UI keeps child text out of the parent's response block. Controls use
  durable monotonic IDs; missing accepted control messages are restored exactly
  once on explicit resume by comparing journal IDs against transcript/queues.

## Persistence and recovery boundaries

The public Y store atomically saves a checkpoint and transcript, with separate
transactions for events and operation claims. Opera owns versioned product tables
in the same SQLite file: session catalog, tasks, children, budgets, controls,
compaction archives and checkpoint boundaries. It does not read Y private tables.

A migration creates a consistent SQLite backup before product schema v1. Future
versions and malformed/corrupt databases fail before execution. Product writes
use full synchronous transactions; version-checked writes are available for CAS.
Concurrent Opera processes may write to the same workspace. The application does
not coordinate mutations across processes; users accept the resulting conflict risk.

Task mutations and child admission commit before their tool result is acknowledged.
A settled execution records the SDK checkpoint version, event cursor, task snapshot,
child records and operation references in one product document. These are separate
transactions from the SDK checkpoint. Explicit recovery rebuilds a missing boundary
from the durable stores; it never repeats uncertain effects to fill a missing record.
A crash between an effect and its operation completion requires reconciliation.

Inspection maps inactive running/queued entries to interrupted presentation state
without scheduling them. `resume` alone reconstructs necessary nonterminal children
with original IDs. Legacy F1/F2 IDs enter the catalog on explicit inspect/resume;
no index can retroactively discover an unknown legacy ID through private SDK tables.

## Automated evidence

| Test | Evidence |
| --- | --- |
| TestWorkerChildrenApprovalRecoveryAndTaskIdentity | Two children, stale approval revalidation, close/reopen, parent resume, stable child/task IDs, shared budget and verified writes |
| TestWorkerDenialDoesNotAuthorizeAnotherChild | Denial creates no file and does not authorize the next child |
| TestWorkerParallelReadsAndSerializedWrites | Barrier proves overlapping reads; middleware counter proves mutation maximum one |
| TestWorkerCancellationPropagatesAndInspectionDoesNotRestart | Cancel root with two child streams; reopen inspection without starting work |
| TestNestedChildrenYieldSingleWorkerSlot | Three-level hierarchy completes with one child slot and depth two |
| TestSharedBudgetFailsBeforeInference | Budget exhaustion reaches no provider call |
| TestActiveControlsAreOrderedAndPersisted | Ordered follow-ups and steering persist in checkpoint and control journal |
| TestControlJournalRecoversMissingQueueExactlyOnce | Inject missing queue after checkpoint, recover from journal, avoid duplicate on second resume |
| TestSessionCatalogBranchCompactionAndExport | Session pagination/grouping, branches from current/historical checkpoints, reset grants, compaction archive, redacted export and branch continuation |
| TestMigrationBackupVersionAndCAS / TestCorruptDatabaseFailsBeforeUse | Backup, stale-write rejection, future schema and corrupt database refusal |
| TestQueueFIFOWithCanceledMiddleWaiter | FIFO order survives cancellation of an intervening waiter |
| TestSessionsInspectionDoesNotLoadProviderOrCredentials | Headless inspection opens no provider/store secret |

The prior approval-denial, stale-diff, uncertain-effect reconciliation, terminal/PTY,
HTTP provider, OAuth and RuntimeView/OperationStore regression tests remain in the
suite. Deterministic fixtures require no real provider credentials.

## Validation and limits

Validation covers `scripts check` (format/architecture/vet/tests/external SDK
consumer/build), golangci-lint, race detection on changed packages and the six-target
CGO-free cross-build matrix. Native Linux/Windows execution remains delegated to
configured Opera CI; this local repository has not been published to run it remotely.

Serializing mutations does not eliminate logical conflicts between agents. Tools
are not an operating-system sandbox. Token reservations are conservative resource
bounds, not final token/cost accounting. Provider summaries can lose nuance; exact
pre-compaction messages and provenance remain archived. Daemon, remote workers,
automatic worktrees and Opera publication remain outside this delivery.

# ADR 002 — Actions, events and headless output

Status: accepted in F0; execution bus is F1.

Application actions express start, resume, follow-up, steer, approve and cancel.
The application translates them to public runner controls. A common execution
service owns policy and persistence for every delivery mode. UI Update/View do
not perform I/O or manipulate providers and agent goroutines.

The [output contract](../cli-contract.md) is versioned from F0. Execution events
will carry timestamps, durable sequence and session/branch/run/task/agent/operation
correlation. Follow-ups are ordered input for a later turn; steering explicitly
changes active execution. Cross-agent order is reception order at the parent;
each run retains its own durable sequence. Replay cursors prevent duplicate
accounting and scrollback commits.

Use bounded queues, cancellation and backpressure. Coalesce streaming deltas only
when the final content remains exact. Approvals, operation results and terminal
states are never dropped. A slow consumer must not create unbounded goroutines.
Headless output never opens forms; approval persists and exits with code 3.

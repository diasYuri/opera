# ADR 003 — Persistence, identities and recovery

Status: accepted in F0; durable adapters/runner integration are F1/F3.

Generic SQLite and filesystem-memory adapters belong in Y as public APIs. Opera
wires them and owns product metadata, migrations/retention policy and commands.
Required persistence failure stops execution; it never silently selects NoopStore.
Published Y v1.2.0 has interfaces/JSONL/in-memory stores, while reusable SQLite and
filesystem-memory adapters still need public exposure (Y2/Y3).

Keep session, branch, run, task, agent and operation IDs distinct. Checkpoints
capture consistent durable state with a schema version and event cursor. F1 must
define transaction boundaries spanning snapshot, operation outcome and events.
F3 adds task hierarchy/branch state. A branch preserves provenance but receives
new execution/operation identities and cannot replay effects or inherit grants.

Operation identity is recorded before side effects. Completed operations reuse
their stored outcome when safe. A crash between an external effect and outcome
commit leaves an in-doubt operation: retry is blocked until explicit reconciliation
records the observed effect or a newly authorized retry. Exactly-once for arbitrary
shell/API effects is not promised. Cleanup preserves data; inspection never resumes
work automatically. Test crashes on both sides of every effect/commit boundary.

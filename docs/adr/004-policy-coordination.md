# ADR 004 — Policy and workspace coordination

Status: configuration boundary implemented in F0; operation enforcement is F1/F3.

Default to `ask`; offer `read-only` and explicit `trusted`. Project configuration
may reduce policy but cannot increase the user-configured ceiling. Reject attempts
to elevate and require fixing the project file; flags/environment cannot hide an
invalid lower layer. Credential-store selection is never project-controlled.

Authorization attaches to an operation's identity, relevant arguments, paths,
diff/preconditions and policy/view version. A shared approval queue identifies
the requesting agent. Approval does not grant indefinite tool-wide permission.
Preparation and user waiting happen outside the mutation lock; acquire the lock,
revalidate preconditions and renew approval if the approved change has changed.

All agents share the canonical workspace. Reads may run concurrently; mutations
use a cancellable exclusive queue, including shell and unclassified external tools.
LSP edits, Git, MCP and WASM must use this same path. Multiple CLIs will either
share local locking or refuse concurrent workspace ownership in F3. Serialization
does not eliminate logical editing conflicts and is not an OS sandbox.

Defaults: four concurrent children, maximum depth one. Future execution enforces
the configured bounded limits and parent-to-child cancellation. RuntimeView snapshots
must retain the durable OperationStore boundary: Y6's integration test is a hard
gate before expanding the UI, not an assumed property of the existing wrapper.

# ADR 001 — Public harness composition and lifecycle

Status: accepted. Package organization is superseded by ADR 009; lifecycle guarantees remain active.

Opera owns factories and product defaults over Y's public `runtime.Harness` and
component service keys. `internal/modules/execution` owns use cases shared by
chat, run and synchronous worker; `internal/ui/cli` adapts process I/O and
`internal/ui/tui` presents its actions/events. Do not import Y internals, invoke
`y` as a backend or copy its agent loop.

Every resource belongs to a component scope: providers, processes, clients,
subscriptions, registries, views and child tasks. Startup failure rolls back
acquisitions. Shutdown stops new actions, cancels the tree and queues, waits
within a bounded deadline, saves consistent durable state, releases views and
registrations, then closes components in reverse dependency order and stores
last. Cleanup uses a separate context (default 10 seconds), attempts every closer
and aggregates errors. It preserves user data and never implies effect rollback.

The F0 consumer test constructs/starts/closes a published empty Harness with the
complete Charm stack compiled alongside it. F1 must add failure-path lifecycle
tests for the concrete composition. No execution survives its owning CLI.

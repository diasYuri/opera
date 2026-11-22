# Develop Opera

## Locate the owner

| Change | Start here |
| --- | --- |
| Graph, profiles of compiled components, startup and shutdown | `internal/core`, `docs/composition.md`, ADRs 001 and 009 |
| Runs, task plans, local children, approvals and sessions | `internal/modules/execution`, `docs/operations.md`, ADRs 002–004 and 010 |
| Agent instructions | `internal/modules/execution/prompt_builder.go`, `prompt_guidance.go`, `profiles.go` and `modes.go` |
| Configuration, accounting, workspace or context rules | Matching capability in `internal/modules` and its concrete adapters |
| Command-line entry, JSONL, exit codes or chat commands | `internal/ui/cli`, `docs/cli-contract.md` |
| Terminal interaction, focus and rendering | `internal/ui/tui`, `DESIGN.md`, `docs/tui-development.md`, `.agents/skills/opera-tui-ui/SKILL.md` |
| Skills and embedded Opera references | `internal/adapters/skills/filesystem`, `internal/modules/skills/catalog`, `docs/skills.md` |

Read the checkout's `AGENTS.md`, `docs/refactoring-plan.md` and relevant `docs/adr/` entries first. Follow actual caller, effect, persistence and recovery paths before changing a public contract.

## Compose with the public SDK Y

- `internal/core` owns the composition root and `runtime.Harness` lifecycle. Y owns the agent loop, turns, registry and checkpoint protocols. Keep product rules in cohesive modules, external effects in adapters, process translation in CLI, and presentation in the TUI.
- Add replaceable capabilities through explicit catalog entries and `component.Definition`. Give each `Key[T]` an owner, declare service requirements and bindings, and keep implementation versions independent of service versions. Acquire resources in Start and attach ownership to the component scope immediately. Validate the graph, partial-start rollback and inverse cleanup.
- Let existing views retain their tools, handlers, hooks and context. Use supported safe points for runtime changes. Keep approval and effect identity stable. Preserve transcripts, checkpoints and operation records during cleanup, and explicitly reconcile uncertain effects.
- Read the Y dependency version in `go.mod` and inspect that version's public exports. `docs/y-public-api.json` is the generated inventory and `docs/y-capability-coverage.md` records the mapping. If the public contract is missing, identify the SDK gap and integrate a published release when available. Y's `docs/philosophy.md`, `docs/component-contract.md`, `docs/runtime-composition.md`, `docs/effects-and-durable-state.md` and `docs/runtime-protocol.md` explain its design.

## Change agent behavior through the execution module

`AgentProfile` selects specialist guidance and capability restrictions. `RuntimeMode` selects the execution strategy and its own restrictions. Both intersect with effective policy. The prompt builder composes guidance, workspace context and skill metadata. Change execution authority at the tool and policy boundaries.

Follow the real run setup in `Service.Execute`. Use durable tasks and child runs through the existing Work service. Persisted definitions and tool and skill generations participate in resume checks. When altering them, test the transition from existing records, including approvals and interrupted children. Use scripted providers to observe what a real run sends to the model.

## Validate

Use the pinned Go version and Makefile. Run focused tests before the relevant project gates. `make check` checks formatting, boundaries, modules, vet, tests, SDK consumption and a CGO-free build. `make lint` runs the pinned linter. `make matrix` cross-compiles supported targets. `make inventory` regenerates the public SDK inventory. Verify that inventory and module changes are intentional.

Use offline scripted providers and CLI smoke checks. For changes to persistence or effects, exercise partial failure, cancellation and resume. For TUI changes, verify focus, resize, Unicode/ANSI, accessibility and actual terminal interaction where supported. Live provider tests require separate authorization and credentials.

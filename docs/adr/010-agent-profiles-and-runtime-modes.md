# ADR 010: Agent profiles and runtime modes

- Status: accepted
- Date: 2026-11-18

## Context

Reusable agent specialization and execution composition describe different
concerns. Treating a profile as an execution mode would couple reusable agent
instructions and requirements to a particular orchestration, lifecycle, or
runtime topology. Conversely, prompt instructions alone cannot establish which
effects an agent is authorized to perform.

Resumption and child execution also need stable provenance: the system must be
able to identify the profile and mode configuration used to construct a run,
even if definitions change later.

## Decision

1. `AgentProfile` is a reusable specialization of an agent. A profile can be
   invoked directly, for example with `/review`, and can be selected for child
   agents. It contains an instruction or prompt recipe, profile-specific
   context, and required or restricted capabilities and tools.
2. `RuntimeMode` defines Y runtime execution and composition. It may change
   orchestration, lifecycle, topology, and capability or policy composition.
   An `AgentProfile` is not a `RuntimeMode`.
3. Profiles and modes are independent, combinable dimensions. Effective
   authority is the intersection of the profile, runtime mode, and applicable
   policy. Prompt text does not grant authority or authorize effects; effect
   boundaries continue to enforce capabilities, policy, consent, and approval.
4. `internal/core` composes and mounts `RuntimeMode` through the public Y SDK.
   The `execution` module resolves profiles and builds prompt inputs from their
   instructions and context. It does not duplicate Y's agent loop, turns, or
   runtime lifecycle.
5. A run and its children or resumptions retain stable profile and mode
   identities together with versions or content digests sufficient to identify
   the definitions used. Changes to a definition must not silently change the
   meaning of an existing run or its provenance.
6. The first implementation provides `default` and `reviewer` profiles, with
   `/review [task]` selecting the reviewer within the currently selected
   RuntimeMode. Profile selection for child agents and additional RuntimeMode
   protocols remain future work. Each future behavior requires a concrete
   protocol and the corresponding capabilities to be published by Y before
   Opera relies on them.

## Consequences

- Profile authors can describe agent specialization and capability constraints
  without selecting or replacing runtime orchestration.
- A selected mode cannot override policy, and a profile's prompt recipe cannot
  bypass effect authorization. The effective set of capabilities must be
  computed and enforced at the relevant boundaries.
- Durable execution records persist profile and mode identity plus definition
  digests for consistent resumption. Child profile inheritance and provenance
  are not implemented yet.
- Implementation work must use published public Y APIs and define the protocol
  and failure behavior for each new runtime capability; this ADR alone does not
  promise those capabilities.

## Alternatives rejected

- Model profiles as runtime modes: this conflates reusable agent specialization
  with execution and lifecycle composition, and prevents independent
  combination.
- Let prompt instructions authorize tools or effects: prompts are inputs to
  agent behavior, not an authorization boundary.
- Implement a separate Opera agent loop to support profiles: the public Y SDK
  owns runtime execution and lifecycle; `execution` prepares profile-based
  inputs and does not reimplement them.

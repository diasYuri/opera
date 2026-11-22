# ADR 012: Bundled Opera skill

- Status: accepted
- Date: 2026-11-22

## Decision

Standard binaries embed the `opera` Agent Skill and its operational,
development and extension references. The existing skill catalog and tools
serve its instructions and resources from memory. The reserved name takes
precedence over workspace and user copies, and appears first in the catalog
so it survives prompt truncation. The skill loads on demand.

The package content participates in the pinned skill generation. To resume
a checkpoint created before the bundled skill, verify the original disk
catalog digest and persist the new generation. A previous disk skill named
`opera` requires the original build to resume if it was part of the pinned
catalog. Changes to bundled resources require the original build for existing
checkpoints that pinned their digest.

Skill tools are reads in Opera's policy, including read-only planning and
review. The WASM bridge continues to exclude skill tools. Skill content and
metadata never grant authorization to execute tools or change the workspace.

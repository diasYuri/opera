---
name: opera
description: Use for questions about Opera itself and for changing or extending Opera. Covers CLI and chat, configuration, tools and skills, approvals and recovery, SDK Y composition, providers, MCP, WASM, memory and UI extensions.
---

# Opera

Use this skill when the subject is Opera itself. It provides version-matched guidance for the installed binary and a map of the contracts to read when developing Opera.

Read the reference for the current job with `skill_resource`, passing `name: "opera"` and its relative path:

| Job | Bundled reference |
| --- | --- |
| Run or diagnose Opera, configure it, inspect tools, recover a run | [references/operate.md](references/operate.md) |
| Fix or evolve Opera, modify agent behavior, integrate with SDK Y, validate a change | [references/develop.md](references/develop.md) |
| Add a provider, component, tool pack, skill or extension, or change memory or UI contributions | [references/extend.md](references/extend.md) |

For questions about an installed binary, use its help, offline diagnostics and these references. For source changes, read the checkout's `AGENTS.md`, the relevant contracts named in the reference and the real entry points and tests. A checkout can be ahead of or behind the installed release. Verify which tree and version the request concerns before making a change or describing a feature.

Treat diagnosis, configuration and source edits as distinct actions. Respect the user's intent and the effective workspace policy. Inspecting a configuration does not activate its extensions or grant authority to its tools.

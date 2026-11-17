# Opera

Opera is a local coding harness built on the public Y SDK. Its CLI and interactive terminal chat share foreground execution, streaming, operation-bound approvals, SQLite checkpoints and explicit resume.

The inferred primary audience is developers working in a local repository. They need to inspect code, request changes, review proposed operations and continue interrupted work while retaining their terminal history and control over provider, model and workspace policy.

The product should feel minimal, direct and dependable. The confirmed visual references are Codex and pi terminal interfaces. Keep the existing English voice: short state labels, concrete commands and actionable recovery messages. Show complete diagnostics on demand rather than crowding the conversation.

The terminal is the platform. Preserve terminal scrollback, the user's background and keyboard-first interaction. The normal chat and append-only `--accessible` path use the same backend and durable approval behavior. Color, animation and continuous redraw must not be prerequisites for understanding or operating the product.

Execution stays attached to the CLI process. Daemons, remote workers, automatic worktrees and public publication are outside the current delivery scope. See [README.md](README.md) for capabilities and [docs/accessibility.md](docs/accessibility.md) for keyboard and accessibility contracts.

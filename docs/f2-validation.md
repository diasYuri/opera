# F2 interactive coding

F2 extends the existing durable execution backend. No Y change or local module
replacement is needed; Opera still consumes published Y v1.3.1.

## Behavior

- Multiline textarea, bracketed paste, Alt+Up/Down history; Enter submits,
  Alt+Enter or Ctrl+J inserts a newline (Ctrl+J is the terminal compatibility fallback).
- Stream fragments deduplicate by persisted event sequence. Completed blocks commit
  once per run/checkpoint, so approval continuation can produce a new block without
  reprinting an earlier checkpoint. Resize affects only the dynamic region.
- Bubbles model selector and viewport panels stay bounded. `/model` selects from the
  provider catalog; `/model <provider> <model>` allows explicit IDs. `/thinking`
  validates levels and advertised support. Selection rebuilds the scoped backend
  while idle, preserving conversation IDs; approval must be resolved first.
- `/context [path]` lists sources or adds a file. `/settings` opens editable
  workspace settings, including per-provider web search defaults. `/help` comes
  from the shared application command registry. CLI
  equivalents include `context`, `settings`, `--provider`, `--model`, `--thinking`,
  and repeatable `--attach`. Accessible chat supports the same textual commands.
- Root AGENTS.md loads first, then ancestor AGENTS.md files from shallow to deep
  for explicitly attached paths, then attached files. Instructions apply to their
  directory scope; deeper instructions take precedence in that scope. Sources
  retain relative paths and byte counts. Content cannot authorize operations.
- Context rejects outside paths, symlinks, Git private paths, ignored files, invalid
  UTF-8 and oversized content. The SDK ignore matcher works without Git; installed
  Git additionally applies repository/global excludes. Total attachment/instruction
  bytes are capped at min(configured attachment bytes, 2 × configured context tokens, 256 KiB), followed by the
  provider request token estimate. Credential assignment lines are redacted before
  provider delivery and persistence. This is pattern redaction, not general secret discovery.
- Supported image formats are PNG, JPEG, GIF and WebP. Image payloads are injected
  into copied provider request messages and never inserted into durable transcripts;
  context records a source/hash. Missing files, unsupported vision and budget excess
  fail before inference. Resume revalidates source availability. Other binary
  attachment modalities return explicit errors.
- All public filesystem, edit/patch, shell, Git and source-search handlers share the
  existing policy, durable operation wrapper and workspace queue. Known read tools
  may run concurrently; shell and unclassified handlers serialize conservatively.
  Patch previews/fingerprints include affected file preconditions. Approval remains
  operation-bound and changed files require fresh review.
- LSP tools use SDK default language-server discovery and lazy scoped processes,
  including process-start authorization and registry shutdown. Standard builds use
  `feature_lsp`; direct `go build` must pass `-tags=feature_lsp`. `doctor` reports
  missing servers, rg and ast-grep. Filesystem `search`/`read_file` are explicit
  fallbacks when semantic/AST/external text search is unavailable.
- Terminal background messages select a light/dark blue accent. NO_COLOR selects
  the ASCII color profile. Harmonica smooths the progress indicator on incoming
  events; it creates no periodic ticks. `--no-motion` disables it and cursor blink.
  `--accessible` uses append-only output. Ctrl+C cancels an active run and preserves
  the conversation; idle Ctrl+C and empty Ctrl+D exit. Bubble Tea restores terminal state.

## Evidence

- `TestCodingEditApprovalAndShell`: filesystem edit pauses, resumes with the reviewed
  approval and writes exactly the requested result.
- `TestPatchApprovalIncludesNestedFilePrecondition`: a changed patch target requires
  a new approval, including paths whose first directory is named b.
- `TestCodingShellRequiresApproval`: shell pauses before execution, then resumes.
- `TestSemanticFallbackAndGitInspection`: semantic-tool failure, filesystem fallback
  and Git calls are observed in the durable operation store with external tools absent.
- `TestReadOnlyRejectsShellWithoutEffect`: shell denial creates no file.
- `TestContextCommandsAndSafeModelSwitch`: idle model switch and invalid context/thinking.
- `TestScopedContextBudgetIgnoreAndRedaction`: instruction ordering, provenance,
  ignore, path containment, budgets and credential assignment redaction.
- `TestImageIsTransientAndRequiresVision`: image source hashes and modality gate.
- `TestPasteHistoryAndCheckpointCommitIdentity`: multiline paste remains input,
  history navigation and distinct approval continuation/replay identities.
- Existing long Unicode/resize and real PTY restoration tests continue to run,
  together with F1 approval, cancellation, recovery and durable replay fixtures.

Validation runs use fake providers/local tools; real provider credentials are not
required. Native Linux/Windows execution still belongs to configured CI; local
cross-builds do not substitute for those platform runs. SDK v1.3.1's previously
reported Ubuntu ast-grep test failure remains separate from Opera's local checks.

Final local checks: `go run ./scripts check`, golangci-lint (zero issues),
`GOFLAGS=-tags=feature_lsp go test -race` for harness/UI/context/CLI, and
`go run ./scripts matrix` (Linux/macOS/Windows × amd64/arm64, CGO disabled).
